package recoverytui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestGuidedRecoveryDefaultsToCoreHomeScope(t *testing.T) {
	m := New()
	if m.scope != "core-home" {
		t.Fatalf("scope=%q", m.scope)
	}
	m.stage = "scope-choice"
	view := m.View()
	for _, wanted := range []string{"System Set", "Core —", "Core + Home", "Everything"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("scope view missing %q: %s", wanted, view)
		}
	}
	if !strings.Contains(view, "Everything") || !strings.Contains(view, "Nearest") || !strings.Contains(view, "compatible Restore Points") {
		t.Fatalf("Everything scope is ambiguous: %s", view)
	}
}

func TestSystemSetPickerOnlyShowsCompleteAtomicGenerations(t *testing.T) {
	m := New()
	m.stage, m.pointIntent = "system-set-loading", "system-set"
	updated, _ := m.Update(pointsMsg{points: []restic.Snapshot{
		{ID: "complete", Time: time.Unix(200, 0), Tags: []string{"profile:core", "generation:one", generation.TagComplete}},
		{ID: "loose", Time: time.Unix(300, 0), Tags: []string{"profile:core"}},
		{ID: "failed", Time: time.Unix(400, 0), Tags: []string{"profile:core", "generation:two", generation.TagFailed}},
	}})
	m = updated.(Model)
	if m.stage != "point-choice" || len(m.points) != 1 || m.points[0].ID != "complete" {
		t.Fatalf("stage=%q points=%+v", m.stage, m.points)
	}
	if view := m.body(); !strings.Contains(view, "CHOOSE SYSTEM SET") || !strings.Contains(view, "Every lane comes from this exact complete generation") {
		t.Fatalf("view=%q", view)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.scope != "everything" || m.stage != "hostname-choice" {
		t.Fatalf("scope=%q stage=%q", m.scope, m.stage)
	}
}

func TestRecoveryBinaryProvenanceIsVisibleBeforeUnlock(t *testing.T) {
	m := New()
	m.mediaVersion = "1.2.3-test"
	m.stage = "pass"
	if view := m.View(); !strings.Contains(view, "recovery binary 1.2.3-test") {
		t.Fatalf("view=%q", view)
	}
}

func TestPlatformMismatchUsesOneWarningInterstitial(t *testing.T) {
	m := New()
	prepared := &freshrestore.Restore{Plan: freshrestore.Plan{ScopeDecision: freshrestore.ScopeDecision{PlatformMismatch: true, Warning: freshrestore.PlatformMismatchWarning}}}
	updated, _ := m.Update(preparedMsg{restore: prepared})
	m = updated.(Model)
	if m.stage != "compatibility-warning" || strings.Count(m.body(), freshrestore.PlatformMismatchWarning) != 1 {
		t.Fatalf("stage=%q view=%q", m.stage, m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).stage != "plan" {
		t.Fatal("continue did not reach plan")
	}
}

func TestDestinationPickerDefaultsToNewestRecoverableSource(t *testing.T) {
	m := New()
	m.stage = "destination-loading"
	message := destinationsMsg{catalog: freshrestore.RecoveryCatalog{Active: "ssh", Destinations: []config.Destination{
		{ID: "local", Name: "NVMe", Kind: "local", Repository: "/mnt/weazlback"},
		{ID: "ssh", Name: "Remote", Kind: "ssh", Repository: "sftp:user@host:/repo"},
	}, Summaries: map[string]freshrestore.DestinationRecoverySummary{
		"local": {LatestComplete: time.Unix(200, 0)}, "ssh": {LatestComplete: time.Unix(100, 0)},
	}}}
	updated, _ := m.Update(message)
	m = updated.(Model)
	if m.stage != "destination-choice" || m.destinationIndex != 0 || m.destinations[0].ID != "local" {
		t.Fatalf("stage=%q index=%d", m.stage, m.destinationIndex)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.destination != "local" || m.stage != "identity-loading" {
		t.Fatalf("destination=%q stage=%q", m.destination, m.stage)
	}
}

func TestChoosingOlderDestinationRequiresWarningInterstitial(t *testing.T) {
	m := New()
	m.stage = "destination-choice"
	m.destinations = []config.Destination{{ID: "new", Name: "NVMe"}, {ID: "old", Name: "Remote"}}
	m.destinationSummaries = map[string]freshrestore.DestinationRecoverySummary{"new": {LatestComplete: time.Unix(200, 0)}, "old": {LatestComplete: time.Unix(100, 0)}}
	m.destinationIndex = 1
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.stage != "destination-warning" || !strings.Contains(m.View(), "OLDER RECOVERY SOURCE") {
		t.Fatalf("stage=%q view=%q", m.stage, m.View())
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
	plan := freshrestore.Plan{Official: []string{"pac"}, AUR: []string{"aur"}, SystemServices: []string{"svc"}, Applications: &inventory.ApplicationManifest{ManualReview: []string{"inspect"}},
		PackageDelta: packagecapsule.Delta{Local: []packagecapsule.Install{{Name: "capsule", Version: "2"}}}}
	joined := strings.Join(reviewLines(plan), "\n")
	for _, wanted := range []string{"pac", "aur", "svc", "inspect", "capsule", "2"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("missing %s", wanted)
		}
	}
}

func TestRunningRecoveryWithFourLanesFitsEightyByTwentyFour(t *testing.T) {
	m := New()
	m.stage, m.started = "running", time.Now()
	m.width, m.height = 80, 24
	m.filesystemProgress = freshrestore.RestoreProgress{Current: "home", Completed: 50, Total: 100}
	m.packageProgress = freshrestore.RestoreProgress{Current: "artifact", Completed: 20, Total: 40}
	m.appProgress = freshrestore.RestoreProgress{Current: "app", Completed: 3, Total: 10}
	view := m.View()
	if lipgloss.Width(view) > 80 || lipgloss.Height(view) > 24 {
		t.Fatalf("rendered %dx%d\n%s", lipgloss.Width(view), lipgloss.Height(view), view)
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
		BytesDone: 1 << 30, BytesTotal: 2 << 30, BytesPerSecond: 75 << 20, WireBytesPerSecond: 62 << 20, FilesPerSecond: 321.5}
	view := laneView("FILESYSTEM / HOME", progress, false)
	for _, wanted := range []string{"output 75.0 MiB/s", "source 62.0 MiB/s", "321.5 files/s", "50 / 100"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("missing %q in %q", wanted, view)
		}
	}
}

func TestTurboIsExplicitAndSSHChoosesLinkPolicy(t *testing.T) {
	m := New()
	if m.engine != freshrestore.EngineStandard {
		t.Fatalf("default engine=%q", m.engine)
	}
	m.stage, m.destination = "engine-choice", "remote"
	m.destinations = []config.Destination{{ID: "remote", Kind: "ssh"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	if m.stage != "turbo-network" || m.engine != freshrestore.EngineTurbo {
		t.Fatalf("stage=%q engine=%q", m.stage, m.engine)
	}
	if !strings.Contains(m.View(), "Full link") {
		t.Fatal("SSH policy is not disclosed")
	}
}

func TestTurboRequiresDistinctConfirmationPhrase(t *testing.T) {
	m := New()
	m.engine = freshrestore.EngineTurbo
	if got := m.confirmationPhrase(); got != "TURBO RESTORE" {
		t.Fatalf("phrase=%q", got)
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
	for _, value := range []string{"System Set", "Core", "Personality and personal data", "Everything", "Applications only", "Select one file"} {
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
