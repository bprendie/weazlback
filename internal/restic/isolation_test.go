package restic

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIndependentRepositoryFamiliesCannotDecryptEachOther(t *testing.T) {
	if testing.Short() {
		t.Skip("real Restic isolation drill")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	root := t.TempDir()
	fixture := filepath.Join(root, "shared-wallpapers")
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "same.png"), []byte("same plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, ctx := NewService(io.Discard), context.Background()
	bob := Repository{Location: filepath.Join(root, "bob"), Password: []byte("bob-secret")}
	serena := Repository{Location: filepath.Join(root, "serena"), Password: []byte("serena-secret")}
	for _, repo := range []Repository{bob, serena} {
		if err := service.Initialize(ctx, repo); err != nil {
			t.Fatal(err)
		}
		if err := service.BackupMachineWithProgress(ctx, repo, "home", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []string{fixture}, nil, false, false, nil); err != nil {
			t.Fatal(err)
		}
	}
	wrong := bob
	wrong.Password = serena.Password
	if _, err := service.Snapshots(ctx, wrong); err == nil {
		t.Fatal("independent repository accepted another sovereign vault's secret")
	}
}
