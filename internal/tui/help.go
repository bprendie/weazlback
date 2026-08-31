package tui

import "strings"

func (m Model) helpScreen() string {
	rows := []string{m.styles.header.Render("WEAZLBACK HELP"), ""}
	for _, entry := range navigation {
		description := strings.ReplaceAll(navigationDescription(entry.mode), "\n", " ")
		rows = append(rows, m.styles.selected.Render(entry.key+"  "+entry.label)+" — "+description)
	}
	rows = append(rows, "",
		m.styles.header.Render("RESTORE POINT"),
		"Immutable Restic file metadata and encrypted deduplicated chunks—not a filesystem snapshot.", "",
		m.styles.help.Render("tab / shift+tab focus panes  •  esc or ? close help"))
	return strings.Join(rows, "\n")
}
