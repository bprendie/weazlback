package tui

import "github.com/charmbracelet/lipgloss"

var (
	accent     = lipgloss.Color("#7aa2f7")
	secondary  = lipgloss.Color("#bb9af7")
	success    = lipgloss.Color("#9ece6a")
	warning    = lipgloss.Color("#e0af68")
	foreground = lipgloss.Color("#c0caf5")
	muted      = lipgloss.Color("#565f89")
	border     = lipgloss.Color("#3b4261")
	canvas     = lipgloss.Color("#1a1b26")
	panel      = lipgloss.Color("#16161e")
)

type styles struct {
	frame, header, panel, active, focused lipgloss.Style
	status, help, selected, item          lipgloss.Style
}

func newStyles() styles {
	loadOmarchyPalette()
	return styles{
		frame:  lipgloss.NewStyle().Foreground(foreground).Background(canvas).Padding(1, 2),
		header: lipgloss.NewStyle().Foreground(accent).Bold(true),
		panel: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).
			Background(panel).Padding(0, 1),
		active: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(secondary).
			Background(panel).Padding(0, 1),
		focused: lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(accent).
			Background(panel).Padding(0, 1),
		status:   lipgloss.NewStyle().Foreground(success).Bold(true),
		help:     lipgloss.NewStyle().Foreground(muted),
		selected: lipgloss.NewStyle().Foreground(accent).Bold(true),
		item:     lipgloss.NewStyle().Foreground(foreground),
	}
}
