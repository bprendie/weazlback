package generation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/restic"
)

func TestCompleteAndScrubGenerationsAgainstRealRestic(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	repo := restic.Repository{Location: filepath.Join(root, "repo"), Password: []byte("test-only")}
	service := restic.NewService(nil)
	if err := service.Initialize(ctx, repo); err != nil {
		t.Fatal(err)
	}
	backup := func(id, profile string) restic.Snapshot {
		source := filepath.Join(root, id+"-"+profile)
		if err := os.MkdirAll(source, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "recent-fixture"), []byte(id+profile), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := service.BackupMachineTaggedWithProgress(ctx, repo, profile, "machine", []string{TagPrefix + id}, []string{source}, nil, false, false, nil); err != nil {
			t.Fatal(err)
		}
		points, err := service.SnapshotsForMachine(ctx, repo, "machine")
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range points {
			if ID(point.Tags) == id && restic.Profile(point.Tags) == profile {
				return point
			}
		}
		t.Fatalf("point %s/%s missing", id, profile)
		return restic.Snapshot{}
	}
	var completeIDs []string
	for _, profile := range append([]string{"generation-ledger"}, RequiredProfiles...) {
		point := backup("complete", profile)
		completeIDs = append(completeIDs, point.ID)
	}
	if err := service.TagSnapshots(ctx, repo, completeIDs, []string{TagComplete}, nil); err != nil {
		t.Fatal(err)
	}
	failed := backup("failed", "generation-ledger")
	if err := service.TagSnapshots(ctx, repo, []string{failed.ID}, []string{TagFailed}, nil); err != nil {
		t.Fatal(err)
	}
	points, err := service.Snapshots(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	latest, ok := LatestComplete(Catalog(points), "machine")
	if !ok || latest.ID != "complete" {
		t.Fatalf("latest=%+v ok=%t", latest, ok)
	}
	if err := service.CheckSubset(ctx, repo, "100%"); err != nil {
		t.Fatal(err)
	}
	if err := service.ForgetSnapshots(ctx, repo, []string{failed.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.PruneUnreferenced(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := service.Check(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	points, err = service.Snapshots(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := LatestComplete(Catalog(points), "machine"); !ok {
		t.Fatal("healthy generation was damaged by scrub")
	}
}
