package recoverytui

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" || key == "q" && m.stage != "pass" && m.stage != "confirm" {
		m.close()
		return m, tea.Quit
	}
	switch m.stage {
	case "kit-choice":
		if key == "up" || key == "k" {
			m.kitIndex = max(0, m.kitIndex-1)
		} else if key == "down" || key == "j" {
			m.kitIndex = min(len(m.kits)-1, m.kitIndex+1)
		} else if key == "enter" {
			m.kit = m.kits[m.kitIndex]
			m.input.SetValue(m.kit)
			m.stage = "kit"
		}
		return m, nil
	case "kit":
		if key == "enter" {
			m.kit = strings.TrimSpace(m.input.Value())
			m.input = textinput.New()
			m.input.Prompt = "vault passphrase > "
			m.input.EchoMode, m.input.EchoCharacter = textinput.EchoPassword, '•'
			m.input.Focus()
			m.stage = "pass"
			return m, textinput.Blink
		}
	case "pass":
		if key == "enter" && m.input.Value() != "" {
			m.passphrase = []byte(m.input.Value())
			m.input.SetValue("")
			m.stage = "destination-loading"
			return m, m.loadDestinations()
		}
	case "destination-choice":
		return m.updateDestinationChoiceKey(key)
	case "destination-warning":
		return m.updateDestinationWarningKey(key)
	case "identity-choice":
		if key == "up" || key == "k" {
			m.identityIndex = max(0, m.identityIndex-1)
		} else if key == "down" || key == "j" {
			m.identityIndex = min(len(m.identities)-1, m.identityIndex+1)
		} else if key == "enter" && len(m.identities) > 0 {
			m.machineID, m.stage = m.identities[m.identityIndex].ID, "target-identity"
		}
		return m, nil
	case "target-identity":
		if key == "c" || key == "enter" {
			m = m.chooseTargetIdentity("current")
			m.stage, m.pointIntent = "selective-points", "action"
			return m, m.loadPoints("core")
		} else if key == "n" {
			m = m.chooseTargetIdentity("new")
			m.stage, m.pointIntent = "selective-points", "action"
			return m, m.loadPoints("core")
		} else if key == "a" {
			m.adoptIdentity, m.targetIdentityMode, m.targetMachineID = true, "adopt", m.machineID
			m.stage, m.pointIntent = "selective-points", "action"
			return m, m.loadPoints("core")
		}
		return m, nil
	case "point-choice":
		if key == "up" || key == "k" {
			m.pointIndex = max(0, m.pointIndex-1)
		} else if key == "down" || key == "j" {
			m.pointIndex = min(len(m.points)-1, m.pointIndex+1)
		} else if key == "enter" {
			m.stage = "action-choice"
		}
		return m, nil
	case "action-choice":
		switch key {
		case "c":
			m.scope, m.stage = "core", "hostname-choice"
		case "h", "enter":
			m.scope, m.stage = "core-home", "hostname-choice"
		case "e":
			m.scope, m.stage = "everything", "hostname-choice"
		case "a":
			m.scope, m.hostname, m.stage = "applications", "current", "loading"
			return m, m.prepare(false)
		case "f":
			m.stage, m.pointIntent = "selective-points", "selective"
			return m, m.loadPoints("")
		case "i":
			m.stage = "catalog-loading"
			return m, m.rebuildCatalog()
		}
		return m, nil
	case "scope-choice":
		if key == "a" {
			m.adoptIdentity = !m.adoptIdentity
		} else if key == "enter" || key == "h" {
			m.scope, m.stage = "core-home", "hostname-choice"
		} else if key == "c" {
			m.scope, m.stage = "core", "hostname-choice"
		} else if key == "e" {
			m.scope, m.stage = "everything", "hostname-choice"
		}
		return m, nil
	case "selective":
		if key == "esc" {
			m.stage = "action-choice"
			return m, nil
		}
		if key == "up" || key == "ctrl+k" {
			m.fileIndex = max(0, m.fileIndex-1)
			return m, nil
		}
		if key == "down" || key == "ctrl+j" {
			m.fileIndex = min(max(0, len(m.visibleFiles)-1), m.fileIndex+1)
			return m, nil
		}
		if key == "[" || key == "]" {
			if key == "[" {
				m.pointIndex = min(len(m.points)-1, m.pointIndex+1)
			} else {
				m.pointIndex = max(0, m.pointIndex-1)
			}
			m.stage = "selective-loading"
			return m, m.loadPointFiles()
		}
		if key == "enter" && len(m.visibleFiles) > 0 {
			m.selectedPath = m.visibleFiles[m.fileIndex].Path
			m.input = textinput.New()
			m.input.Prompt = "type RESTORE > "
			m.input.Focus()
			m.stage = "selective-confirm"
			return m, textinput.Blink
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.filterFiles()
		return m, cmd
	case "selective-confirm":
		if key == "esc" {
			m.stage = "selective"
			return m, nil
		}
		if key == "enter" {
			if m.input.Value() != "RESTORE" {
				m.err = "confirmation must be exactly RESTORE"
				return m, nil
			}
			m.stage = "selective-running"
			return m, m.executeSelective()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case "selective-done":
		if key == "enter" || key == "esc" {
			m.stage, m.err = "action-choice", ""
		}
		return m, nil
	case "hostname-choice":
		if key == "enter" || key == "o" {
			m.hostname, m.stage = "original", "engine-choice"
			return m, nil
		}
		if key == "c" {
			m.hostname, m.stage = "current", "engine-choice"
			return m, nil
		}
		if key == "n" {
			m.input = textinput.New()
			m.input.Prompt = "new hostname > "
			m.input.Focus()
			m.stage = "hostname-custom"
			return m, textinput.Blink
		}
	case "hostname-custom":
		if key == "enter" && strings.TrimSpace(m.input.Value()) != "" {
			m.hostname, m.stage = strings.TrimSpace(m.input.Value()), "engine-choice"
			return m, nil
		}
	case "engine-choice":
		if key == "enter" || key == "s" {
			m.engine, m.stage = "standard", "loading"
			return m, m.prepare(false)
		}
		if key == "t" {
			m.engine = "turbo"
			if m.destinationIsSSH() {
				m.stage = "turbo-network"
				return m, nil
			}
			m.stage = "loading"
			return m, m.prepare(false)
		}
		return m, nil
	case "turbo-network":
		if key == "enter" || key == "r" {
			m.turboFullLink, m.stage = false, "loading"
			return m, m.prepare(false)
		}
		if key == "f" {
			m.turboFullLink, m.stage = true, "loading"
			return m, m.prepare(false)
		}
		return m, nil
	case "access":
		if key == "a" {
			return m.authorizeAdoption()
		}
		if key == "o" {
			m.input = textinput.New()
			m.input.Prompt = "local repository > "
			m.input.Focus()
			m.stage = "override"
			return m, textinput.Blink
		}
		if key == "r" {
			m.stage = "loading"
			return m, m.prepare(false)
		}
	case "override":
		if key == "enter" {
			m.repository = strings.TrimSpace(m.input.Value())
			m.stage = "loading"
			return m, m.prepare(false)
		}
	case "plan":
		if key == "up" || key == "k" {
			m.reviewIndex = max(0, m.reviewIndex-1)
		} else if key == "down" || key == "j" {
			m.reviewIndex = min(max(0, len(m.review)-1), m.reviewIndex+1)
		} else if key == "b" && m.engine == "turbo" && len(m.restore.Journal.Qualification.HardFailures) == 0 && len(m.restore.Journal.Qualification.SoftFindings) > 0 {
			m.restore.Close()
			m.restore, m.turboBreakGlass, m.stage = nil, true, "loading"
			return m, m.prepare(false)
		} else if key == "enter" {
			m.input = textinput.New()
			m.input.Prompt = "type " + m.confirmationPhrase() + " > "
			m.input.Focus()
			m.stage = "confirm"
			return m, textinput.Blink
		}
		return m, nil
	case "compatibility-warning":
		if key == "enter" {
			m.stage = "plan"
			return m, nil
		}
		if key == "esc" {
			m.close()
			return m, tea.Quit
		}
		return m, nil
	case "confirm":
		if key == "enter" {
			if m.input.Value() != m.confirmationPhrase() {
				m.err = "confirmation must be exactly " + m.confirmationPhrase()
				return m, nil
			}
			m.stage = "authorizing"
			cmd := exec.Command("sudo", "-v")
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return executeSudoMsg{err} })
		}
	case "done":
		if key == "enter" || key == "q" {
			m.close()
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) confirmationPhrase() string {
	if m.engine == "turbo" {
		return "TURBO RESTORE"
	}
	return "RESTORE"
}
