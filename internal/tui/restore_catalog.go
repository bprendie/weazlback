package tui

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) loadRestoreCatalogCmd(snapshots []restic.Snapshot) tea.Cmd {
	destination, _, _, err := m.activeRuntime("")
	if err != nil {
		return func() tea.Msg { return restoreCatalogMsg{err: err} }
	}
	m.catalogRequest++
	request := m.catalogRequest
	v := m.vault
	return func() tea.Msg {
		started := time.Now()
		path, err := catalog.Path(destination.ID)
		if err != nil {
			return restoreCatalogMsg{request: request, err: err}
		}
		c, err := catalog.Load(path, v)
		return restoreCatalogMsg{request: request, catalog: c, err: err, elapsed: time.Since(started)}
	}
}

// Refresh is explicit and bounded to the visible machine/profile. Browsing never
// writes the catalog: the backup process owns its persistent incremental cache.
func (m Model) refreshRestoreHistory() (tea.Model, tea.Cmd) {
	if len(m.snapshots) == 0 {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.catalogCancel != nil {
		m.catalogCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	m.catalogCancel = cancel
	m.catalogRequest++
	request := m.catalogRequest
	point := m.snapshots[m.restoreSnapshot]
	snapshots := m.snapshots
	m.restoreCatalogState = "Refreshing selected machine/profile history (30s budget)…"
	return m, func() tea.Msg {
		defer cancel()
		started := time.Now()
		c := catalog.New()
		err := catalog.Update(ctx, &c, restic.NewService(io.Discard), repo, snapshots, restic.MachineID(point.Tags), restic.Profile(point.Tags))
		if err != nil {
			err = fmt.Errorf("history refresh: %w; selected-point browsing remains available", err)
		}
		return restoreCatalogMsg{request: request, catalog: c, err: err, elapsed: time.Since(started)}
	}
}
