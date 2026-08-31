package tui

import (
	"fmt"
	"strings"
)

func lineHeight(value string) int { return strings.Count(value, "\n") + 1 }
func (m Model) title() string {
	for _, entry := range navigation {
		if entry.mode == m.mode {
			return strings.ToUpper(entry.label)
		}
	}
	return "HOME"
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func formatBytes(value int64) string {
	if value == 0 {
		return "--"
	}
	return fmt.Sprintf("%.1f GiB", float64(value)/(1<<30))
}
