package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/preflight"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

type operationProgressMsg struct {
	progress restic.BackupProgress
	events   <-chan tea.Msg
}

type operationDoneMsg struct {
	err       error
	snapshots []restic.Snapshot
	manifest  string
}
type preflightDoneMsg struct {
	report preflight.Report
	heavy  heavy.Report
}
type heavyInspectMsg struct{ report heavy.Report }
type sudoDoneMsg struct{ err error }
type detachDoneMsg struct{ err error }

func (m Model) startBackup() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	m.incomplete, m.skippedPaths, m.skippedManifest = false, nil, ""
	_, profile, _, err := m.activeRuntime(m.selectedProfile)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.operation, m.status, m.err = true, "preflight", "checking file readability", ""
	return m, func() tea.Msg {
		message := preflightDoneMsg{report: preflight.Scan(profile.Includes, profile.Excludes)}
		if profile.Name == "heavy" {
			message.heavy = heavy.Inspect(profile.Includes)
		}
		return message
	}
}

func (m Model) inspectHeavy() tea.Cmd {
	var roots []string
	for _, profile := range m.cfg.Profiles {
		if profile.Name == "heavy" {
			roots = append(roots, profile.Includes...)
		}
	}
	return func() tea.Msg { return heavyInspectMsg{report: heavy.Inspect(roots)} }
}

func (m Model) beginEngineBackup() (tea.Model, tea.Cmd) {
	destination, profile, repo, err := m.activeRuntime(m.selectedProfile)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy, m.operation = cancel, true, "backup"
	m.err = ""
	m.progress = restic.BackupProgress{MessageType: "discovery"}
	m.status = "discovering files"
	// Rendering may pause while a tmux client detaches. Keep only the latest
	// progress frame so terminal lifecycle can never back-pressure Restic.
	events := make(chan tea.Msg, 1)
	go func() {
		if identityErr := verifyTUIRepositoryIdentity(ctx, &m.cfg, destination, repo); identityErr != nil {
			sendLatestOperationEvent(events, operationDoneMsg{err: identityErr})
			close(events)
			return
		}
		manifestPath, cleanupManifest, prepareErr := backupmeta.PrepareApplications(ctx, profile.Name)
		if prepareErr != nil {
			sendLatestOperationEvent(events, operationDoneMsg{err: fmt.Errorf("application manifest: %w", prepareErr)})
			close(events)
			return
		}
		defer cleanupManifest()
		excludes := append(append([]string{}, profile.Excludes...), m.skippedPaths...)
		if profile.Name == "core" || profile.Name == "home" {
			home, _ := os.UserHomeDir()
			excludes = append(excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
		}
		includes := append([]string(nil), profile.Includes...)
		if manifestPath != "" {
			includes = append(includes, manifestPath)
		}
		wireCtx, stopWire := context.WithCancel(ctx)
		var wireMu sync.RWMutex
		var wireRate float64
		go restic.SampleWireRate(wireCtx, restic.NewWireCounter(destination.Repository), func(rate float64) {
			wireMu.Lock()
			wireRate = rate
			wireMu.Unlock()
		})
		defer stopWire()
		err := restic.NewService(io.Discard).BackupMachineWithProgress(ctx, repo, profile.Name, m.cfg.Machine.ID,
			includes, excludes, false, m.incomplete, func(progress restic.BackupProgress) {
				wireMu.RLock()
				progress.WireBytesPerSecond = wireRate
				wireMu.RUnlock()
				sendLatestOperationEvent(events, operationProgressMsg{progress: progress, events: events})
			})
		if err == nil {
			_ = catalog.Refresh(ctx, m.vault, destination.ID, repo, m.cfg.Machine.ID, profile.Name)
		}
		manifest := ""
		if err == nil && m.incomplete {
			manifest, err = writeSkippedManifest(profile.Name, destination.ID, m.skippedPaths)
		}
		sendLatestOperationEvent(events, operationDoneMsg{err: err, manifest: manifest})
		close(events)
	}()
	_ = destination
	return m, waitOperation(events)
}

func verifyTUIRepositoryIdentity(ctx context.Context, cfg *config.Config, destination *config.Destination, repo restic.Repository) error {
	id, err := restic.NewService(io.Discard).RepositoryID(ctx, repo)
	if err != nil {
		return fmt.Errorf("verify repository identity: %w", err)
	}
	if destination.RepositoryID != "" && destination.RepositoryID != id {
		return fmt.Errorf("repository identity mismatch: expected %s, got %s", destination.RepositoryID, id)
	}
	if destination.RepositoryID == "" {
		destination.RepositoryID = id
		path, pathErr := config.Path()
		if pathErr != nil {
			return pathErr
		}
		return config.Save(path, *cfg)
	}
	return nil
}

func sendLatestOperationEvent(events chan tea.Msg, message tea.Msg) {
	select {
	case events <- message:
		return
	default:
	}
	select {
	case <-events:
	default:
	}
	events <- message
}

func writeSkippedManifest(profile, destination string, skipped []string) (string, error) {
	path, err := preflight.ManifestPath()
	if err != nil {
		return "", err
	}
	err = preflight.WriteManifest(path, preflight.Manifest{SchemaVersion: 1, CreatedAt: time.Now(), Profile: profile,
		Destination: destination, Reason: "sudo declined", Skipped: skipped})
	return path, err
}

func authorizeSudoCmd() tea.Cmd {
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(command, func(err error) tea.Msg { return sudoDoneMsg{err: err} })
}

func (m Model) startSnapshots() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.operation, m.status = true, "restore-point refresh", "loading restore points"
	return m, func() tea.Msg {
		snapshots, err := restic.NewService(io.Discard).SnapshotsForMachine(context.Background(), repo, m.cfg.Machine.ID)
		return operationDoneMsg{err: err, snapshots: snapshots}
	}
}

func (m Model) startCheck() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy, m.operation, m.status = cancel, true, "repository check", "checking repository"
	return m, func() tea.Msg {
		return operationDoneMsg{err: restic.NewService(io.Discard).Check(ctx, repo, false)}
	}
}

func (m Model) activeRuntime(profileName string) (*config.Destination, *config.Profile, restic.Repository, error) {
	if m.vault == nil || !m.vault.Unlocked() {
		return nil, nil, restic.Repository{}, fmt.Errorf("vault is locked")
	}
	if len(m.cfg.Destinations) == 0 {
		return nil, nil, restic.Repository{}, fmt.Errorf("no destination; run init or open Destinations")
	}
	destination := m.cfg.Active()
	password, err := m.vault.Get(destination.PasswordKey)
	if err != nil {
		return nil, nil, restic.Repository{}, err
	}
	repo := restic.Repository{Location: destination.Repository, Password: password, KnownHosts: destination.SSHKnownHosts,
		Elevated: destination.Privileged, Connections: destination.Connections, UploadLimitKiB: destination.UploadLimitKiB}
	if destination.SSHKeyKey != "" {
		repo.SSHKey, err = m.vault.Get(destination.SSHKeyKey)
	}
	var profile *config.Profile
	for i := range m.cfg.Profiles {
		if m.cfg.Profiles[i].Name == profileName {
			profile = &m.cfg.Profiles[i]
		}
	}
	if profileName != "" && profile == nil {
		return nil, nil, repo, fmt.Errorf("profile %q not found", profileName)
	}
	return destination, profile, repo, err
}

func waitOperation(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return operationDoneMsg{}
		}
		return message
	}
}

func progressStatus(progress restic.BackupProgress) string {
	if progress.MessageType != "status" {
		return progress.MessageType
	}
	return fmt.Sprintf("%3.0f%%  %d/%d files  %s / %s  %s elapsed  %s left",
		progress.PercentDone*100, progress.FilesDone, progress.TotalFiles,
		bytesText(progress.BytesDone), bytesText(progress.TotalBytes),
		(time.Duration(progress.SecondsElapsed) * time.Second).String(),
		(time.Duration(progress.SecondsRemaining) * time.Second).String())
}

func bytesText(value uint64) string {
	if value < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(value)/(1<<10))
	}
	if value < 1<<30 {
		return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
	}
	return fmt.Sprintf("%.1f GiB", float64(value)/(1<<30))
}
