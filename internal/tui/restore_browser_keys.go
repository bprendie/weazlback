package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) restoreSessionKey(key string) (tea.Model, tea.Cmd, bool) {
	if key == "ctrl+c" {
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
	if os.Getenv("TMUX") != "" {
		m.status = "detaching — vault remains in the tmux backend"
		return m, detachClientCmd(), true
	}
	return m, tea.Quit, true
}

func (m Model) updateRestoreBrowserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.restoreSearching, m.restorePathMode, m.restoreDeletedOnly, m.restorePathHistory, m.restoreIndex = true, false, false, false, 0
		m.restoreInput = newRestoreInput()
		m.restoreInput.Prompt = "/ "
		m.restoreInput.Focus()
		m.filterRestore()
		return m, textinput.Blink
	case "up", "k":
		m.restoreIndex = max(0, m.restoreIndex-1)
	case "down", "j":
		m.restoreIndex = min(max(0, len(m.restoreVisible)-1), m.restoreIndex+1)
	case "right", "enter":
		if len(m.restoreVisible) == 0 {
			return m, nil
		}
		entry := m.restoreVisible[m.restoreIndex]
		if entry.Type == "dir" {
			m.restoreTreePath, m.restoreIndex = entry.Path, 0
			m.filterRestore()
		} else {
			m.restoreVersionPath, m.restoreStage = entry.Path, "versions"
		}
	case "left", "backspace":
		parent := filepath.Dir(m.restoreTreePath)
		if parent != m.restoreTreePath {
			m.restoreTreePath, m.restoreIndex = parent, 0
			m.filterRestore()
		}
	case " ":
		m = m.toggleRestoreBasket()
	case "e":
		if len(m.restoreBasket) > 0 {
			m.restoreStage, m.restoreTargetMode, m.status = "transaction-target", "original", "choose restore destination"
		}
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
		m.restoreBasket[path] = restoreBasketItem{Path: path, Snapshot: point.ID, MachineID: restic.IdentityID(point), Profile: restic.Profile(point.Tags), Time: point.Time}
	}
	m.status = restoreBasketStatus(len(m.restoreBasket))
	return m
}

func restoreBasketStatus(count int) string {
	if count == 1 {
		return "1 path selected for restore"
	}
	return fmt.Sprintf("%d paths selected for restore", count)
}

func (m Model) loadSelectedIdentity() (tea.Model, tea.Cmd) {
	if len(m.restoreIdentities) == 0 {
		return m, nil
	}
	id := m.restoreIdentities[m.restoreIdentity].ID
	m.restoreTreePath = ""
	for i, snapshot := range m.snapshots {
		if len(restic.FilterIdentity([]restic.Snapshot{snapshot}, id)) == 1 {
			return m.loadRestorePoint(i)
		}
	}
	return m, nil
}

func (m Model) previousIdentityPoint(direction int) (tea.Model, tea.Cmd) {
	if len(m.restoreIdentities) == 0 || len(m.snapshots) == 0 {
		return m, nil
	}
	id := m.restoreIdentities[m.restoreIdentity].ID
	filtered := restic.FilterIdentity(m.snapshots, id)
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
