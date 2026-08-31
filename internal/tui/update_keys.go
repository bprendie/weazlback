package tui

import (
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, model, cmd := m.updateModalKey(msg); handled {
		return model, cmd
	}
	if m.workspace == "restore" {
		return m.updateRestoreWorkspaceKey(msg)
	}
	if m.railFocused && (msg.String() == "r" || msg.String() == "R") {
		if m.busy {
			m.status = "Restore Mode unavailable while backup work is active"
			return m, nil
		}
		m.backupMode, m.backupIndex, m.backupRailFocused = m.mode, m.index, m.railFocused
		m.workspace, m.mode, m.index, m.restoreStage, m.railFocused = "restore", modeRestore, 3, "dashboard", false
		m.status = "Restore Mode"
		return m, nil
	}
	if m.mode == modeNuke && !m.railFocused {
		return m.updateNuke(msg)
	}
	if m.mode == modeTune && !m.railFocused && m.tuneStage != "" {
		return m.updateTuneKey(msg)
	}
	if m.mode == modeRestore && m.restoreStage == "browse" {
		if m.restoreSearching || msg.String() == "[" || msg.String() == "]" {
			return m.updateRestore(msg)
		}
		if msg.String() == "/" {
			m.restoreSearching = true
			m.restoreInput.Focus()
			m.status = "searching restore-point paths"
			return m, textinput.Blink
		}
	}
	if m.mode == modeRestore && m.restoreStage == "full" &&
		(msg.String() == "f" || msg.String() == "x" || msg.String() == "enter" || msg.String() == "esc") {
		return m.updateRestore(msg)
	}
	if m.mode == modeRecovery && m.recoveryStage == "" && !m.railFocused {
		switch msg.String() {
		case "u":
			return m.startRecoveryForm(true)
		case "e":
			return m.startRecoveryForm(false)
		}
	}
	if msg.String() == "tab" || msg.String() == "shift+tab" {
		m.railFocused = msg.String() == "tab" && !m.railFocused
		if m.railFocused {
			m.status = "navigation focused"
		} else {
			m.status = "content focused"
		}
		return m, nil
	}
	// Advertised hot letters always select their menu item, including while the
	// navigation rail is focused. Arrow keys remain the unambiguous rail movers.
	for i, entry := range navigation {
		if entry.mode == modeRestore {
			continue
		}
		if msg.String() == entry.key {
			m.index, m.mode, m.railFocused = i, entry.mode, false
			if m.mode == modeDestinations {
				m.selectActiveDestination()
			}
			return m, nil
		}
	}
	if msg.String() == "D" {
		return m.cycleDestination()
	}
	switch msg.String() {
	case "q":
		if os.Getenv("TMUX") != "" {
			m.status = "detaching — operations continue in the tmux backend"
			return m, detachClientCmd()
		}
		if m.busy {
			m.status = "operation still running — Ctrl+C cancels it"
			return m, nil
		}
		return m, tea.Quit
	case "ctrl+c":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.status = "cancelling " + m.operation
			return m, nil
		}
		return m, tea.Quit
	case "up", "k":
		if m.railFocused {
			m.index = max(0, m.index-1)
		} else if m.mode == modeDestinations {
			m.destinationSelection = max(0, m.destinationSelection-1)
		} else if m.mode == modeRestore && m.restoreStage == "browse" {
			m.restoreIndex = max(0, m.restoreIndex-1)
		}
	case "down", "j":
		if m.railFocused {
			m.index = min(len(navigation)-1, m.index+1)
		} else if m.mode == modeDestinations && len(m.cfg.Destinations) > 0 {
			m.destinationSelection = min(len(m.cfg.Destinations)-1, m.destinationSelection+1)
		} else if m.mode == modeRestore && m.restoreStage == "browse" && len(m.restoreVisible) > 0 {
			m.restoreIndex = min(len(m.restoreVisible)-1, m.restoreIndex+1)
		}
	case "enter":
		return m.activate()
	case "1", "2", "3":
		if m.mode == modeBackup {
			profiles := []string{"core", "home", "heavy"}
			m.selectedProfile = profiles[int(msg.Runes[0]-'1')]
			m.status = "selected " + m.selectedProfile + " profile"
			if m.selectedProfile == "heavy" {
				return m, m.inspectHeavy()
			}
		} else if m.mode == modeDestinations {
			index := int(msg.Runes[0] - '1')
			if index < len(m.cfg.Destinations) {
				m.destinationSelection = index
				return m.activateDestinationSelection()
			}
		}
	case "4", "5", "6", "7", "8", "9":
		if m.mode == modeDestinations {
			index := int(msg.Runes[0] - '1')
			if index < len(m.cfg.Destinations) {
				m.destinationSelection = index
				return m.activateDestinationSelection()
			}
		}
	case "n":
		if m.mode == modeDestinations && !m.railFocused {
			return m.startDestination()
		}
	}
	return m, nil
}

func detachClientCmd() tea.Cmd {
	return func() tea.Msg {
		command := exec.Command("tmux", "detach-client")
		if err := command.Run(); err != nil {
			return detachDoneMsg{err: err}
		}
		return nil
	}
}

func (m Model) updateModalKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	if msg.String() == "?" {
		m.helpVisible = !m.helpVisible
		return true, m, nil
	}
	if m.helpVisible {
		if msg.String() == "esc" {
			m.helpVisible = false
		}
		return true, m, nil
	}
	if m.vaultStage != "" {
		model, cmd := m.updateVault(msg)
		return true, model, cmd
	}
	if m.destinationStage != "" {
		model, cmd := m.updateDestination(msg)
		return true, model, cmd
	}
	if m.recoveryStage != "" {
		model, cmd := m.updateRecovery(msg)
		return true, model, cmd
	}
	if m.sudoPending {
		if msg.String() == "esc" || msg.String() == "s" {
			m.sudoPending, m.incomplete, m.err = false, true, ""
			model, cmd := m.beginEngineBackup()
			return true, model, cmd
		}
		if msg.String() == "enter" {
			return true, m, authorizeSudoCmd()
		}
		return true, m, nil
	}
	return false, m, nil
}

func (m Model) activate() (tea.Model, tea.Cmd) {
	if m.railFocused {
		m.mode, m.railFocused = navigation[m.index].mode, false
		if m.mode == modeDestinations {
			m.selectActiveDestination()
		}
		return m, nil
	}
	switch m.mode {
	case modeDestinations:
		if len(m.cfg.Destinations) == 0 {
			return m.startDestination()
		}
		return m.activateDestinationSelection()
	case modeRecovery:
		return m.startRecoveryForm(true)
	case modeBackup:
		return m.startBackup()
	case modeSnapshots:
		return m.startSnapshots()
	case modeRestore:
		return m.startOrRunRestore()
	case modeProfiles:
		return m.startApplications()
	case modeCheck:
		return m.startCheck()
	case modeTune:
		return m.startTune()
	}
	return m, nil
}
