package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRestoreSplitsFilesAndFullSystem(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeRestore, workspace: "restore", restoreStage: "dashboard"}
	updated, _ := m.updateRestoreWorkspaceKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if m.restoreStage != "full" || !strings.Contains(m.restoreScreen(), "guided interface") {
		t.Fatalf("full-system choice not rendered: %q", m.restoreScreen())
	}
}
