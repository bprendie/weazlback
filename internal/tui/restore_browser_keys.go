package tui

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) restoreSessionKey(key string) (tea.Model, tea.Cmd, bool) {
	if key == "ctrl+c" {
		if m.loadCancel != nil {
			m.loadCancel()
		}
		if m.searchCancel != nil {
			m.searchCancel()
		}
		if m.catalogCancel != nil {
			m.catalogCancel()
		}
		if m.busy && m.cancel != nil {
			m.cancel()
			m.status = "cancelling " + m.operation
			return m, nil, true
		}
		return m, tea.Quit, true
	}
	if key != "q" {
		return m, nil, false
	}
	if !m.restoreTextFocused() {
		if m.loadCancel != nil {
			m.loadCancel()
		}
		m.loadRequest++
		if m.searchCancel != nil {
			m.searchCancel()
		}
		m.searchRequest++
		if m.catalogCancel != nil {
			m.catalogCancel()
		}
		m.catalogRequest++
	}
	if os.Getenv("TMUX") != "" {
		m.status = "detaching — vault remains in the tmux backend"
		return m, detachClientCmd(), true
	}
	return m, tea.Quit, true
}

func (m Model) updateRestoreBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.browserIndex = m.restoreIndex
		m.restoreSearching, m.restorePathMode, m.restoreDeletedOnly, m.restorePathHistory, m.restoreIndex = true, false, false, false, 0
		m.restoreInput = newRestoreInput()
		m.restoreInput.Prompt = "/ "
		m.restoreInput.Focus()
		next, search := m.queueRestoreSearch()
		return next, tea.Batch(textinput.Blink, search)
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
		m.restoreIndex = moveList(m.restoreIndex, len(m.restoreVisible), m.restorePageSize(), msg.String())
	case "right", "enter":
		m.moveRestoreTree(true)
	case "left", "backspace":
		m.moveRestoreTree(false)
	case " ":
		m = m.toggleRestoreBasket()
	case "e", "r", "ctrl+e":
		m.openRestoreBasket()
	case "t":
		m.openRestorePoints()
	case "ctrl+r":
		return m.refreshRestoreHistory()
	case "[":
		return m.previousIdentityPoint(1)
	case "]":
		return m.previousIdentityPoint(-1)
	case "i":
		if len(m.restoreIdentities) > 1 {
			m.restoreIdentity = (m.restoreIdentity + 1) % len(m.restoreIdentities)
			return m.loadSelectedIdentity()
		}
	case "esc":
		m.restoreStage, m.status = "dashboard", "Restore Mode"
	}
	return m, nil
}

func (m Model) toggleRestoreBasket() Model {
	if len(m.restoreVisible) == 0 {
		return m
	}
	path := m.restoreVisible[m.restoreIndex].Path
	if _, selected := m.restoreBasket[path]; selected {
		delete(m.restoreBasket, path)
	} else if len(m.snapshots) > 0 && m.restoreSnapshot < len(m.snapshots) {
		point := m.snapshots[m.restoreSnapshot]
		m.addRestoreBasket(restoreBasketItem{Path: path, Snapshot: point.ID, MachineID: restic.IdentityID(point), Profile: restic.Profile(point.Tags), Time: point.Time})
	}
	m.status = restoreBasketStatus(len(m.restoreBasket))
	return m
}

func restoreBasketStatus(count int) string {
	if count == 1 {
		return "1 path selected — press r to restore"
	}
	return fmt.Sprintf("%d paths selected — press r to restore", count)
}

func (m Model) loadSelectedIdentity() (tea.Model, tea.Cmd) {
	if len(m.restoreIdentities) == 0 {
		return m, nil
	}
	id := m.restoreIdentities[m.restoreIdentity].ID
	m.restoreTreePath = ""
	m.searchScope, m.restoreSearching, m.restoreAllMachines = "", false, false
	point := preferredRestorePoint(restic.FilterIdentity(m.snapshots, id))
	if point.ID != "" {
		return m.loadRestorePoint(snapshotIndex(m.snapshots, point.ID))
	}
	return m, nil
}

func (m Model) previousIdentityPoint(direction int) (tea.Model, tea.Cmd) {
	if len(m.restoreIdentities) == 0 || len(m.snapshots) == 0 {
		return m, nil
	}
	id := m.restoreIdentities[m.restoreIdentity].ID
	filtered := restic.FilterIdentity(m.snapshots, id)
	profile := restic.Profile(m.snapshots[m.restoreSnapshot].Tags)
	points := make([]restic.Snapshot, 0, len(filtered))
	for _, p := range filtered {
		if restic.Profile(p.Tags) == profile {
			points = append(points, p)
		}
	}
	filtered = points
	if len(filtered) == 0 {
		return m, nil
	}
	current := 0
	for i := range filtered {
		if filtered[i].ID == m.snapshots[m.restoreSnapshot].ID {
			current = i
		}
	}
	current = max(0, min(len(filtered)-1, current+direction))
	return m.loadRestorePoint(snapshotIndex(m.snapshots, filtered[current].ID))
}

func (m Model) loadRestoreEntry(snapshotID string) tea.Cmd {
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		return func() tea.Msg { return restoreBrowseMsg{err: err} }
	}
	return func() tea.Msg {
		files, loadErr := restic.NewService(io.Discard).Files(context.Background(), repo, snapshotID)
		return restoreBrowseMsg{snapshots: m.snapshots, files: files, index: snapshotIndex(m.snapshots, snapshotID), err: loadErr}
	}
}
