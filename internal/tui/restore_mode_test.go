package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRestoreModeOnlyEntersFromFocusedRailAndReturnsFromDashboard(t *testing.T) {
	m := Model{styles: newStyles(), workspace: "backup", mode: modeTune, index: 8}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if updated.(Model).workspace != "backup" {
		t.Fatal("content hotkey entered Restore Mode")
	}
	m.railFocused = true
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if m.workspace != "restore" || m.restoreStage != "dashboard" {
		t.Fatalf("workspace=%q stage=%q", m.workspace, m.restoreStage)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	returned := updated.(Model)
	if returned.workspace != "backup" || returned.mode != modeTune || returned.index != 8 || !returned.railFocused {
		t.Fatalf("backup state was not preserved: mode=%v index=%d rail=%v", returned.mode, returned.index, returned.railFocused)
	}
}

func TestBundleChoicesAreThreeFocusedPresets(t *testing.T) {
	m := Model{styles: newStyles(), restoreBundleChoices: map[restoretxn.Bundle]bool{}}
	updated, _ := m.updateBundleComponentsKey("h")
	m = updated.(Model)
	if !m.restoreBundleChoices[restoretxn.SystemConfig] || !m.restoreBundleChoices[restoretxn.PersonalFiles] || m.restoreBundleChoices[restoretxn.HeavyData] {
		t.Fatalf("Core + Home preset=%v", m.restoreBundleChoices)
	}
	updated, _ = m.updateBundleComponentsKey("e")
	m = updated.(Model)
	if !m.restoreBundleChoices[restoretxn.SystemConfig] || !m.restoreBundleChoices[restoretxn.PersonalFiles] || !m.restoreBundleChoices[restoretxn.HeavyData] {
		t.Fatalf("Everything preset=%v", m.restoreBundleChoices)
	}
}

func TestBundlePlatformMismatchUsesSharedWarning(t *testing.T) {
	m := Model{styles: newStyles(), restoreBasket: map[string]restoreBasketItem{"/home/me/Documents": {}}, restoreStage: "bundle-components"}
	decision := freshrestore.ScopeDecision{PlatformMismatch: true, Warning: freshrestore.PlatformMismatchWarning}
	updated, _, handled := m.updateRestoreMessage(bundlePreparedMsg{decision: decision, basket: m.restoreBasket})
	m = updated.(Model)
	if !handled || m.restoreStage != "bundle-compatibility-warning" || strings.Count(m.restoreScreen(), freshrestore.PlatformMismatchWarning) != 1 {
		t.Fatalf("stage=%q", m.restoreStage)
	}
}

func TestPersonalBundleExcludesCoreButKeepsSiblingBranches(t *testing.T) {
	cfg := config.Config{Profiles: []config.Profile{{Name: "core", Includes: []string{"/home/bob/.config", "/home/bob/.local/bin"}}}}
	files := []restic.FileEntry{
		{Path: "/home/bob/Pictures"}, {Path: "/home/bob/Pictures/a.png"},
		{Path: "/home/bob/.config"}, {Path: "/home/bob/.config/app"},
		{Path: "/home/bob/.local"}, {Path: "/home/bob/.local/bin"}, {Path: "/home/bob/.local/state"},
	}
	paths := personalBundlePaths(files, cfg)
	joined := strings.Join(paths, "\n")
	if strings.Contains(joined, ".config") || strings.Contains(joined, ".local/bin") {
		t.Fatalf("personal bundle leaked core boundaries: %v", paths)
	}
	if !strings.Contains(joined, "/home/bob/Pictures") || !strings.Contains(joined, "/home/bob/.local/state") {
		t.Fatalf("personal bundle lost independent branches: %v", paths)
	}
}

func TestWorkspaceSwitchesAreBlockedDuringOperations(t *testing.T) {
	m := Model{styles: newStyles(), workspace: "backup", mode: modeHome, railFocused: true, busy: true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if got := updated.(Model); got.workspace != "backup" || !strings.Contains(got.status, "unavailable") {
		t.Fatalf("backup switch result=%#v", got)
	}
	m = Model{styles: newStyles(), workspace: "restore", mode: modeRestore, restoreStage: "dashboard", busy: true}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if got := updated.(Model); got.workspace != "restore" || !strings.Contains(got.status, "unavailable") {
		t.Fatalf("restore switch result=%#v", got)
	}
}

func TestRestoreTreeNavigationAndDeduplicatedVersionSearch(t *testing.T) {
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	m := Model{styles: newStyles(), workspace: "restore", mode: modeRestore, restoreStage: "browse",
		cfg: config.Config{Machine: config.Machine{ID: machine}}, restoreBasket: map[string]restoreBasketItem{}, restoreTreePath: "/home/alice",
		restoreEntries:    []restic.FileEntry{{Path: "/home/alice/Code", Name: "Code", Type: "dir"}, {Path: "/home/alice/Code/app/main.go", Name: "main.go", Type: "file", Size: 10}},
		restoreIdentities: []restic.Identity{{ID: machine, Name: "ThinkPad"}}, restoreCatalog: catalog.New()}
	m.restoreCatalog.Paths["/home/alice/Code/app/main.go"] = &catalog.PathRecord{Path: "/home/alice/Code/app/main.go", Versions: []catalog.Version{
		{MachineID: machine, SnapshotID: "new", Time: time.Now()}, {MachineID: machine, SnapshotID: "old", Time: time.Now().Add(-time.Hour)},
	}}
	m.filterRestore()
	// Exercise the public Bubble Tea update path: Restore Mode must receive
	// navigation keys before the backup workspace can consume them.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if m.restoreTreePath != "/home/alice/Code" {
		t.Fatalf("tree path=%q", m.restoreTreePath)
	}
	updated, _ = m.updateRestoreWorkspaceKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	m.restoreInput.SetValue("maingo")
	m.filterRestore()
	if len(m.restoreResults) != 1 || len(m.restoreResults[0].Versions) != 2 {
		t.Fatalf("results=%#v", m.restoreResults)
	}
	if !strings.Contains(m.catalogResultsView(), "main.go") {
		t.Fatal("search result not rendered")
	}
}

func TestContentSearchRequiresExplicitSessionUnlock(t *testing.T) {
	m := Model{styles: newStyles(), workspace: "restore", restoreStage: "browse", restoreSearching: true, restoreInput: newRestoreInput()}
	m.restoreInput.SetValue("c API_TOKEN")
	updated, _ := m.applyRestoreSearch()
	m = updated.(Model)
	if m.restoreStage != "content-confirm" || m.restoreContentQuery != "API_TOKEN" || m.restoreContentOpen {
		t.Fatalf("stage=%q query=%q unlocked=%v", m.restoreStage, m.restoreContentQuery, m.restoreContentOpen)
	}
	if !strings.Contains(m.restoreScreen(), "No content index") {
		t.Fatal("content-search privacy warning missing")
	}
}

func TestDeletedSinceSelectedPointIsRelative(t *testing.T) {
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cutoff := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	c := catalog.New()
	c.Paths["/home/alice/gone.txt"] = &catalog.PathRecord{Path: "/home/alice/gone.txt", Versions: []catalog.Version{
		{MachineID: machine, Change: "-", Time: cutoff.Add(time.Hour)},
		{MachineID: machine, Change: "+", Time: cutoff.Add(-time.Hour)},
	}}
	c.Paths["/home/alice/already-gone.txt"] = &catalog.PathRecord{Path: "/home/alice/already-gone.txt", Versions: []catalog.Version{
		{MachineID: machine, Change: "-", Time: cutoff.Add(-time.Hour)},
	}}
	results := deletedCatalogResults(c, []restic.Identity{{ID: machine}}, 0, cutoff)
	if len(results) != 1 || results[0].Path != "/home/alice/gone.txt" {
		t.Fatalf("results=%#v", results)
	}
}

func TestRestoreTreeRendersLargeDirectoriesAtResponsiveSizes(t *testing.T) {
	m := Model{styles: newStyles(), workspace: "restore", mode: modeRestore, restoreStage: "browse",
		restoreBasket: map[string]restoreBasketItem{}, restoreTreePath: "/home/alice", restoreCatalog: catalog.New()}
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("file-%04d", i)
		m.restoreEntries = append(m.restoreEntries, restic.FileEntry{Path: "/home/alice/" + name, Name: name, Type: "file"})
	}
	m.filterRestore()
	for _, size := range [][2]int{{48, 18}, {80, 24}, {160, 45}, {240, 30}} {
		m.width, m.height = size[0], size[1]
		if view := m.View(); view == "" || !strings.Contains(view, "RESTORE MODE") {
			t.Fatalf("empty restore view at %dx%d", size[0], size[1])
		}
	}
}
