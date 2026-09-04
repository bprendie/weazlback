package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

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
	m.cancel, m.operation = cancel, "Full System Snapshot "+action
	return m, func() tea.Msg {
		service := restic.NewService(io.Discard)
		id, runErr := generation.Execute(ctx, service, repo, cfg.Machine.ID, action, "", func(profile, id string) error {
			return captureTUISystemLane(ctx, cfg, repo, profile, id)
		})
		result := "complete"
		if runErr != nil {
			result = "failed"
		}
		_, auditErr := generation.SaveAudit(v, generation.Audit{GenerationID: id, RepositoryID: destination.RepositoryID, Action: "capture", Result: result})
		if runErr == nil {
			runErr = auditErr
		}
		return systemSnapshotDoneMsg{id: id, err: runErr}
	}
}

func captureTUISystemLane(ctx context.Context, cfg config.Config, repo restic.Repository, profile, id string) error {
	if profile == "generation-ledger" {
		return captureTUILedger(ctx, repo, cfg.Machine.ID, id)
	}
	if profile == "packages" {
		return captureTUIPackages(ctx, cfg, repo, id)
	}
	p := tuiProfile(cfg, profile)
	if p == nil {
		return fmt.Errorf("profile %q is not configured", profile)
	}
	if profile == "heavy" {
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
	return restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, profile, cfg.Machine.ID, []string{generation.TagPrefix + id}, includes, excludes, false, false, nil)
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

func captureTUIPackages(ctx context.Context, cfg config.Config, repo restic.Repository, id string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	staging := filepath.Join(filepath.Dir(path), "staging")
	_, root, cleanup, err := packagecapsule.Capture(packagecapsule.Options{Context: ctx, MachineID: cfg.Machine.ID, StagingRoot: staging, Download: true, Run: packagecapsule.ExecRunner{Context: ctx}})
	if err != nil {
		return err
	}
	defer cleanup()
	return restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, "packages", cfg.Machine.ID, []string{generation.TagPrefix + id}, []string{root}, nil, false, false, nil)
}

func tuiProfile(cfg config.Config, name string) *config.Profile {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			return &cfg.Profiles[i]
		}
	}
	return nil
}
