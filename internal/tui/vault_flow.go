package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Close() {
	if m.vault != nil {
		m.vault.Lock()
	}
}

func (m Model) updateVault(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.Close()
		return m, tea.Quit
	}
	if msg.String() != "enter" {
		var cmd tea.Cmd
		m.vaultInput, cmd = m.vaultInput.Update(msg)
		return m, cmd
	}
	passphrase := m.vaultInput.Value()
	if passphrase == "" {
		m.err = "passphrase must not be empty"
		return m, nil
	}
	switch m.vaultStage {
	case "create":
		m.pendingPass, m.vaultStage = passphrase, "confirm"
		m.vaultInput.SetValue("")
		m.vaultInput.Prompt = "confirm passphrase > "
		m.status, m.err = "confirm passphrase — there is no recovery", ""
		return m, textinput.Blink
	case "confirm":
		if passphrase != m.pendingPass {
			m.pendingPass, m.vaultStage = "", "create"
			m.vaultInput.SetValue("")
			m.vaultInput.Prompt, m.err = "new vault passphrase > ", "passphrases do not match"
			return m, textinput.Blink
		}
		if err := m.vault.Create([]byte(passphrase)); err != nil {
			m.err = err.Error()
			return m, nil
		}
	case "unlock":
		if err := m.vault.Unlock([]byte(passphrase)); err != nil {
			m.vaultInput.SetValue("")
			m.err = err.Error()
			return m, textinput.Blink
		}
	}
	m.pendingPass = ""
	m.vaultInput.SetValue("")
	m.vaultInput.Blur()
	m.vaultStage = ""
	m.status, m.err = "vault unlocked / session owns key", ""
	return m, nil
}
