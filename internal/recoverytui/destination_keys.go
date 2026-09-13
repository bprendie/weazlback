package recoverytui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) updateDestinationChoiceKey(key string) (tea.Model, tea.Cmd) {
	if key == "up" || key == "k" {
		m.destinationIndex = max(0, m.destinationIndex-1)
	}
	if key == "down" || key == "j" {
		m.destinationIndex = min(len(m.destinations)-1, m.destinationIndex+1)
	}
	if key != "enter" || len(m.destinations) == 0 {
		return m, nil
	}
	m.destination = m.destinations[m.destinationIndex].ID
	chosen := m.destinationSummaries[m.destination]
	if chosen.Error != "" {
		m.err, m.stage = chosen.Error, "history-access"
		return m, nil
	}
	if m.destinationIndex > 0 && !recoveryDestinationTime(chosen).IsZero() && recoveryDestinationTime(m.destinationSummaries[m.destinations[0].ID]).After(recoveryDestinationTime(chosen)) {
		m.stage = "destination-warning"
		return m, nil
	}
	m.stage = "identity-loading"
	return m, m.loadIdentities()
}

func (m Model) updateDestinationWarningKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.stage = "destination-choice"
		return m, nil
	}
	if key == "enter" {
		m.stage = "identity-loading"
		return m, m.loadIdentities()
	}
	return m, nil
}
