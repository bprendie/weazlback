package freshrestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestRecoverySessionUnlocksRepositoryWithoutPersistingSecrets(t *testing.T) {
	dir := t.TempDir()
	passphrase := []byte("x")
	vaultPath := filepath.Join(dir, "source.vault")
	v := vault.New(vaultPath)
	if err := v.Create(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("repo-pass", []byte("repository-secret")); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("remote-pass", []byte("remote-secret")); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	cfg := config.Default()
	cfg.Destinations = []config.Destination{{ID: "local", Kind: "local", Repository: "/repo", PasswordKey: "repo-pass"},
		{ID: "remote", Kind: "ssh", Repository: "sftp:user@host:/repo", PasswordKey: "remote-pass"}}
	cfg.ActiveDestination = "local"
	configBytes, _ := json.Marshal(cfg)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(dir, "kit.wzrk")
	if err := recovery.Export(kit, recovery.Sources{Vault: vaultPath, Config: configPath}, passphrase); err != nil {
		t.Fatal(err)
	}
	session, err := OpenSession(kit, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	private := session.PrivateDir
	if string(session.Repository.Password) != "repository-secret" {
		t.Fatal("repository secret was not recovered")
	}
	session.Close()
	if _, err := os.Stat(private); !os.IsNotExist(err) {
		t.Fatalf("private workspace survived close: %v", err)
	}
	catalog, err := ReadRecoveryCatalog(kit, passphrase)
	if err != nil || catalog.Active != "local" || len(catalog.Destinations) != 2 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
	remote, err := OpenSessionDestinationAt(kit, passphrase, "remote", "")
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	if remote.Destination.ID != "remote" || string(remote.Repository.Password) != "remote-secret" {
		t.Fatalf("destination=%q password=%q", remote.Destination.ID, remote.Repository.Password)
	}
}
