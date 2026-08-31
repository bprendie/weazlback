package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/vault"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type recoveryDoneMsg struct {
	path     string
	prepared bool
	err      error
}

func (m Model) startRecovery() (tea.Model, tea.Cmd) {
	m.recoveryStage, m.recoveryPrepare, m.recoveryInputs = "choice", false, nil
	m.status, m.err = "choose recovery-kit export or complete USB preparation", ""
	return m, nil
}

func (m Model) startRecoveryForm(prepare bool) (tea.Model, tea.Cmd) {
	pathInput := textinput.New()
	pathInput.Prompt, pathInput.Placeholder = "output .wzrk > ", "/mnt/weazlback/weazlback-recovery.wzrk"
	if prepare {
		pathInput.Prompt, pathInput.Placeholder = "recovery folder > ", "/mnt/WEAZLBACK-RECOVERY"
		pathInput.SetValue("/mnt/WEAZLBACK-RECOVERY")
	}
	pathInput.Focus()
	passInput := textinput.New()
	passInput.Prompt = "vault passphrase > "
	passInput.EchoMode, passInput.EchoCharacter = textinput.EchoPassword, '•'
	m.recoveryInputs, m.recoveryIndex, m.recoveryStage = []textinput.Model{pathInput, passInput}, 0, "form"
	m.recoveryPrepare = prepare
	m.status, m.err = "prepare password-locked recovery media", ""
	if !prepare {
		m.status = "export password-locked recovery kit"
	}
	return m, textinput.Blink
}

func (m Model) updateRecovery(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.recoveryStage, m.recoveryInputs, m.err = "", nil, ""
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	if m.recoveryStage == "choice" {
		switch msg.String() {
		case "u", "enter":
			return m.startRecoveryForm(true)
		case "e":
			return m.startRecoveryForm(false)
		}
		return m, nil
	}
	if msg.String() == "enter" {
		if m.recoveryInputs[m.recoveryIndex].Value() == "" {
			m.err = "path and passphrase must not be empty"
			return m, nil
		}
		if m.recoveryIndex == 0 {
			m.recoveryInputs[0].Blur()
			m.recoveryIndex = 1
			m.recoveryInputs[1].Focus()
			return m, textinput.Blink
		}
		return m.exportRecovery()
	}
	var cmd tea.Cmd
	m.recoveryInputs[m.recoveryIndex], cmd = m.recoveryInputs[m.recoveryIndex].Update(msg)
	return m, cmd
}

func (m Model) exportRecovery() (tea.Model, tea.Cmd) {
	output := strings.TrimSpace(m.recoveryInputs[0].Value())
	passphrase := []byte(m.recoveryInputs[1].Value())
	activeVault := m.cfg.ActiveVault
	cfg := m.cfg
	prepare := m.recoveryPrepare
	m.busy, m.status, m.err = true, "encrypting recovery kit", ""
	return m, func() tea.Msg {
		cfgPath, err := config.Path()
		if err != nil {
			return recoveryDoneMsg{err: err}
		}
		if err = config.Save(cfgPath, cfg); err != nil {
			return recoveryDoneMsg{err: err}
		}
		vaultPath, err := vault.Path(activeVault)
		if err != nil {
			return recoveryDoneMsg{err: err}
		}
		probe := vault.New(vaultPath)
		if err = probe.Unlock(passphrase); err != nil {
			return recoveryDoneMsg{err: err}
		}
		probe.Lock()
		knownHosts := filepath.Join(filepath.Dir(cfgPath), "known_hosts")
		if _, err = os.Stat(knownHosts); err != nil {
			knownHosts = ""
		}
		kitOutput := output
		if prepare {
			info, statErr := os.Stat(output)
			if statErr != nil || !info.IsDir() {
				return recoveryDoneMsg{err: fmt.Errorf("recovery target must be an existing directory")}
			}
			temporary, createErr := os.CreateTemp(output, ".weazlback-recovery-*.wzrk")
			if createErr != nil {
				return recoveryDoneMsg{err: createErr}
			}
			kitOutput = temporary.Name()
			temporary.Close()
			os.Remove(kitOutput)
			defer os.Remove(kitOutput)
		}
		err = recovery.Export(kitOutput, recovery.Sources{Vault: vaultPath, Config: cfgPath, KnownHosts: knownHosts}, passphrase)
		defer func() {
			for i := range passphrase {
				passphrase[i] = 0
			}
		}()
		if err != nil || !prepare {
			return recoveryDoneMsg{path: output, err: err}
		}
		executable, err := os.Executable()
		if err == nil {
			err = recovery.PrepareMedia(output, recovery.MediaSources{Weazlback: executable,
				Restore: filepath.Join(filepath.Dir(executable), "weazlback-restore"), Kit: kitOutput, Restic: commandPath("restic")})
		}
		return recoveryDoneMsg{path: output, prepared: true, err: err}
	}
}

func commandPath(name string) string {
	path, _ := exec.LookPath(name)
	return path
}

func (m Model) recoveryScreen() string {
	body := m.styles.header.Render("RECOVERY KIT") + "\n\n"
	if m.busy {
		return body + m.styles.status.Render("◉ encrypting portable kit…")
	}
	if m.recoveryStage == "" {
		return body + m.styles.selected.Render("u / enter  Prepare USB / recovery folder") +
			"\n\ne          Export .wzrk kit only\n\n" +
			m.styles.help.Render("Preparation preserves unrelated files and never formats media. No passphrase recovery.")
	}
	if m.recoveryStage == "choice" {
		return body + m.styles.selected.Render("u  Prepare USB / recovery folder") +
			"\n\ne  Export .wzrk kit only\n\n" + m.styles.help.Render("Preparation preserves unrelated files and never formats media.")
	}
	for i := range m.recoveryInputs {
		body += m.recoveryInputs[i].View() + "\n"
	}
	body += "\nThe entire kit unlocks only with the vault password."
	if m.err != "" {
		body += "\n\n" + fmt.Sprint(m.err)
	}
	return body
}
