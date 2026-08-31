package tui

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestConnectExistingLocalVerifiesWithoutReinitializing(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic is not installed")
	}
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repository")
	password := []byte("existing-secret")
	service := restic.NewService(io.Discard)
	repo := restic.Repository{Location: repoPath, Password: password}
	if err := service.Initialize(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	before, err := service.RepositoryID(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEAZLBACK_HOME", filepath.Join(dir, "state"))
	cfg := config.Default()
	v := vault.New(filepath.Join(dir, "current.vault"))
	if err := v.Create([]byte("vault-pass")); err != nil {
		t.Fatal(err)
	}
	m := Model{cfg: cfg, vault: v}
	started, _ := m.startExistingLocalFields()
	m = started.(Model)
	m.destinationInputs[0].SetValue(repoPath)
	m.destinationInputs[1].SetValue(string(password))
	_, command := m.connectExistingLocal()
	message := command()
	setup, ok := message.(sshSetupMsg)
	if !ok || setup.err != nil {
		t.Fatalf("message=%#v", message)
	}
	after, err := service.RepositoryID(context.Background(), repo)
	if err != nil || before != after {
		t.Fatalf("repository was changed: before=%q after=%q err=%v", before, after, err)
	}
	if len(setup.cfg.Destinations) != 1 || setup.cfg.Destinations[0].RepositoryID != before {
		t.Fatalf("config=%#v", setup.cfg)
	}
}
