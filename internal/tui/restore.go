package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type restoreBrowseMsg struct {
	tree       *restoreNavigation
	request    uint64
	snapshots  []restic.Snapshot
	files      []restic.FileEntry
	index      int
	identities []restic.Identity
	catalog    catalog.Catalog
	err        error
}

type restoreCatalogMsg struct {
	request uint64
	catalog catalog.Catalog
	err     error
	elapsed time.Duration
}
type restoreLiveHintsMsg struct {
	hints []string
	err   error
}

type fullRestoreDoneMsg struct{ err error }

func (m Model) startOrRunRestore() (tea.Model, tea.Cmd) {
	m.loadSpecific = false
	if m.busy || m.restoreStage == "full" {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.status, m.err = true, "loading machine history and encrypted path catalog", ""
	if repo.Elevated && !m.restoreAuthorized {
		m.busy = false
		m.status = "Authorize repository access with your system password"
		return m, authorizeRestoreLoad(0, false)
	}
	m.restoreStage = "load"
	m.restoreCatalog = catalog.New()
	if m.loadCancel != nil {
		m.loadCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.loadCancel = cancel
	m.loadRequest++
	request := m.loadRequest
	return m, func() tea.Msg {
		defer cancel()
		service := restic.NewService(io.Discard)
		snapshots, err := service.Snapshots(ctx, repo)
		if err != nil || len(snapshots) == 0 {
			if err == nil {
				err = fmt.Errorf("repository has no restore points")
			}
			return restoreBrowseMsg{request: request, err: err}
		}
		sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
		identities := restic.GroupIdentities(snapshots, m.cfg.Machine.ID, m.cfg.Machine.Name)
		machineID := m.cfg.Machine.ID
		if len(restic.FilterIdentity(snapshots, machineID)) == 0 && len(identities) > 0 {
			machineID = identities[0].ID
		}
		filtered := restic.FilterIdentity(snapshots, machineID)
		if len(filtered) == 0 {
			return restoreBrowseMsg{request: request, err: fmt.Errorf("selected machine has no restore points")}
		}
		selected := preferredRestorePoint(filtered)
		if selected.ID == "" {
			return restoreBrowseMsg{request: request, err: fmt.Errorf("selected machine has no file restore points")}
		}
		index := snapshotIndex(snapshots, selected.ID)
		files, err := service.Files(ctx, repo, selected.ID)
		return restoreBrowseMsg{tree: prepareRestoreTree(files), request: request, snapshots: snapshots, files: files, index: index, identities: identities, err: err}
	}
}

func snapshotIndex(snapshots []restic.Snapshot, id string) int {
	for i := range snapshots {
		if snapshots[i].ID == id {
			return i
		}
	}
	return 0
}

func (m Model) updateRestore(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.restoreStage == "full" {
		if msg.String() == "enter" {
			binary, err := exec.LookPath("weazlback-restore")
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			command := exec.Command(binary)
			command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
			return m, tea.ExecProcess(command, func(err error) tea.Msg { return fullRestoreDoneMsg{err} })
		}
		if msg.String() == "esc" {
			m.restoreStage, m.status = "dashboard", "Restore Mode"
		}
		return m, nil
	}
	if msg.String() == "esc" && m.restoreSearching {
		m.restoreSearching = false
		m.restoreInput.Blur()
		m.status = "restore-point browser"
		return m, nil
	}
	if msg.String() == "enter" && m.restoreSearching {
		return m.applyRestoreSearch()
	}
	if m.restoreSearching && (msg.String() == "up" || msg.String() == "ctrl+p") {
		m.restoreIndex = max(0, m.restoreIndex-1)
		return m, nil
	}
	if m.restoreSearching && (msg.String() == "down" || msg.String() == "ctrl+n") {
		count := len(m.restoreResults)
		if count == 0 {
			count = len(m.restoreVisible)
		}
		m.restoreIndex = min(max(0, count-1), m.restoreIndex+1)
		return m, nil
	}
	if !m.restoreSearching {
		return m, nil
	}
	var cmd tea.Cmd
	m.restoreInput, cmd = m.restoreInput.Update(msg)
	m.filterRestore()
	return m, cmd
}

func newRestoreInput() textinput.Model {
	input := textinput.New()
	input.Prompt, input.Placeholder = "/", "filename, p ~/path, or c text"
	input.Blur()
	return input
}
