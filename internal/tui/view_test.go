package tui

import (
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestConfiguredHomeUsesInitializedVaultWording(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeHome, cfg: config.Default()}
	m.cfg.Destinations = []config.Destination{{ID: "local", Name: "drive"}}
	view := m.screen()
	if !strings.Contains(view, "Sovereign vault initialized") || strings.Contains(view, "no recovery") || strings.Contains(view, "Choose Destinations") {
		t.Fatalf("stale home wording: %q", view)
	}
}

func TestBackupProgressIsIndeterminateUntilTotalsExist(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeBackup, busy: true, progress: restic.BackupProgress{MessageType: "discovery"}}
	view := m.backupScreen()
	if !strings.Contains(view, "calculating totals") || strings.Contains(view, "0%") {
		t.Fatalf("misleading discovery progress: %q", view)
	}
}

func TestBackupProgressDisplaysMeasuredPercentage(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeBackup, busy: true, progress: restic.BackupProgress{
		MessageType: "status", PercentDone: .5, FilesDone: 4, TotalFiles: 8, BytesDone: 10, TotalBytes: 20,
	}}
	view := m.backupScreen()
	if !strings.Contains(view, "50%") || !strings.Contains(view, "4 / 8") {
		t.Fatalf("missing measured progress: %q", view)
	}
}

func TestWideViewUsesFullBanner(t *testing.T) {
	m := New()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 36})
	view := updated.(Model).View()
	if !strings.Contains(view, "____") {
		t.Fatal("wide view did not include banner")
	}
	if lipgloss.Height(view) > 36 {
		t.Fatalf("view height %d exceeds terminal", lipgloss.Height(view))
	}
}

func TestWideBannerUsesGradientAndDiagonalFrames(t *testing.T) {
	loadOmarchyPalette()
	banner := renderLogo(100)
	plain := stripANSIForTest(banner)
	for _, line := range strings.Split(plain, "\n") {
		if !strings.HasPrefix(line, "╱╱╱╱╱╱ ") || !strings.Contains(line, " ╱") {
			t.Fatalf("banner line is not diagonally framed: %q", line)
		}
	}
	stops := gradientStops(accent, secondary, warning, success)
	if sampleGradient(0, stops) == sampleGradient(0.5, stops) || sampleGradient(0.5, stops) == sampleGradient(1, stops) {
		t.Fatal("banner gradient stops collapsed to one color")
	}
}

func stripANSIForTest(value string) string {
	var out strings.Builder
	escape := false
	for _, r := range value {
		if r == '\x1b' {
			escape = true
			continue
		}
		if escape {
			if r == 'm' {
				escape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func TestNarrowViewUsesCompactHeader(t *testing.T) {
	m := New()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	view := updated.(Model).View()
	if strings.Contains(view, "____") {
		t.Fatal("narrow view included full banner")
	}
	headerLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "weazlback") {
			headerLine = line
			break
		}
	}
	if headerLine == "" || lipgloss.Width(strings.TrimSpace(headerLine)) >= 60 {
		t.Fatalf("compact header was not tightened: %q", headerLine)
	}
}

func TestEightyByTwentyFourFitsAndUsesCompactHeader(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeHome, status: "ready"}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(Model).View()
	if strings.Contains(view, "____") {
		t.Fatal("80x24 view used full banner")
	}
	if lipgloss.Height(view) > 24 || lipgloss.Width(view) > 80 {
		t.Fatalf("80x24 rendered %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	if !strings.Contains(view, "BACKUP") || !strings.Contains(view, "Phase 4") {
		t.Fatal("80x24 view did not retain both navigation and active content")
	}
}

func TestResponsiveRailBreakpoints(t *testing.T) {
	for _, width := range []int{76, 80, 89, 100} {
		m := Model{styles: newStyles(), mode: modeHome, width: width, height: 24, status: "ready"}
		view := m.View()
		if !strings.Contains(view, "BACKUP") || !strings.Contains(view, "Phase 4") {
			t.Errorf("width %d did not show rail and content", width)
		}
		if lipgloss.Width(view) > width {
			t.Errorf("width %d rendered %d columns", width, lipgloss.Width(view))
		}
	}
}

func TestViewsRespectReportedTerminalSize(t *testing.T) {
	for _, size := range []struct{ width, height int }{{48, 18}, {60, 24}, {80, 24}, {100, 30}, {140, 42}} {
		m := New()
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		view := updated.(Model).View()
		if lipgloss.Height(view) > size.height || lipgloss.Width(view) > size.width {
			t.Errorf("terminal %dx%d rendered %dx%d", size.width, size.height, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}
