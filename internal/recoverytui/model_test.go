package recoverytui

import (
	"os"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestGuidedRecoveryDefaultsToHomeScope(t *testing.T) {
	m := New()
	if m.scope != "home" {
		t.Fatalf("scope=%q", m.scope)
	}
	m.stage = "scope-choice"
	view := m.View()
	for _, wanted := range []string{"Core", "Home", "Everything"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("scope view missing %q: %s", wanted, view)
		}
	}
}

func TestDestinationPickerDefaultsToKitActiveAndSelectsAnother(t *testing.T) {
	m := New()
	m.stage = "destination-loading"
	message := destinationsMsg{catalog: freshrestore.RecoveryCatalog{Active: "ssh", Destinations: []config.Destination{
		{ID: "local", Name: "NVMe", Kind: "local", Repository: "/mnt/weazlback"},
		{ID: "ssh", Name: "Remote", Kind: "ssh", Repository: "sftp:user@host:/repo"},
	}}}
	updated, _ := m.Update(message)
	m = updated.(Model)
	if m.stage != "destination-choice" || m.destinationIndex != 1 {
		t.Fatalf("stage=%q index=%d", m.stage, m.destinationIndex)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.destination != "local" || m.stage != "identity-loading" {
		t.Fatalf("destination=%q stage=%q", m.destination, m.stage)
	}
}

func TestViewFitsEightyByTwentyFour(t *testing.T) {
	m := New()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(Model).View()
	if lipgloss.Width(view) > 80 || lipgloss.Height(view) > 24 {
		t.Fatalf("rendered %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestReviewIncludesEveryQueueAndManualItem(t *testing.T) {
	plan := freshrestore.Plan{Official: []string{"pac"}, AUR: []string{"aur"}, SystemServices: []string{"svc"}, Applications: &inventory.ApplicationManifest{ManualReview: []string{"inspect"}}}
	joined := strings.Join(reviewLines(plan), "\n")
	for _, wanted := range []string{"pac", "aur", "svc", "inspect"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("missing %s", wanted)
		}
	}
}

func TestProgressUsesResolvedQueueDenominator(t *testing.T) {
	m := Model{appProgress: freshrestore.RestoreProgress{Phase: "applications", Lane: "AUR", Current: "pkg", Completed: 79, Total: 145}}
	view := m.progressView()
	if !strings.Contains(view, "79 / 145") || !strings.Contains(view, "54%") {
		t.Fatalf("progress=%q", view)
	}
}

func TestFilesystemProgressShowsByteAndFileSpeed(t *testing.T) {
	progress := freshrestore.RestoreProgress{Phase: "filesystem", Lane: "Home", Current: "extracting", Completed: 50, Total: 100,
		BytesDone: 1 << 30, BytesTotal: 2 << 30, BytesPerSecond: 75 << 20, FilesPerSecond: 321.5}
	view := laneView("FILESYSTEM / HOME", progress, false)
	for _, wanted := range []string{"75.0 MiB/s", "321.5 files/s", "50 / 100"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("missing %q in %q", wanted, view)
		}
	}
}

func TestMissingKitReturnsToEditableKitField(t *testing.T) {
	m := New()
	m.kit = "/missing/recovery.wzrkx"
	m.stage = "loading"
	updated, _ := m.Update(preparedMsg{err: &os.PathError{Op: "open", Path: m.kit, Err: os.ErrNotExist}})
	got := updated.(Model)
	if got.stage != "kit" || got.input.Value() != m.kit {
		t.Fatalf("stage=%q input=%q", got.stage, got.input.Value())
	}
}

func TestRecoveryWorkspaceSeparatesSourceTargetIdentityAndActions(t *testing.T) {
	m := New()
	m.machineID = "11111111111111111111111111111111"
	m.stage = "target-identity"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.stage != "selective-points" || m.targetMachineID == m.machineID || !m.persistTargetIdentity {
		t.Fatalf("stage=%q source=%q target=%q persist=%v", m.stage, m.machineID, m.targetMachineID, m.persistTargetIdentity)
	}
	updated, _ = m.Update(pointsMsg{points: []restic.Snapshot{{ID: "core", ShortID: "core", Tags: []string{"profile:core", "healthy"}}}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	view := m.View()
	for _, value := range []string{"System Config", "Personal Files", "Everything", "Applications only", "Select one file"} {
		if !strings.Contains(view, value) {
			t.Fatalf("action workspace missing %q", value)
		}
	}
}

func TestSelectiveBrowserFuzzyFiltersWithoutCatalog(t *testing.T) {
	m := New()
	m.files = []restic.FileEntry{{Path: "/home/bob/Pictures/wallpapers/cyberpunk.png"}, {Path: "/home/bob/Documents/tax.pdf"}}
	m.visibleFiles = append([]restic.FileEntry(nil), m.files...)
	m.input.SetValue("pwall")
	m.filterFiles()
	if len(m.visibleFiles) != 1 || !strings.Contains(m.visibleFiles[0].Path, "cyberpunk") {
		t.Fatalf("visible=%#v", m.visibleFiles)
	}
}
