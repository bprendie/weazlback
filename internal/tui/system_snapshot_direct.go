package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

type systemSnapshotSudoMsg struct {
	action string
	err    error
}
type systemSnapshotDoneMsg struct {
	id  string
	err error
}
type systemSnapshotLane struct {
	State, Current        string
	Completed, Total      int
	Percent               float64
	BytesDone, BytesTotal uint64
	Rate                  float64
}
type systemSnapshotProgressMsg struct {
	lane     string
	progress systemSnapshotLane
	events   <-chan tea.Msg
}

func (m Model) authorizeSystemSnapshot(action string) (tea.Model, tea.Cmd) {
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	m.busy, m.operation, m.err = true, "authorizing Full System Snapshot", ""
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return systemSnapshotSudoMsg{action: action, err: err} })
}

func (m Model) beginDirectSystemSnapshot(action string) (tea.Model, tea.Cmd) {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.busy, m.err = false, err.Error()
		return m, nil
	}
	cfg, v := m.cfg, m.vault
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.operation, m.systemSnapshotStart = cancel, "Full System Snapshot "+action, time.Now()
	m.systemSnapshotLanes = map[string]systemSnapshotLane{}
	for _, lane := range []string{"packages", "aur", "core", "home", "heavy"} {
		m.systemSnapshotLanes[lane] = systemSnapshotLane{State: "waiting"}
	}
	events := make(chan tea.Msg, 64)
	go func() {
		emit := func(lane string, progress systemSnapshotLane) {
			sendSystemSnapshotEvent(events, systemSnapshotProgressMsg{lane: lane, progress: progress, events: events})
		}
		service := restic.NewService(io.Discard)
		id, runErr := generation.ExecuteParallel(ctx, service, repo, cfg.Machine.ID, action, "", func(profile, id string) error {
			return captureTUISystemLane(ctx, cfg, repo, profile, id, emit)
		})
		result := "complete"
		if runErr != nil {
			result = "failed"
		}
		_, auditErr := generation.SaveAudit(v, generation.Audit{GenerationID: id, RepositoryID: destination.RepositoryID, Action: "capture", Result: result})
		if runErr == nil {
			runErr = auditErr
		}
		sendSystemSnapshotEvent(events, systemSnapshotDoneMsg{id: id, err: runErr})
		close(events)
	}()
	return m, waitSystemSnapshotEvent(events)
}

func captureTUISystemLane(ctx context.Context, cfg config.Config, repo restic.Repository, profile, id string, emit func(string, systemSnapshotLane)) error {
	if profile == "generation-ledger" {
		return captureTUILedger(ctx, repo, cfg.Machine.ID, id)
	}
	if profile == "packages" {
		return captureTUIPackages(ctx, cfg, repo, id, emit)
	}
	p := tuiProfile(cfg, profile)
	if p == nil {
		return fmt.Errorf("profile %q is not configured", profile)
	}
	if profile == "heavy" {
		emit(profile, systemSnapshotLane{State: "preflight", Current: "checking writable workloads"})
		if report := heavy.Inspect(p.Includes); !report.Safe {
			return fmt.Errorf("Heavy contains writable open files; stop workloads and retry")
		}
	}
	manifest, cleanup, err := backupmeta.PrepareApplications(ctx, profile)
	if err != nil {
		return err
	}
	defer cleanup()
	includes := append([]string(nil), p.Includes...)
	if manifest != "" {
		includes = append(includes, manifest)
	}
	excludes := append([]string(nil), p.Excludes...)
	if profile == "core" || profile == "home" {
		home, _ := os.UserHomeDir()
		excludes = append(excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
	}
	started := time.Now()
	emit(profile, systemSnapshotLane{State: "discovering", Current: "measuring files"})
	err = restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, profile, cfg.Machine.ID, []string{generation.TagPrefix + id}, includes, excludes, false, false, func(p restic.BackupProgress) {
		rate := float64(0)
		if elapsed := time.Since(started).Seconds(); elapsed > 0 {
			rate = float64(p.BytesDone) / elapsed
		}
		emit(profile, systemSnapshotLane{State: "running", Current: fmt.Sprintf("%d/%d files", p.FilesDone, p.TotalFiles), Completed: int(p.FilesDone), Total: int(p.TotalFiles), Percent: p.PercentDone, BytesDone: p.BytesDone, BytesTotal: p.TotalBytes, Rate: rate})
	})
	if err == nil {
		emit(profile, systemSnapshotLane{State: "complete", Current: "complete", Completed: 1, Total: 1, Percent: 1})
	}
	return err
}

func captureTUILedger(ctx context.Context, repo restic.Repository, machineID, id string) error {
	dir, err := os.MkdirTemp("", "weazlback-generation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "generation.json"), []byte(id+"\n"), 0o600); err != nil {
		return err
	}
	return restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, "generation-ledger", machineID, []string{generation.TagPrefix + id}, []string{dir}, nil, false, false, nil)
}

func captureTUIPackages(ctx context.Context, cfg config.Config, repo restic.Repository, id string, emit func(string, systemSnapshotLane)) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	staging := filepath.Join(filepath.Dir(path), "staging")
	emit("packages", systemSnapshotLane{State: "running", Current: "inventorying installed packages"})
	emit("aur", systemSnapshotLane{State: "running", Current: "inventorying cached artifacts"})
	manifest, root, cleanup, err := packagecapsule.Capture(packagecapsule.Options{
		Context: ctx, MachineID: cfg.Machine.ID, StagingRoot: staging, Download: true,
		Run: packagecapsule.ExecRunner{Context: ctx, Quiet: true},
		Progress: func(p packagecapsule.Progress) {
			lane := "packages"
			if p.Source == "foreign" {
				lane = "aur"
			}
			bytes := uint64(0)
			if p.Bytes > 0 {
				bytes = uint64(p.Bytes)
			}
			emit(lane, systemSnapshotLane{State: p.Phase, Current: p.Package, Completed: p.Completed, Total: p.Total, Percent: lanePercent(p.Completed, p.Total), BytesDone: bytes})
		},
	})
	if err != nil {
		return err
	}
	defer cleanup()
	emit("packages", systemSnapshotLane{State: "archiving", Current: "writing encrypted capsule", Completed: manifest.Summary.Official, Total: manifest.Summary.Official, Percent: 1})
	emit("aur", systemSnapshotLane{State: "archiving", Current: "writing encrypted capsule", Completed: manifest.Summary.Foreign, Total: manifest.Summary.Foreign, Percent: 1})
	err = restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, "packages", cfg.Machine.ID, []string{generation.TagPrefix + id}, []string{root}, nil, false, false, nil)
	if err == nil {
		emit("packages", systemSnapshotLane{State: "complete", Current: "capsule complete", Completed: manifest.Summary.Official, Total: manifest.Summary.Official, Percent: 1})
		emit("aur", systemSnapshotLane{State: "complete", Current: "artifacts complete", Completed: manifest.Summary.Foreign, Total: manifest.Summary.Foreign, Percent: 1})
	}
	return err
}

func lanePercent(completed, total int) float64 {
	if total <= 0 {
		return 0
	}
	percent := float64(completed) / float64(total)
	if percent > 1 {
		return 1
	}
	return percent
}

func sendSystemSnapshotEvent(events chan tea.Msg, message tea.Msg) {
	select {
	case events <- message:
	default:
		select {
		case <-events:
		default:
		}
		events <- message
	}
}

func waitSystemSnapshotEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func tuiProfile(cfg config.Config, name string) *config.Profile {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			return &cfg.Profiles[i]
		}
	}
	return nil
}
