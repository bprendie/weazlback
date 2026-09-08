package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTreeExpansionParentAndPaging(t *testing.T) {
	m := restoreFixture()
	m.filterRestore()
	m = press(m, tea.KeyRight, "")
	if !m.expanded["/home/alice/Code"] {
		t.Fatal("not expanded")
	}
	m = press(m, tea.KeyDown, "")
	if m.restoreVisible[m.restoreIndex].Name != "main.go" {
		t.Fatal("child missing")
	}
	m = press(m, tea.KeyLeft, "")
	if m.restoreIndex != 0 {
		t.Fatal("did not select parent")
	}
	m = press(m, tea.KeyLeft, "")
	if m.expanded["/home/alice/Code"] {
		t.Fatal("not collapsed")
	}
	for i := 0; i < 1000; i++ {
		m.restoreEntries = append(m.restoreEntries, restic.FileEntry{Path: fmt.Sprintf("/home/alice/file-%04d", i), Type: "file"})
	}
	m.buildRestoreTree()
	m.filterRestoreTree()
	m = press(m, tea.KeyEnd, "")
	if !strings.Contains(m.fileRows(m.restoreVisible), "file-0999") {
		t.Fatal("end not visible")
	}
	m = press(m, tea.KeyPgUp, "")
	if m.restoreIndex != 1000-m.restorePageSize() {
		t.Fatal("paging incorrect")
	}
}

func TestRestoreViewportFitsAndKeepsCursor(t *testing.T) {
	m := restoreFixture()
	for i := 0; i < 1000; i++ {
		m.restoreEntries = append(m.restoreEntries, restic.FileEntry{Path: fmt.Sprintf("/home/alice/%s-%04d", strings.Repeat("長", 45), i), Type: "file"})
	}
	m.filterRestore()
	m.restoreIndex = 700
	for _, size := range [][2]int{{48, 18}, {80, 24}, {160, 45}, {240, 30}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("size %v rendered %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(view, ">") {
			t.Fatalf("cursor invisible at %v", size)
		}
	}
}

func TestRestoreSearchRejectsStaleResponse(t *testing.T) {
	m := restoreFixture()
	m.restoreSearching = true
	m.searchRequest = 2
	m2, _, _ := m.updateRestoreMessage(restoreSearchMsg{request: 1, files: []restic.FileEntry{{Path: "stale"}}})
	if len(m2.(Model).restoreVisible) != 0 {
		t.Fatal("stale search applied")
	}
	m.loadRequest = 3
	m2, _ = m.restoreBrowseFinished(restoreBrowseMsg{request: 2})
	if m2.(Model).restoreStage != "browse" {
		t.Fatal("stale point applied")
	}
}

func TestHomePreferredAndSourcePathMapped(t *testing.T) {
	m := restoreFixture()
	home := m.snapshots[0]
	home.Paths = []string{"/home/alice"}
	packages := home
	packages.ID = "packages"
	packages.Tags = []string{"profile:packages", "machine:alice"}
	packages.Time = home.Time.Add(time.Hour)
	if got := preferredRestorePoint([]restic.Snapshot{packages, home}); got.ID != home.ID {
		t.Fatal("package point selected")
	}
	m.snapshots = []restic.Snapshot{home}
	m.buildRestoreTree()
	if got := m.sourceRestorePath("~/Code"); got != "/home/alice/Code" {
		t.Fatalf("source=%s", got)
	}
	if got := m.sourceRestorePath("~"); got != "/home/alice" {
		t.Fatalf("source=%s", got)
	}
}

func TestPathFallbackOpensHighlightedFile(t *testing.T) {
	m := restoreFixture()
	m.buildRestoreTree()
	m.restoreSearching = true
	m.restoreInput.SetValue("p /home/alice/Code/mai")
	next, cmd := m.queueRestoreSearch()
	m = next.(Model)
	next, _, _ = m.updateRestoreMessage(cmd())
	m = next.(Model)
	if len(m.restoreVisible) != 1 {
		t.Fatalf("matches=%v", m.restoreVisible)
	}
	next, _ = m.applyRestoreSearch()
	m = next.(Model)
	if m.restoreStage != "versions" || m.restoreVersionPath != "/home/alice/Code/main.go" {
		t.Fatal("fallback did not open")
	}
}

func TestVersionScopeDeletionAndOverlap(t *testing.T) {
	m := restoreFixture()
	m.restoreVersionPath = "/home/alice/Code/main.go"
	m.snapshots = append(m.snapshots, restic.Snapshot{ID: "deleted"}, restic.Snapshot{ID: "other"})
	m.restoreEntries = nil
	m.restoreCatalog.Paths[m.restoreVersionPath] = &catalog.PathRecord{Path: m.restoreVersionPath, Versions: []catalog.Version{
		{SnapshotID: "other", MachineID: "bob", Change: "+", Time: time.Now().Add(time.Hour)},
		{SnapshotID: "deleted", MachineID: "alice", Profile: "home", Change: "-", Time: time.Now()},
		{SnapshotID: "point", MachineID: "alice", Profile: "home", Change: "+", Time: time.Now().Add(-time.Hour)},
	}}
	m.chooseRestoreVersion()
	if m.versionIndex != 1 || len(m.restoreBasket) != 0 {
		t.Fatal("deletion must offer preceding version, not restore")
	}
	m.chooseRestoreVersion()
	if m.restoreBasket[m.restoreVersionPath].Snapshot != "point" {
		t.Fatal("incorrect version")
	}
	m.addRestoreBasket(restoreBasketItem{Path: "/home/alice/Code", Snapshot: "other", MachineID: "alice"})
	if m.restoreStage != "basket-conflict" {
		t.Fatal("overlap silently accepted")
	}
	m = press(m, tea.KeyRunes, "n")
	if m.restoreBasket[m.restoreVersionPath].Snapshot != "point" {
		t.Fatal("decline mutated basket")
	}
}

func TestRestoreDateResolution(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.Local)
	d, err := parseRestoreDate("Monday", now)
	if err != nil || d.Day() != 7 {
		t.Fatalf("date=%v err=%v", d, err)
	}
	if _, err := parseRestoreDate("someday", now); err == nil {
		t.Fatal("ambiguous garbage accepted")
	}
}
