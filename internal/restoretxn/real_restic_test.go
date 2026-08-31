package restoretxn

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestRealResticSelectiveTransactionDrill(t *testing.T) {
	if testing.Short() {
		t.Skip("real Restic drill")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source", "Code")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "main.go")
	if err := os.WriteFile(file, []byte("package restored\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("main.go", filepath.Join(source, "current")); err != nil {
		t.Fatal(err)
	}
	repo := restic.Repository{Location: filepath.Join(root, "repository"), Password: []byte("drill-secret")}
	service := restic.NewService(io.Discard)
	ctx := context.Background()
	if err := service.Initialize(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := service.BackupMachineWithProgress(ctx, repo, "home", "thinkpad", []string{source}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	points, err := service.SnapshotsForMachine(ctx, repo, "thinkpad")
	if err != nil || len(points) != 1 {
		t.Fatalf("points=%v err=%v", points, err)
	}
	target := filepath.Join(root, "alternate", "Code")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "main.go"), []byte("live\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cryptor := vault.New(filepath.Join(root, "vault"))
	if err := cryptor.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	plan := Plan{ID: "real-restic", Snapshot: points[0].ID, SourceMachineID: "thinkpad", TargetMachineID: "desktop", Repository: repo,
		Items: []Item{{SourcePath: source, TargetPath: target}}, StageRoot: filepath.Join(root, "stage"), JournalPath: filepath.Join(root, "journal.enc"),
		Conflict: OverlayPreserving, TargetUID: uint32(os.Getuid()), TargetGID: uint32(os.Getgid())}
	engine := Engine{Service: service, Cryptor: cryptor}
	result, err := engine.Run(ctx, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(target, "main.go"))
	if err != nil || string(content) != "package restored\n" || len(result.Rollback) == 0 {
		t.Fatalf("content=%q result=%#v err=%v", content, result, err)
	}
	info, _ := os.Stat(filepath.Join(target, "main.go"))
	link, linkErr := os.Readlink(filepath.Join(target, "current"))
	if info.Mode().Perm() != 0o640 || linkErr != nil || link != "main.go" {
		t.Fatalf("mode=%o link=%q err=%v", info.Mode().Perm(), link, linkErr)
	}
	if err := engine.Rollback(plan); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(filepath.Join(target, "main.go"))
	if string(content) != "live\n" {
		t.Fatalf("rollback content=%q", content)
	}
}
