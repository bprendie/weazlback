package tui

import (
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTabFocusesRailAndEnterActivatesSelection(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeHome}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if !m.railFocused {
		t.Fatal("tab did not focus navigation rail")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.index != 1 || m.mode != modeHome {
		t.Fatalf("rail selection changed content prematurely: index=%d mode=%d", m.index, m.mode)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.railFocused || m.mode != modeBackup {
		t.Fatalf("enter did not activate content: focus=%v mode=%d", m.railFocused, m.mode)
	}
}

func TestEveryAdvertisedNavigationHotkeyWorksFromContent(t *testing.T) {
	for i, entry := range navigation {
		if entry.mode == modeRestore {
			continue
		}
		m := Model{styles: newStyles(), mode: modeHome}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(entry.key)})
		got := updated.(Model)
		if got.mode != entry.mode || got.index != i {
			t.Errorf("hotkey %q selected mode=%d index=%d; want mode=%d index=%d", entry.key, got.mode, got.index, entry.mode, i)
		}
	}
}

func TestEveryAdvertisedNavigationHotkeyWorksFromFocusedRail(t *testing.T) {
	for i, entry := range navigation {
		m := Model{styles: newStyles(), mode: modeHome, railFocused: true}
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(entry.key)})
		got := updated.(Model)
		if entry.mode == modeRestore {
			if got.workspace != "restore" || got.mode != modeRestore || got.index != i || got.railFocused {
				t.Errorf("focused restore hotkey selected workspace=%q mode=%d index=%d focus=%v", got.workspace, got.mode, got.index, got.railFocused)
			}
			continue
		}
		if got.mode != entry.mode || got.index != i || got.railFocused {
			t.Errorf("focused hotkey %q selected mode=%d index=%d focus=%v", entry.key, got.mode, got.index, got.railFocused)
		}
	}
}

func TestDestinationPickerStartsOnActiveAndCanSwitch(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := Model{styles: newStyles(), mode: modeHome, cfg: config.Default()}
	m.cfg.Destinations = []config.Destination{{ID: "ssh", Name: "remote"}, {ID: "local", Name: "drive"}}
	m.cfg.ActiveDestination = "local"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if m.destinationSelection != 1 {
		t.Fatalf("selection=%d, want active index 1", m.destinationSelection)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.cfg.ActiveDestination != "ssh" {
		t.Fatalf("active=%q", m.cfg.ActiveDestination)
	}
	loaded, err := config.Load(mustConfigPath(t))
	if err != nil || loaded.ActiveDestination != "ssh" {
		t.Fatalf("persisted active=%q err=%v", loaded.ActiveDestination, err)
	}
}

func TestDestinationPickerCanAddWhenConfigured(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeDestinations, cfg: config.Default()}
	m.cfg.Destinations = []config.Destination{{ID: "ssh", Name: "remote"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if updated.(Model).destinationStage != "choose" {
		t.Fatal("n did not open add-destination flow")
	}
}

func TestDestinationQuickCycleAndNumberSelection(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := Model{styles: newStyles(), mode: modeHome, cfg: config.Default()}
	m.cfg.Destinations = []config.Destination{{ID: "ssh", Name: "remote"}, {ID: "local", Name: "drive"}}
	m.cfg.ActiveDestination = "ssh"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m = updated.(Model)
	if m.cfg.ActiveDestination != "local" {
		t.Fatalf("cycle active=%q", m.cfg.ActiveDestination)
	}
	m.mode = modeDestinations
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(Model)
	if m.cfg.ActiveDestination != "ssh" {
		t.Fatalf("number active=%q", m.cfg.ActiveDestination)
	}
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestShiftTabReturnsToContent(t *testing.T) {
	m := Model{styles: newStyles(), railFocused: true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if updated.(Model).railFocused {
		t.Fatal("shift+tab did not return focus to content")
	}
}

func TestHelpExplainsEveryNavigationItem(t *testing.T) {
	m := Model{styles: newStyles(), helpVisible: true}
	view := m.helpScreen()
	for _, entry := range navigation {
		if !strings.Contains(view, entry.label) || navigationDescription(entry.mode) == "" {
			t.Fatalf("missing help for %s", entry.label)
		}
	}
	if !strings.Contains(view, "not a filesystem snapshot") {
		t.Fatal("restore point definition is ambiguous")
	}
}

func TestFullSystemSnapshotIsPrimaryDedicatedMenuItem(t *testing.T) {
	index := navigationIndex(modeSystemSnapshot)
	if index != navigationIndex(modeBackup)+1 {
		t.Fatalf("snapshot index=%d backup index=%d", index, navigationIndex(modeBackup))
	}
	entry := navigation[index]
	if entry.key != "y" || entry.label != "Full System Snapshot" {
		t.Fatalf("entry=%+v", entry)
	}
	m := Model{styles: newStyles(), width: 80, height: 24, mode: modeHome, index: 0, railFocused: true, cfg: config.Default()}
	if view := m.View(); !strings.Contains(view, "y System set") {
		t.Fatalf("primary item hidden at 80x24:\n%s", view)
	}
}

func TestNarrowRailAppearsWhenFocused(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeHome, railFocused: true}
	view := m.content(60, 18)
	if !strings.Contains(view, "BACKUP") || !strings.Contains(view, "Restore Points") {
		t.Fatal("focused rail not visible at narrow width")
	}
}

func TestQDetachesWithoutCancellingTmuxBackup(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux,1,0")
	cancelled := false
	m := Model{styles: newStyles(), mode: modeBackup, busy: true, cancel: func() { cancelled = true }}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(Model)
	if cancelled || cmd == nil || !strings.Contains(got.status, "operations continue") {
		t.Fatalf("q cancelled=%v cmd=%v status=%q", cancelled, cmd != nil, got.status)
	}
}

func TestCtrlCCancelsBusyOperation(t *testing.T) {
	cancelled := false
	m := Model{styles: newStyles(), mode: modeBackup, busy: true, operation: "backup", cancel: func() { cancelled = true }}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !cancelled || updated.(Model).status != "cancelling backup" {
		t.Fatal("Ctrl+C did not explicitly cancel the backup")
	}
}

func TestRestoreSearchDoesNotStealGlobalShortcutsUntilSlash(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeRestore, restoreStage: "browse", restoreInput: newRestoreInput()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if updated.(Model).mode != modeHome {
		t.Fatal("restore browser stole global home shortcut")
	}
	m.mode = modeRestore
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)
	if m.mode != modeRestore || m.restoreInput.Value() != "h" {
		t.Fatal("active restore search did not capture text")
	}
}
