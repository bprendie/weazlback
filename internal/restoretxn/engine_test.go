package restoretxn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

type fakeService struct {
	sources  map[string]string
	entries  []restic.FileEntry
	failOnce bool
}

func (f *fakeService) Check(context.Context, restic.Repository, bool) error { return nil }
func (f *fakeService) FilesAt(_ context.Context, _ restic.Repository, _ string, _ []string) ([]restic.FileEntry, error) {
	return append([]restic.FileEntry(nil), f.entries...), nil
}
func (f *fakeService) RestoreWithProgress(_ context.Context, _ restic.Repository, _ string, target string, includes []string, progress func(restic.RestoreProgress)) error {
	if f.failOnce {
		f.failOnce = false
		return errors.New("injected interruption")
	}
	for _, include := range includes {
		if err := os.MkdirAll(filepath.Dir(stagedPath(target, include)), 0o700); err != nil {
			return err
		}
		if err := copyPreserving(f.sources[include], stagedPath(target, include)); err != nil {
			return err
		}
	}
	progress(restic.RestoreProgress{MessageType: "summary", FilesRestored: uint64(len(includes)), TotalFiles: uint64(len(includes))})
	return nil
}

func testEngine(t *testing.T, failOnce bool) (Engine, Plan, string, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "repository-file")
	target := filepath.Join(root, "live", "api-keys.txt")
	if err := os.WriteFile(source, []byte("restored-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("live-version"), 0o640); err != nil {
		t.Fatal(err)
	}
	v := vault.New(filepath.Join(root, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	path := "/home/source/api-keys.txt"
	service := &fakeService{sources: map[string]string{path: source}, failOnce: failOnce,
		entries: []restic.FileEntry{{Path: path, Name: "api-keys.txt", Type: "file", Size: uint64(len("restored-secret")), Mode: 0o600}}}
	plan := Plan{ID: "operation-one", Snapshot: "point-one", SourceMachineID: "a", TargetMachineID: "a",
		Items: []Item{{SourcePath: path, TargetPath: target}}, StageRoot: filepath.Join(root, "stage"),
		JournalPath: filepath.Join(root, "journals", "one.enc"), Conflict: ReplacePreserving}
	return Engine{Service: service, Cryptor: v}, plan, source, target
}

func TestReplacePreservesRollbackAndEncryptedJournal(t *testing.T) {
	engine, plan, _, target := testEngine(t, false)
	result, err := engine.Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "restored-secret" || len(result.Rollback) != 1 {
		t.Fatalf("content=%q result=%#v", content, result)
	}
	old, _ := os.ReadFile(result.Rollback[0])
	if string(old) != "live-version" {
		t.Fatalf("rollback content=%q", old)
	}
	raw, _ := os.ReadFile(plan.JournalPath)
	if strings.Contains(string(raw), "api-keys") || strings.Contains(string(raw), "operation-one") {
		t.Fatal("encrypted journal leaked paths or operation identity")
	}
	if err := engine.Rollback(plan); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(target)
	if string(restored) != "live-version" {
		t.Fatalf("rollback failed: %q", restored)
	}
}

func TestInterruptedExtractionResumesAndStagingOnlyDoesNotPlace(t *testing.T) {
	engine, plan, _, target := testEngine(t, true)
	plan.StagingOnly = true
	if _, err := engine.Run(context.Background(), plan, nil); err == nil {
		t.Fatal("injected interruption did not fail")
	}
	raw, err := os.ReadFile(plan.JournalPath)
	if err != nil || strings.Contains(string(raw), "operation-one") {
		t.Fatalf("resumable journal missing or plaintext: %q err=%v", raw, err)
	}
	if _, err := engine.Run(context.Background(), plan, nil); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "live-version" {
		t.Fatalf("staging-only changed live target: %q", content)
	}
}

func TestCrossMachineIdentityPathIsBlocked(t *testing.T) {
	engine, plan, _, _ := testEngine(t, false)
	plan.SourceMachineID, plan.TargetMachineID = "source", "target"
	plan.Items[0].TargetPath = filepath.Join(t.TempDir(), ".config", "weazlback", "config.json")
	if _, err := engine.Run(context.Background(), plan, nil); err == nil || !strings.Contains(err.Error(), "identity-bearing") {
		t.Fatalf("err=%v", err)
	}
}

func TestSafeOverlayPreservesUnrelatedLiveFilesAndRollsBackConflicts(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	for _, directory := range []string{source, target} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(source, "same.txt"), []byte("historical"), 0o600)
	_ = os.WriteFile(filepath.Join(target, "same.txt"), []byte("live"), 0o600)
	_ = os.WriteFile(filepath.Join(target, "created-later.txt"), []byte("later"), 0o600)
	v := vault.New(filepath.Join(root, "vault"))
	_ = v.Create([]byte("x"))
	repositoryPath := "/home/source/folder"
	service := &fakeService{sources: map[string]string{repositoryPath: source}, entries: []restic.FileEntry{
		{Path: repositoryPath, Type: "dir"}, {Path: repositoryPath + "/same.txt", Type: "file", Size: uint64(len("historical"))},
	}}
	plan := Plan{ID: "overlay", Snapshot: "point", SourceMachineID: "a", TargetMachineID: "a", Conflict: OverlayPreserving,
		Items: []Item{{SourcePath: repositoryPath, TargetPath: target}}, StageRoot: filepath.Join(root, "stage"), JournalPath: filepath.Join(root, "journal.enc")}
	engine := Engine{Service: service, Cryptor: v}
	result, err := engine.Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "created-later.txt")); err != nil {
		t.Fatal("safe overlay removed a later file")
	}
	content, _ := os.ReadFile(filepath.Join(target, "same.txt"))
	if string(content) != "historical" || len(result.Rollback) != 1 {
		t.Fatalf("content=%q result=%#v", content, result)
	}
	if err := engine.Rollback(plan); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(filepath.Join(target, "same.txt"))
	if string(content) != "live" {
		t.Fatalf("overlay rollback content=%q", content)
	}
}

func TestExactDeletionIsPreservedAndRollbackRestoresIt(t *testing.T) {
	engine, plan, _, _ := testEngine(t, false)
	removed := filepath.Join(filepath.Dir(plan.Items[0].TargetPath), "created-later.txt")
	if err := os.WriteFile(removed, []byte("keep for rollback"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyExactDeletions(plan, []string{removed}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Fatalf("exact deletion remained: %v", err)
	}
	if err := engine.Rollback(plan); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(removed)
	if err != nil || string(content) != "keep for rollback" {
		t.Fatalf("rollback content=%q err=%v", content, err)
	}
}

func TestCrossDevicePlacementUsesValidatedTargetSideCommit(t *testing.T) {
	if _, err := os.Stat("/dev/shm"); err != nil {
		t.Skip("no separate memory filesystem")
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("cross-device"), 0o640); err != nil {
		t.Fatal(err)
	}
	targetRoot, err := os.MkdirTemp("/dev/shm", "weazlback-cross-device-")
	if err != nil {
		t.Skipf("cannot create cross-device fixture: %v", err)
	}
	defer os.RemoveAll(targetRoot)
	left, right := &syscall.Stat_t{}, &syscall.Stat_t{}
	if syscall.Stat(source, left) != nil || syscall.Stat(targetRoot, right) != nil || left.Dev == right.Dev {
		t.Skip("fixture filesystems are not distinct")
	}
	target := filepath.Join(targetRoot, "placed")
	if err := placeAtomic(source, target, ""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "cross-device" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestFullDiskPreflightBlocksBeforeExtraction(t *testing.T) {
	engine, plan, _, target := testEngine(t, false)
	original := availableBytesForPreflight
	availableBytesForPreflight = func(string) (uint64, error) { return 1, nil }
	defer func() { availableBytesForPreflight = original }()
	if _, err := engine.Run(context.Background(), plan, nil); err == nil || !strings.Contains(err.Error(), "insufficient destination space") {
		t.Fatalf("err=%v", err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "live-version" {
		t.Fatal("full-disk preflight changed the live target")
	}
}

func TestRepositoryLossLeavesResumableEncryptedJournal(t *testing.T) {
	engine, plan, _, target := testEngine(t, true)
	if _, err := engine.Run(context.Background(), plan, nil); err == nil {
		t.Fatal("injected repository loss did not interrupt")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "live-version" {
		t.Fatal("repository loss changed live data")
	}
	if _, err := engine.Run(context.Background(), plan, nil); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
}

func TestPermissionDeniedPlacementPreservesLiveObject(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses fixture permissions")
	}
	engine, plan, _, target := testEngine(t, false)
	parent := filepath.Dir(target)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o700)
	if _, err := engine.Run(context.Background(), plan, nil); err == nil {
		t.Fatal("permission-denied placement succeeded")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "live-version" {
		t.Fatal("permission failure lost live data")
	}
}
