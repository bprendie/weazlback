package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

func restoreFixture() Model {
	return Model{styles: newStyles(), workspace: "restore", mode: modeRestore, restoreStage: "browse", width: 100, height: 30,
		restoreTreePath: "/home/alice", restoreInput: newRestoreInput(), restoreCatalog: catalog.New(), restoreBasket: map[string]restoreBasketItem{},
		snapshots:         []restic.Snapshot{{ID: "point", Time: time.Now(), Tags: []string{"profile:home", "machine:alice"}}},
		restoreIdentities: []restic.Identity{{ID: "alice", Name: "Laptop"}},
		restoreEntries:    []restic.FileEntry{{Path: "/home/alice/Code", Name: "Code", Type: "dir"}, {Path: "/home/alice/Code/main.go", Name: "main.go", Type: "file"}}}
}

func press(m Model, key tea.KeyType, text string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: key, Runes: []rune(text)})
	return next.(Model)
}

func TestRestoreRailEnterUsesWorkspace(t *testing.T) {
	m := restoreFixture()
	m.workspace = "backup"
	m.railFocused = true
	m.index = navigationIndex(modeRestore)
	m = press(m, tea.KeyEnter, "")
	if m.workspace != "restore" || m.restoreStage != "dashboard" {
		t.Fatalf("workspace=%s stage=%s", m.workspace, m.restoreStage)
	}
}

func TestRestoreSearchAcceptsLiteralShortcuts(t *testing.T) {
	m := restoreFixture()
	m = press(m, tea.KeyRunes, "/")
	for _, r := range "qAH?" {
		m = press(m, tea.KeyRunes, string(r))
	}
	if m.restoreInput.Value() != "qAH?" || m.helpVisible || m.restoreAllMachines {
		t.Fatalf("input=%q help=%t", m.restoreInput.Value(), m.helpVisible)
	}
}

func TestRestoreSearchSelectionScrolls(t *testing.T) {
	m := restoreFixture()
	m.restoreSearching = true
	for i := 0; i < 100; i++ {
		m.restoreResults = append(m.restoreResults, catalog.PathRecord{Path: fmt.Sprintf("/file-%03d", i)})
	}
	m.restoreIndex = 75
	if !strings.Contains(m.catalogResultsView(), "> /file-075") {
		t.Fatal("selected search row is offscreen")
	}
}

func TestRestoreVersionCanEnterBasket(t *testing.T) {
	m := restoreFixture()
	m.restoreStage = "versions"
	m.restoreVersionPath = "/home/alice/Code/main.go"
	m.restoreCatalog.Paths[m.restoreVersionPath] = &catalog.PathRecord{Path: m.restoreVersionPath, Versions: []catalog.Version{{MachineID: "alice", SnapshotID: "point", Profile: "home", Time: m.snapshots[0].Time, Change: "+"}}}
	m = press(m, tea.KeyRunes, " ")
	if m.restoreBasket[m.restoreVersionPath].Snapshot != "point" {
		t.Fatal("version cannot be selected")
	}
}
