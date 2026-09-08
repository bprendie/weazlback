package tui

import "fmt"

// Keep the next action above the scrolling list, including on small terminals.
func (m Model) restoreSelectionAction() string {
	if len(m.restoreBasket) == 0 {
		return ""
	}
	key := "r"
	if m.restoreSearching && m.restoreStage == "browse" {
		key = "Ctrl+E"
	}
	return m.restoreRow(fmt.Sprintf("[%s] Restore selected (%d)", key, len(m.restoreBasket)), true) + "\n"
}
