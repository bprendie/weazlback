package freshrestore

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestLegacyRecoveryKitEnumeratesWithoutRepositoryInitialization(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic is not installed")
	}
	dir := t.TempDir()
	repoPath, fixture := filepath.Join(dir, "repository"), filepath.Join(dir, "fixture")
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "legacy.txt"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	password, passphrase := []byte("repo-secret"), []byte("kit-pass")
	service := restic.NewService(io.Discard)
	repo := restic.Repository{Location: repoPath, Password: password}
	if err := service.Initialize(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if err := service.Backup(context.Background(), repo, "home", []string{fixture}, nil, false); err != nil {
		t.Fatal(err)
	}
	repositoryID, _ := service.RepositoryID(context.Background(), repo)
	vaultPath := filepath.Join(dir, "vault.enc")
	v := vault.New(vaultPath)
	if err := v.Create(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("destination/legacy/repository-password", password); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	cfg := config.Default()
	cfg.Machine = config.Machine{} // rc2-style configuration
	cfg.ActiveDestination = "legacy"
	cfg.Destinations = []config.Destination{{ID: "legacy", Name: "legacy", Kind: "local", Repository: repoPath,
		RepositoryID: repositoryID, PasswordKey: "destination/legacy/repository-password"}}
	cfgPath := filepath.Join(dir, "config.json")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(dir, "legacy.wzrk")
	if err := recovery.Export(kit, recovery.Sources{Vault: vaultPath, Config: cfgPath}, passphrase); err != nil {
		t.Fatal(err)
	}
	before, _ := service.RepositoryID(context.Background(), repo)
	identities, err := ReadRecoveryIdentities(context.Background(), kit, passphrase, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := service.RepositoryID(context.Background(), repo)
	if len(identities) != 1 || !identities[0].Legacy || before != after {
		t.Fatalf("identities=%#v before=%q after=%q", identities, before, after)
	}
}
