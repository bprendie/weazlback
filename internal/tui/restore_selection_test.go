package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRestoreSelectedAction(t *testing.T) {
	for _, stage := range []string{"browse", "versions"} {
		for _, size := range [][2]int{{48, 18}, {80, 24}, {160, 45}} {
			m := restoreFixture()
			m.width, m.height = size[0], size[1]
			m.restoreStage = stage
			m.restoreBasket["/file"] = restoreBasketItem{Path: "/file", Snapshot: "point"}
			if !strings.Contains(m.View(), "[r] Restore selected (1)") {
				t.Fatalf("missing visible action in %s at %v", stage, size)
			}
			m = press(m, tea.KeyRunes, "r")
			if m.restoreStage != "basket" {
				t.Fatalf("r did not open selection review: %s", m.restoreStage)
			}
			if !strings.Contains(m.View(), "Enter: Choose destination") {
				t.Fatal("destination action not visible")
			}
			m = press(m, tea.KeyEnter, "")
			if m.restoreStage != "transaction-target" || m.busy {
				t.Fatal("must choose destination before executing")
			}
		}
	}
}

func TestRestoreSelectedFromSearch(t *testing.T) {
	m := restoreFixture()
	m.restoreSearching = true
	m.restoreInput.Focus()
	m.restoreBasket["/file"] = restoreBasketItem{Path: "/file", Snapshot: "point"}
	if !strings.Contains(m.View(), "[Ctrl+E] Restore selected (1)") {
		t.Fatal("search restore action not visible")
	}
	m = press(m, tea.KeyRunes, "r")
	if m.restoreInput.Value() != "r" || m.restoreStage != "browse" {
		t.Fatal("restore shortcut stole search text")
	}
	m = press(m, tea.KeyCtrlE, "")
	if m.restoreStage != "basket" {
		t.Fatal("Ctrl+E did not open review")
	}
}
