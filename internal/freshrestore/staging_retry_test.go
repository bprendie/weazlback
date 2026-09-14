package freshrestore

import (
	"github.com/bprendie/weazlback/internal/restic"
	"os"
	"path/filepath"
	"testing"
)

func TestStandardRetryPreservesPreviouslyExtractedFiles(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(stage, "extracted")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STAGE_SENTINEL", sentinel)
	script := filepath.Join(root, "restic")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\ntest -f \"$STAGE_SENTINEL\" || exit 97\nprintf '{}\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r := Restore{StageDir: stage, Plan: Plan{OriginalHome: "/home/source", Snapshot: restic.Snapshot{ID: "core"}, HomeSnapshot: &restic.Snapshot{ID: "home"}, ScopeDecision: ScopeDecision{Requested: "core-home", IncludeHome: true}}, Journal: Journal{SnapshotID: "core", HomeSnapshotID: "home"}, Session: &Session{Repository: restic.Repository{Location: root, Password: []byte("fixture")}}, service: restic.Service{Runner: restic.Runner{Binary: script}}}
	if err := r.stageCoreStandard(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
		t.Fatalf("lost extracted files: %s %v", b, err)
	}
}

func TestPlacementCopiesRestrictedTreesAndPreservesModes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	restricted := filepath.Join(source, "pkg")
	if err := os.MkdirAll(restricted, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(restricted, "data")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restricted, 0111); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(restricted, 0700)
	defer os.Chmod(filepath.Join(target, "pkg"), 0700)
	// Existing targets must be merged directly, never nested as target/source.
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyAcrossFilesystems(source, target); err != nil {
		t.Fatal(err)
	}
	for _, tree := range []string{source, target} {
		for rel, mode := range map[string]os.FileMode{"pkg": 0111, "pkg/data": 0} {
			info, err := os.Stat(filepath.Join(tree, rel))
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("%s/%s mode: %v %v", tree, rel, info, err)
			}
		}
	}
	if err := os.Chmod(filepath.Join(target, "pkg/data"), 0600); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "pkg/data")); err != nil || string(b) != "keep" {
		t.Fatalf("copy content %q %v", b, err)
	}
}
