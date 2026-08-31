package restoretxn

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

type timedRestic struct {
	restic.Service
	extraction time.Duration
}

func (s *timedRestic) RestoreWithProgress(ctx context.Context, repo restic.Repository, snapshot, target string, includes []string, progress func(restic.RestoreProgress)) error {
	started := time.Now()
	err := s.Service.RestoreWithProgress(ctx, repo, snapshot, target, includes, progress)
	s.extraction = time.Since(started)
	return err
}

func TestSelectiveExtractionRetainsDirectResticThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("real Restic throughput drill")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	root, fixture := t.TempDir(), ""
	fixture = filepath.Join(root, "fixture")
	if err := os.MkdirAll(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 8<<20)
	for index := 0; index < 8; index++ {
		if _, err := rand.Read(payload); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture, string(rune('a'+index))+".bin"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo := restic.Repository{Location: filepath.Join(root, "repo"), Password: []byte("performance-secret")}
	service, ctx := restic.NewService(io.Discard), context.Background()
	if err := service.Initialize(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := service.BackupMachineWithProgress(ctx, repo, "home", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []string{fixture}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	points, _ := service.SnapshotsForMachine(ctx, repo, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	directStarted := time.Now()
	if err := service.Restore(ctx, repo, points[0].ID, filepath.Join(root, "direct"), []string{fixture}); err != nil {
		t.Fatal(err)
	}
	direct := time.Since(directStarted)
	measured := &timedRestic{Service: service}
	v := vault.New(filepath.Join(root, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	plan := Plan{ID: "throughput", Snapshot: points[0].ID, SourceMachineID: "a", TargetMachineID: "a", Repository: repo,
		Items: []Item{{SourcePath: fixture, TargetPath: filepath.Join(root, "target")}}, StageRoot: filepath.Join(root, "stage"),
		JournalPath: filepath.Join(root, "journal.enc"), Conflict: ReplacePreserving, StagingOnly: true,
		TargetUID: uint32(os.Getuid()), TargetGID: uint32(os.Getgid())}
	if _, err := (Engine{Service: measured, Cryptor: v}).Run(ctx, plan, nil); err != nil {
		t.Fatal(err)
	}
	ratio := float64(direct) / float64(measured.extraction)
	t.Logf("direct=%s transactional extraction=%s throughput ratio=%.2f", direct, measured.extraction, ratio)
	if ratio < 0.80 {
		t.Fatalf("transactional extraction throughput %.1f%% is below 80%%", ratio*100)
	}
}
