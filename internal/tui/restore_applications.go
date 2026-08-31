package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/apprestore"
	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	tea "github.com/charmbracelet/bubbletea"
)

type restoreApplicationPlanMsg struct {
	plan apprestore.Plan
	err  error
}
type restoreApplicationSudoMsg struct{ err error }
type restoreApplicationProgressMsg struct {
	progress apprestore.Progress
	events   <-chan tea.Msg
}
type restoreApplicationDoneMsg struct {
	result  apprestore.Result
	journal string
	err     error
}

func (m Model) loadApplicationPlanCmd() tea.Cmd {
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		return func() tea.Msg { return restoreApplicationPlanMsg{err: err} }
	}
	machineID, snapshots := m.selectedRestoreMachineID(), append([]restic.Snapshot(nil), m.snapshots...)
	requested := time.Now()
	if len(snapshots) > 0 && m.restoreSnapshot < len(snapshots) {
		requested = snapshots[m.restoreSnapshot].Time
	}
	return func() tea.Msg {
		components, composeErr := restoretxn.ComposeNearest(snapshots, machineID, requested, map[restoretxn.Bundle]string{restoretxn.SystemConfig: "core"})
		if composeErr != nil {
			return restoreApplicationPlanMsg{err: composeErr}
		}
		point := components[0].Snapshot
		service := restic.NewService(io.Discard)
		files, loadErr := service.Files(context.Background(), repo, point.ID)
		if loadErr != nil {
			return restoreApplicationPlanMsg{err: loadErr}
		}
		manifestPath := ""
		for _, file := range files {
			if filepath.Base(file.Path) == backupmeta.ManifestName {
				manifestPath = file.Path
				break
			}
		}
		if manifestPath == "" {
			return restoreApplicationPlanMsg{err: fmt.Errorf("selected Core Restore Point has no application manifest")}
		}
		encoded, loadErr := service.Dump(context.Background(), repo, point.ID, manifestPath)
		if loadErr != nil {
			return restoreApplicationPlanMsg{err: loadErr}
		}
		var desired inventory.ApplicationManifest
		if json.Unmarshal(encoded, &desired) != nil || inventory.ValidateApplications(desired) != nil {
			return restoreApplicationPlanMsg{err: fmt.Errorf("selected application manifest is invalid")}
		}
		current, captureErr := inventory.CaptureApplications(context.Background())
		if captureErr != nil {
			return restoreApplicationPlanMsg{err: captureErr}
		}
		availability := apprestore.ResolveManifest(context.Background(), desired, 4)
		plan := apprestore.Build(machineID, point.ID, desired, current, availability)
		plan.Timestamp = point.Time
		return restoreApplicationPlanMsg{plan: plan}
	}
}

func authorizeRestoreApplications() tea.Cmd {
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(command, func(err error) tea.Msg { return restoreApplicationSudoMsg{err} })
}

func (m Model) runApplicationReconciliation() (tea.Model, tea.Cmd) {
	m.busy, m.operation, m.status, m.err = true, "application reconciliation", "installing application intent", ""
	plan, vaultFile := m.restoreAppPlan, m.vault
	events := make(chan tea.Msg, 1)
	go func() {
		result := apprestore.Execute(context.Background(), plan, nil, func(progress apprestore.Progress) {
			sendLatestOperationEvent(events, restoreApplicationProgressMsg{progress: progress, events: events})
		})
		cfgPath, err := config.Path()
		journal := ""
		if err == nil {
			journal = filepath.Join(filepath.Dir(cfgPath), "restore-journals", "applications-"+shortID(plan.Snapshot)+".enc")
			err = apprestore.SaveResult(journal, vaultFile, plan, result)
		}
		sendLatestOperationEvent(events, restoreApplicationDoneMsg{result: result, journal: journal, err: err})
		close(events)
	}()
	return m, waitApplicationEvent(events)
}

func waitApplicationEvent(events <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-events } }
