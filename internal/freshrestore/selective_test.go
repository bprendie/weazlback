package freshrestore

import (
	"context"
	"encoding/json"
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

func TestRecoveryKitCanBrowseAndSelectivelyRestoreWithoutInstalledApp(t *testing.T) {
	if testing.Short() {
		t.Skip("real recovery-kit Restic drill")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	root := t.TempDir()
	t.Setenv("WEAZLBACK_HOME", filepath.Join(root, "state"))
	fixture := filepath.Join(root, "fixture", "Code")
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(fixture, "project.txt")
	if err := os.WriteFile(file, []byte("recover me"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := restic.NewService(io.Discard)
	repoPassword := []byte("repository-secret")
	repo := restic.Repository{Location: filepath.Join(root, "repository"), Password: repoPassword}
	ctx := context.Background()
	if err := service.Initialize(ctx, repo); err != nil {
		t.Fatal(err)
	}
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := service.BackupMachineWithProgress(ctx, repo, "home", machine, []string{fixture}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	points, err := service.SnapshotsForMachine(ctx, repo, machine)
	if err != nil || len(points) != 1 {
		t.Fatalf("points=%v err=%v", points, err)
	}
	passphrase := []byte("vault-passphrase")
	vaultPath := filepath.Join(root, "vault.enc")
	v := vault.New(vaultPath)
	if err := v.Create(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("repo-password", repoPassword); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	cfg := config.Default()
	cfg.Machine.ID = machine
	cfg.ActiveDestination = "local"
	cfg.Destinations = []config.Destination{{ID: "local", Kind: "local", Repository: repo.Location, PasswordKey: "repo-password"}}
	cfgBytes, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(cfgPath, cfgBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(root, "recovery.wzrk")
	if err := recovery.Export(kit, recovery.Sources{Vault: vaultPath, Config: cfgPath}, passphrase); err != nil {
		t.Fatal(err)
	}
	files, err := RecoveryFiles(ctx, kit, passphrase, "local", points[0].ID)
	if err != nil || len(files.Files) == 0 {
		t.Fatalf("files=%v err=%v", files.Files, err)
	}
	if err := os.RemoveAll(fixture); err != nil {
		t.Fatal(err)
	}
	result, err := RestoreRecoverySelection(ctx, SelectiveOptions{RecoveryPath: kit, Destination: "local", MachineID: machine,
		TargetMachineID: machine, Snapshot: points[0].ID, SourcePath: fixture, Passphrase: passphrase, WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(file)
	if err != nil || string(body) != "recover me" || len(result.Placed) == 0 {
		t.Fatalf("body=%q result=%#v err=%v", body, result, err)
	}
}
