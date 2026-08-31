package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) startExistingLocalFields() (tea.Model, tea.Cmd) {
	path, secret := textinput.New(), textinput.New()
	path.Prompt, path.Placeholder = "existing repository > ", "/mnt/weazlback/repository"
	secret.Prompt, secret.Placeholder = "repository password > ", "required to verify; stored in vault"
	secret.EchoMode, secret.EchoCharacter = textinput.EchoPassword, '•'
	path.Focus()
	m.destinationInputs, m.destinationIndex, m.destinationStage = []textinput.Model{path, secret}, 0, "connect-local"
	m.status, m.err = "connect existing repository — initialization is forbidden", ""
	return m, textinput.Blink
}

func (m Model) connectExistingLocal() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.destinationInputs[0].Value())
	secret := []byte(m.destinationInputs[1].Value())
	vaultFile, cfg := m.vault, m.cfg
	m.busy, m.status, m.err = true, "verifying existing repository identity", ""
	return m, func() tea.Msg {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if info, statErr := os.Stat(absolute); statErr != nil || !info.IsDir() {
			return sshSetupMsg{err: fmt.Errorf("existing repository is not an accessible directory: %s", absolute)}
		}
		repository := restic.Repository{Location: absolute, Password: secret}
		service := restic.NewService(nil)
		repositoryID, err := service.RepositoryID(context.Background(), repository)
		if err != nil {
			return sshSetupMsg{err: fmt.Errorf("verify existing repository without initialization: %w", err)}
		}
		id := fmt.Sprintf("local-%x", randomFour())
		passwordKey := "destination/" + id + "/repository-password"
		if err = vaultFile.Put(passwordKey, secret); err != nil {
			return sshSetupMsg{err: err}
		}
		destination := config.Destination{ID: id, Name: filepath.Base(absolute), Kind: "local", Repository: absolute,
			RepositoryID: repositoryID, PasswordKey: passwordKey}
		cfg.Destinations = append(cfg.Destinations, destination)
		cfg.ActiveDestination = id
		cfgPath, err := config.Path()
		if err == nil {
			err = config.Save(cfgPath, cfg)
		}
		return sshSetupMsg{cfg: cfg, err: err}
	}
}

func (m Model) startLocalField() (tea.Model, tea.Cmd) {
	input := textinput.New()
	input.Prompt = "local repository > "
	input.Placeholder = "/mnt/weazlback/repository"
	input.Focus()
	m.destinationInputs, m.destinationIndex, m.destinationStage = []textinput.Model{input}, 0, "local"
	m.status = "local encrypted destination"
	return m, textinput.Blink
}

func (m Model) setupLocalDestination() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.destinationInputs[0].Value())
	if path == "" {
		m.err = "repository path is required"
		return m, nil
	}
	vaultFile, cfg := m.vault, m.cfg
	m.busy, m.status, m.err = true, "initializing local encrypted repository", ""
	return m, func() tea.Msg {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if err = os.MkdirAll(absolute, 0o700); err != nil {
			return sshSetupMsg{err: err}
		}
		entries, err := os.ReadDir(absolute)
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if len(entries) != 0 {
			return sshSetupMsg{err: fmt.Errorf("create-new repository requires an empty directory: %s", absolute)}
		}
		id := fmt.Sprintf("local-%x", randomFour())
		passwordKey := "destination/" + id + "/repository-password"
		encoded, err := destinationSecret()
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if err = vaultFile.Put(passwordKey, encoded); err != nil {
			return sshSetupMsg{err: err}
		}
		service := restic.NewService(nil)
		repository := restic.Repository{Location: absolute, Password: encoded}
		if err = service.Initialize(context.Background(), repository); err != nil {
			return sshSetupMsg{err: err}
		}
		repositoryID, err := service.RepositoryID(context.Background(), repository)
		if err != nil {
			return sshSetupMsg{err: err}
		}
		destination := config.Destination{ID: id, Name: filepath.Base(absolute), Kind: "local", Repository: absolute, RepositoryID: repositoryID, PasswordKey: passwordKey}
		cfg.Destinations = append(cfg.Destinations, destination)
		cfg.ActiveDestination = destination.ID
		cfgPath, err := config.Path()
		if err != nil {
			return sshSetupMsg{err: err}
		}
		if err = config.Save(cfgPath, cfg); err != nil {
			return sshSetupMsg{err: err}
		}
		return sshSetupMsg{cfg: cfg}
	}
}
