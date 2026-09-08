package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) restoreRow(value string, selected bool) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " ")
	value = ansi.Truncate(value, max(1, m.width-16), "…")
	if selected {
		return restoreSelection.Render("> " + value)
	}
	return "  " + value
}

func (m Model) restoreViewport(value string, width, height int) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, width), "…")
	}
	if len(lines) > height {
		lines = lines[:max(1, height)]
	}
	return lipgloss.NewStyle().MaxWidth(max(1, width)).Render(strings.Join(lines, "\n"))
}
