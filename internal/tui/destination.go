package tui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/sshsetup"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type sshProbeMsg struct {
	fingerprint, hostLine string
	err                   error
}
type sshSetupMsg struct {
	cfg config.Config
	err error
}

func (m Model) startDestination() (tea.Model, tea.Cmd) {
	m.destinationStage = "choose"
	m.status, m.err = "create new or connect existing encrypted storage", ""
	return m, nil
}

func (m *Model) selectActiveDestination() {
	active := m.cfg.Active()
	if active == nil {
		m.destinationSelection = 0
		return
	}
	for i := range m.cfg.Destinations {
		if m.cfg.Destinations[i].ID == active.ID {
			m.destinationSelection = i
			return
		}
	}
}

func (m Model) activateDestinationSelection() (tea.Model, tea.Cmd) {
	if m.busy {
		m.status = "active operation keeps its current destination"
		return m, nil
	}
	if len(m.cfg.Destinations) == 0 {
		return m.startDestination()
	}
	if m.destinationSelection < 0 || m.destinationSelection >= len(m.cfg.Destinations) {
		m.destinationSelection = 0
	}
	destination := m.cfg.Destinations[m.destinationSelection]
	m.cfg.ActiveDestination = destination.ID
	path, err := config.Path()
	if err == nil {
		err = config.Save(path, m.cfg)
	}
	if err != nil {
		m.err, m.status = err.Error(), "could not switch destination"
		return m, nil
	}
	m.err, m.status = "", "active destination: "+destination.Name
	return m, nil
}

func (m Model) cycleDestination() (tea.Model, tea.Cmd) {
	if m.busy {
		m.status = "active operation keeps its current destination"
		return m, nil
	}
	if len(m.cfg.Destinations) < 2 {
		m.status = "configure another destination before switching"
		return m, nil
	}
	m.selectActiveDestination()
	m.destinationSelection = (m.destinationSelection + 1) % len(m.cfg.Destinations)
	return m.activateDestinationSelection()
}

func (m Model) startSSHFields() (tea.Model, tea.Cmd) {
	placeholders := []string{"hostname", "setup username", "password"}
	m.destinationInputs = make([]textinput.Model, len(placeholders))
	for i, placeholder := range placeholders {
		input := textinput.New()
		input.Placeholder, input.Prompt = placeholder, placeholder+" > "
		if i == 2 {
			input.EchoMode, input.EchoCharacter = textinput.EchoPassword, '•'
		}
		m.destinationInputs[i] = input
	}
	m.destinationInputs[0].Focus()
	m.destinationIndex, m.destinationStage = 0, "credentials"
	m.status, m.err = "SSH destination setup", ""
	return m, textinput.Blink
}

func (m Model) updateDestination(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.destinationStage, m.destinationInputs, m.err = "", nil, ""
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	if m.destinationStage == "choose" {
		if msg.String() == "s" || msg.String() == "enter" {
			return m.startSSHFields()
		}
		if msg.String() == "l" {
			return m.startLocalField()
		}
		if msg.String() == "c" {
			return m.startExistingLocalFields()
		}
		return m, nil
	}
	if m.destinationStage == "local" {
		if msg.String() == "enter" {
			return m.setupLocalDestination()
		}
		var cmd tea.Cmd
		m.destinationInputs[0], cmd = m.destinationInputs[0].Update(msg)
		return m, cmd
	}
	if m.destinationStage == "connect-local" {
		if msg.String() == "enter" {
			if strings.TrimSpace(m.destinationInputs[m.destinationIndex].Value()) == "" {
				m.err = "repository path and repository password are required"
				return m, nil
			}
			if m.destinationIndex == 0 {
				m.destinationInputs[0].Blur()
				m.destinationIndex = 1
				m.destinationInputs[1].Focus()
				return m, textinput.Blink
			}
			return m.connectExistingLocal()
		}
		var cmd tea.Cmd
		m.destinationInputs[m.destinationIndex], cmd = m.destinationInputs[m.destinationIndex].Update(msg)
		return m, cmd
	}
	if m.destinationStage == "confirm-host" {
		if msg.String() == "enter" {
			return m.bootstrapDestination()
		}
		return m, nil
	}
	if msg.String() == "enter" {
		if strings.TrimSpace(m.destinationInputs[m.destinationIndex].Value()) == "" {
			m.err = "all fields are required"
			return m, nil
		}
		if m.destinationIndex < len(m.destinationInputs)-1 {
			m.destinationInputs[m.destinationIndex].Blur()
			m.destinationIndex++
			m.destinationInputs[m.destinationIndex].Focus()
			return m, textinput.Blink
		}
		host := strings.TrimSpace(m.destinationInputs[0].Value())
		m.busy, m.status, m.err = true, "probing SSH host key", ""
		return m, func() tea.Msg {
			fingerprint, line, err := sshsetup.Probe(context.Background(), host, 22)
			return sshProbeMsg{fingerprint: fingerprint, hostLine: line, err: err}
		}
	}
	var cmd tea.Cmd
	m.destinationInputs[m.destinationIndex], cmd = m.destinationInputs[m.destinationIndex].Update(msg)
	return m, cmd
}

func (m Model) bootstrapDestination() (tea.Model, tea.Cmd) {
	host := strings.TrimSpace(m.destinationInputs[0].Value())
	user := strings.TrimSpace(m.destinationInputs[1].Value())
	password := m.destinationInputs[2].Value()
	fingerprint, vaultFile, cfg := m.sshFingerprint, m.vault, m.cfg
	m.busy, m.status, m.err = true, "installing restricted SSH credentials", ""
	return m, func() tea.Msg {
		hostname, _ := os.Hostname()
		result, err := sshsetup.Bootstrap(context.Background(), sshsetup.Target{Host: host, User: user, Password: password}, fingerprint, destinationID(hostname))
		if err != nil {
			return sshSetupMsg{err: err}
		}
		id := fmt.Sprintf("ssh-%x", randomFour())
		passwordKey, sshKeyKey := "destination/"+id+"/repository-password", "destination/"+id+"/private-key"
		encoded, err := destinationSecret()
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if err = vaultFile.Put(passwordKey, encoded); err != nil {
			return sshSetupMsg{err: err}
		}
		if err = vaultFile.Put(sshKeyKey, result.PrivateKey); err != nil {
			return sshSetupMsg{err: err}
		}
		cfgPath, err := config.Path()
		if err != nil {
			return sshSetupMsg{err: err}
		}
		knownHosts := filepath.Join(filepath.Dir(cfgPath), "known_hosts")
		if err = os.MkdirAll(filepath.Dir(knownHosts), 0o700); err != nil {
			return sshSetupMsg{err: err}
		}
		if err = os.WriteFile(knownHosts, []byte(result.KnownHostLine+"\n"), 0o600); err != nil {
			return sshSetupMsg{err: err}
		}
		destination := config.Destination{ID: id, Name: host, Kind: "ssh", Repository: result.Repository,
			PasswordKey: passwordKey, SSHKeyKey: sshKeyKey, SSHKnownHosts: knownHosts}
		repo := restic.Repository{Location: result.Repository, Password: encoded, SSHKey: result.PrivateKey, KnownHosts: knownHosts}
		service := restic.NewService(nil)
		if err = service.Initialize(context.Background(), repo); err != nil {
			return sshSetupMsg{err: err}
		}
		repositoryID, err := service.RepositoryID(context.Background(), repo)
		if err != nil {
			return sshSetupMsg{err: err}
		}
		destination.RepositoryID = repositoryID
		cfg.Destinations = append(cfg.Destinations, destination)
		cfg.ActiveDestination = destination.ID
		if err = config.Save(cfgPath, cfg); err != nil {
			return sshSetupMsg{err: err}
		}
		return sshSetupMsg{cfg: cfg}
	}
}

func destinationID(hostname string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(hostname) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		value = "machine"
	}
	return fmt.Sprintf("%s-%x", value, randomFour())
}

func randomFour() []byte { value := make([]byte, 4); _, _ = rand.Read(value); return value }

func destinationSecret() ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	for i := range raw {
		raw[i] = 0
	}
	return encoded, nil
}
