package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"testing"
)

// Opt-in real-terminal drill; the model uses fixture metadata and no live vault.
type restoreTerminalFixture struct{ model Model }

func (m restoreTerminalFixture) Init() tea.Cmd { return nil }
func (m restoreTerminalFixture) View() string  { return m.model.View() }
func (m restoreTerminalFixture) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.model.Update(msg)
	m.model = next.(Model)
	return m, cmd
}

func TestRestoreTerminalSmoke(t *testing.T) {
	if os.Getenv("WEAZLBACK_TUI_SMOKE") != "1" {
		t.Skip("requires an interactive terminal")
	}
	t.Setenv("TMUX", "")
	m := restoreFixture()
	m.filterRestore()
	result, err := tea.NewProgram(restoreTerminalFixture{model: m}, tea.WithAltScreen()).Run()
	if err != nil {
		t.Fatal(err)
	}
	if result.(restoreTerminalFixture).model.restoreStage != "versions" {
		t.Fatal("navigate into Code/main.go before quitting")
	}
}
