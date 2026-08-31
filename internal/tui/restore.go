package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type restoreBrowseMsg struct {
	snapshots  []restic.Snapshot
	files      []restic.FileEntry
	index      int
	identities []restic.Identity
	catalog    catalog.Catalog
	err        error
}

type restoreCatalogMsg struct {
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
	if m.busy || m.restoreStage == "full" {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.status, m.err = true, "loading machine history and encrypted path catalog", ""
	return m, func() tea.Msg {
		service := restic.NewService(io.Discard)
		snapshots, err := service.Snapshots(context.Background(), repo)
		if err != nil || len(snapshots) == 0 {
			if err == nil {
				err = fmt.Errorf("repository has no restore points")
			}
			return restoreBrowseMsg{err: err}
		}
		sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
		identities := restic.GroupIdentities(snapshots, m.cfg.Machine.ID, m.cfg.Machine.Name)
		machineID := m.cfg.Machine.ID
		if len(restic.FilterIdentity(snapshots, machineID)) == 0 && len(identities) > 0 {
			machineID = identities[0].ID
		}
		filtered := restic.FilterIdentity(snapshots, machineID)
		if len(filtered) == 0 {
			return restoreBrowseMsg{err: fmt.Errorf("selected machine has no restore points")}
		}
		selected := filtered[0]
		index := snapshotIndex(snapshots, selected.ID)
		files, err := service.Files(context.Background(), repo, selected.ID)
		return restoreBrowseMsg{snapshots: snapshots, files: files, index: index, identities: identities, err: err}
	}
}

func loadRestoreCatalog(m Model, destinationID string, service restic.Service, repo restic.Repository, snapshots []restic.Snapshot) (catalog.Catalog, error) {
	cat := catalog.New()
	path, err := catalog.Path(destinationID)
	if err != nil {
		return cat, err
	}
	if loaded, loadErr := catalog.Load(path, m.vault); loadErr == nil {
		cat = loaded
	}
	profiles := map[string]bool{}
	for _, point := range snapshots {
		if restic.MachineID(point.Tags) != "" && restic.Profile(point.Tags) != "" {
			profiles[restic.MachineID(point.Tags)+"\x00"+restic.Profile(point.Tags)] = true
		}
	}
	for pair := range profiles {
		parts := strings.SplitN(pair, "\x00", 2)
		if err := catalog.Update(context.Background(), &cat, service, repo, snapshots, parts[0], parts[1]); err != nil {
			cat = catalog.New()
			for rebuildPair := range profiles {
				rebuild := strings.SplitN(rebuildPair, "\x00", 2)
				if rebuildErr := catalog.Update(context.Background(), &cat, service, repo, snapshots, rebuild[0], rebuild[1]); rebuildErr != nil {
					return cat, fmt.Errorf("catalog rebuild: %w", rebuildErr)
				}
			}
			break
		}
	}
	if err := catalog.Save(path, cat, m.vault); err != nil {
		return cat, err
	}
	return cat, nil
}

func (m Model) loadRestoreCatalogCmd(snapshots []restic.Snapshot) tea.Cmd {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		return func() tea.Msg { return restoreCatalogMsg{err: err} }
	}
	return func() tea.Msg {
		started := time.Now()
		cat, loadErr := loadRestoreCatalog(m, destination.ID, restic.NewService(io.Discard), repo, snapshots)
		return restoreCatalogMsg{catalog: cat, err: loadErr, elapsed: time.Since(started)}
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
