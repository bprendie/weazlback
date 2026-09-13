package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

type packageSudoDoneMsg struct{ err error }
type packageProgressMsg struct {
	progress packagecapsule.Progress
	events   <-chan tea.Msg
}
type packageDoneMsg struct {
	manifest packagecapsule.Manifest
	err      error
}

func (m Model) requestPackageCapture(buildAUR bool) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	m.packageBuildAUR, m.packageStage = buildAUR, "authorizing"
	m.status, m.err = "authorize package artifact capture with sudo", ""
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return packageSudoDoneMsg{err: err} })
}

func (m Model) beginPackageCapture() (tea.Model, tea.Cmd) {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err, m.packageStage = err.Error(), ""
		return m, nil
	}
	root, err := packageTUIStagingRoot()
	if err != nil {
		m.err, m.packageStage = err.Error(), ""
		return m, nil
	}
	m.busy, m.operation, m.packageStage = true, "package capsule", "capturing"
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.packageProgress = packagecapsule.Progress{Phase: "inventory"}
	m.status = "inventorying installed packages"
	events := make(chan tea.Msg, 1)
	go func() {
		if err := verifyTUIRepositoryIdentity(ctx, &m.cfg, destination, repo); err != nil {
			sendLatestOperationEvent(events, packageDoneMsg{err: err})
			close(events)
			return
		}
		manifest, staging, cleanup, err := packagecapsule.Capture(packagecapsule.Options{
			Context: ctx, MachineID: m.cfg.Machine.ID, StagingRoot: root, Download: m.cfg.PackagePolicy.DownloadOfficial,
			BuildMissingAUR: m.packageBuildAUR, Run: packagecapsule.ExecRunner{Context: ctx, Quiet: true},
			Progress: func(progress packagecapsule.Progress) {
				sendLatestOperationEvent(events, packageProgressMsg{progress: progress, events: events})
			},
		})
		if err == nil {
			err = restic.NewService(io.Discard).BackupMachineWithProgress(ctx, repo, "packages", m.cfg.Machine.ID,
				[]string{staging}, nil, false, false, func(progress restic.BackupProgress) {
					sendLatestOperationEvent(events, packageProgressMsg{progress: packagecapsule.Progress{Phase: "repository", Completed: int(progress.FilesDone), Total: int(progress.TotalFiles), Bytes: int64(progress.BytesDone)}, events: events})
				})
		}
		if cleanup != nil {
			cleanup()
		}
		if err == nil {
			_ = catalog.Refresh(ctx, m.vault, destination.ID, repo, m.cfg.Machine.ID, "packages")
		}
		sendLatestOperationEvent(events, packageDoneMsg{manifest: manifest, err: err})
		close(events)
	}()
	return m, waitPackageEvent(events)
}

func packageTUIStagingRoot() (string, error) {
	path, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "staging"), nil
}

func waitPackageEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return packageDoneMsg{}
		}
		return message
	}
}

func (m Model) togglePackageSchedule() (tea.Model, tea.Cmd) {
	m.cfg.PackagePolicy.Scheduled = !m.cfg.PackagePolicy.Scheduled
	path, err := config.Path()
	if err == nil {
		err = config.Save(path, m.cfg)
	}
	if err != nil {
		m.err, m.status = err.Error(), "package schedule update failed"
		return m, nil
	}
	state := "disabled"
	if m.cfg.PackagePolicy.Scheduled {
		state = fmt.Sprintf("every %d days", m.cfg.PackagePolicy.IntervalDays)
	}
	m.err, m.status = "", "Package Capsule schedule "+state
	return m, nil
}
