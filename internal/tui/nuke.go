package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type nukeDoneMsg struct{ err error }

func (m Model) nukeScreen() string {
	body := m.styles.header.Render("BREAK GLASS / NUKE REPOSITORY") + "\n\n"
	destination := m.cfg.Active()
	if destination == nil {
		return body + "No active repository is configured."
	}
	if m.busy {
		return body + m.styles.status.Render("Destroying the exact confirmed repository…")
	}
	switch m.nukeStage {
	case "passphrase":
		return body + "Re-enter the vault passphrase:\n\n" + m.nukeInput.View() + "\n\n" + m.styles.help.Render("esc abort")
	case "confirm":
		return body + "Type exactly:\n\n" + m.styles.status.Render("NUKE "+destination.ID) + "\n\n" + m.nukeInput.View() + "\n\n" + m.styles.help.Render("esc abort")
	}
	mode := "DELETE REPOSITORY + DESTROY KEYS (default)"
	if m.nukeMode == "keys" {
		mode = "DESTROY KEYS / LEAVE CIPHERTEXT"
	}
	body += "Repository  " + destination.ID + "\nLocation    " + destination.Repository + "\nMode        " + mode
	if destination.Kind == "local" && m.nukeMode == "full" {
		local := "preserve empty repository directory"
		if m.nukeRemoveDirectory {
			local = "remove exact repository directory"
		}
		body += "\nLocal       " + local
	}
	body += "\n\nThis is cryptographic destruction, not guaranteed physical secure erase."
	body += "\nCorruption does not block this failsafe. Active work must terminate first."
	body += "\n\n[1] full deletion  [2] keys only  [d] local directory  [enter] continue"
	if m.err != "" {
		body += "\n\n" + m.styles.status.Render(m.err)
	}
	return body
}

func (m Model) updateNuke(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	if m.nukeStage == "" {
		switch msg.String() {
		case "1":
			m.nukeMode = "full"
		case "2":
			m.nukeMode = "keys"
		case "d":
			if active := m.cfg.Active(); active != nil && active.Kind == "local" {
				m.nukeRemoveDirectory = !m.nukeRemoveDirectory
			}
		case "enter":
			m.nukeStage = "passphrase"
			m.nukeInput = textinput.New()
			m.nukeInput.EchoMode, m.nukeInput.EchoCharacter = textinput.EchoPassword, '•'
			m.nukeInput.Prompt = "vault passphrase > "
			m.nukeInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}
	if msg.String() == "esc" {
		m.nukeStage, m.nukePass, m.err = "", "", ""
		return m, nil
	}
	if msg.String() == "enter" {
		if m.nukeStage == "passphrase" {
			if m.nukeInput.Value() == "" {
				m.err = "Vault passphrase is required"
				return m, nil
			}
			m.nukePass, m.nukeStage = m.nukeInput.Value(), "confirm"
			m.nukeInput = textinput.New()
			m.nukeInput.Prompt = "confirmation > "
			m.nukeInput.Focus()
			return m, textinput.Blink
		}
		active := m.cfg.Active()
		if active == nil || m.nukeInput.Value() != "NUKE "+active.ID {
			m.err = "Confirmation did not match"
			return m, nil
		}
		pass, confirmation := m.nukePass, m.nukeInput.Value()
		m.nukePass, m.busy, m.status = "", true, "break-glass destruction running"
		return m, runNukeCmd(active.ID, m.nukeMode, m.nukeRemoveDirectory, pass, confirmation)
	}
	var cmd tea.Cmd
	m.nukeInput, cmd = m.nukeInput.Update(msg)
	return m, cmd
}

func runNukeCmd(destination, mode string, removeDirectory bool, passphrase, confirmation string) tea.Cmd {
	return func() tea.Msg {
		executable, err := os.Executable()
		if err != nil {
			return nukeDoneMsg{err}
		}
		args := []string{"internal-nuke", "--destination", destination, "--mode", mode}
		if removeDirectory {
			args = append(args, "--remove-directory")
		}
		command := exec.Command(executable, args...)
		command.Stdin = strings.NewReader(passphrase + "\n" + confirmation + "\n")
		var stderr bytes.Buffer
		command.Stdout, command.Stderr = nil, &stderr
		if err := command.Run(); err != nil {
			return nukeDoneMsg{fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))}
		}
		return nukeDoneMsg{}
	}
}
