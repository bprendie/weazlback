package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPackageCapsuleManagementKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("WEAZLBACK_CONFIG", path)
	m := Model{mode: modeProfiles, cfg: config.Default(), styles: newStyles()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = updated.(Model)
	if !m.cfg.PackagePolicy.Scheduled {
		t.Fatal("S did not enable the independent schedule")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = updated.(Model)
	if m.packageStage != "confirm-aur" || !strings.Contains(m.applicationsScreen(), "PKGBUILDs execute code") {
		t.Fatalf("stage=%q view=%q", m.packageStage, m.applicationsScreen())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.packageStage != "" {
		t.Fatalf("AUR confirmation did not cancel: %q", m.packageStage)
	}
}

func TestPackageCapsuleResultIsSanitizedAndVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("WEAZLBACK_CONFIG", path)
	m := Model{mode: modeProfiles, cfg: config.Default(), styles: newStyles(), busy: true, operation: "package capsule"}
	updated, _ := m.Update(packageDoneMsg{manifest: packagecapsule.Manifest{Summary: packagecapsule.Summary{Installed: 10, Captured: 8, Official: 7, Foreign: 3}}})
	m = updated.(Model)
	view := m.applicationsScreen()
	if !strings.Contains(view, "Artifacts captured  8") || !strings.Contains(m.status, "8 artifacts") {
		t.Fatalf("view=%q status=%q", view, m.status)
	}
}
