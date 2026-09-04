package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/generation"
)

func systemSnapshotSetList(sets []generation.Generation, height int) string {
	limit := 8
	if height > 0 && height < 30 {
		limit = 4
	}
	lines := []string{"RECOVERY GENERATIONS"}
	for index, set := range sets {
		if index >= limit {
			lines = append(lines, fmt.Sprintf("… %d older", len(sets)-limit))
			break
		}
		state := "INCOMPLETE"
		switch {
		case set.Abandoned:
			state = "ABANDONED"
		case set.Failed:
			state = "FAILED / RETRYABLE"
		case set.Complete:
			state = "COMPLETE"
		}
		lines = append(lines, fmt.Sprintf("%s  %-18s  %d/4 lanes", set.StartedAt.Local().Format("2006-01-02 15:04"), state, generationLaneCount(set)))
	}
	return strings.Join(lines, "\n")
}

func generationLaneCount(set generation.Generation) int {
	count := 0
	for _, profile := range generation.RequiredProfiles {
		if _, ok := set.Members[profile]; ok {
			count++
		}
	}
	return count
}

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
