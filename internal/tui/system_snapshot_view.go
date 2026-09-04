package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) systemSnapshotScreen() string {
	lines := []string{m.styles.header.Render("FULL SYSTEM SNAPSHOT"), ""}
	compact := m.height > 0 && m.height < 30
	for _, item := range []struct{ key, title string }{
		{"packages", "PACKAGES"}, {"aur", "AUR"}, {"core", "CORE"}, {"home", "HOME"}, {"heavy", "HEAVY"},
	} {
		lines = append(lines, snapshotLaneView(item.title, m.systemSnapshotLanes[item.key], compact))
		if !compact {
			lines = append(lines, "")
		}
	}
	lines = append(lines, "Elapsed "+time.Since(m.systemSnapshotStart).Round(time.Second).String()+"  •  one atomic recovery generation")
	return strings.Join(lines, "\n")
}

func snapshotLaneView(title string, lane systemSnapshotLane, compact bool) string {
	percent := int(lane.Percent * 100)
	if lane.Total > 0 {
		percent = min(100, lane.Completed*100/lane.Total)
	}
	width := 12
	if !compact {
		width = 24
	}
	filled := min(width, max(0, percent*width/100))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	detail := lane.Current
	if lane.Total > 0 {
		detail = fmt.Sprintf("%d/%d  %s", lane.Completed, lane.Total, detail)
	}
	if lane.Rate > 0 {
		detail += "  " + snapshotRate(lane.Rate)
	}
	if detail == "" {
		detail = lane.State
	}
	if compact {
		return fmt.Sprintf("%-8s [%s] %3d%%  %s", title, bar, percent, detail)
	}
	return fmt.Sprintf("%-8s  %s\n[%s] %3d%%  %s", title, strings.ToUpper(lane.State), bar, percent, detail)
}

func snapshotRate(rate float64) string {
	if rate >= 1<<30 {
		return fmt.Sprintf("%.1f GiB/s", rate/(1<<30))
	}
	if rate >= 1<<20 {
		return fmt.Sprintf("%.1f MiB/s", rate/(1<<20))
	}
	if rate >= 1<<10 {
		return fmt.Sprintf("%.1f KiB/s", rate/(1<<10))
	}
	return fmt.Sprintf("%.0f B/s", rate)
}
