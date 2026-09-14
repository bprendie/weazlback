package freshrestore

import (
	"github.com/bprendie/weazlback/internal/architecture"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRetainTargetBeforeParentPlacement(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	for root, body := range map[string]string{source: "x64-device", target: "snapdragon-device"} {
		os.MkdirAll(filepath.Join(root, ".config/hypr"), 0700)
		os.WriteFile(filepath.Join(root, ".config/hypr/monitors.conf"), []byte(body), 0600)
	}
	os.WriteFile(filepath.Join(source, ".config/portable.conf"), []byte("theme"), 0600)
	if err := retainTarget(source, target, ".config/hypr"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(source, ".config/hypr/monitors.conf"))
	if string(b) != "snapdragon-device" {
		t.Fatal(string(b))
	}
	b, _ = os.ReadFile(filepath.Join(source, ".config/portable.conf"))
	if string(b) != "theme" {
		t.Fatal("lost portable settings")
	}
}
func TestRetainTargetRefusesStagedSymlinkParent(t *testing.T) {
	source, outside := t.TempDir(), t.TempDir()
	os.Symlink(outside, filepath.Join(source, ".config"))
	if err := retainTarget(source, t.TempDir(), ".config/hypr"); err == nil {
		t.Fatal("followed staged symlink")
	}
}
func TestWithheldPlacementIsJournaledWithoutDeletingTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "existing")
	os.WriteFile(target, []byte("keep"), 0600)
	r := Restore{Plan: Plan{Migration: Migration{Active: true}}, JournalPath: filepath.Join(root, "journal.json")}
	if err := r.placeJournaled(filepath.Join(root, "missing"), target); err != nil {
		t.Fatal(err)
	}
	if !contains(r.Journal.WithheldPaths, target) {
		t.Fatal("missing withheld record")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "keep" {
		t.Fatal("target changed")
	}
}
func TestArchitectureMigrationDoesNotAffectNativeRestore(t *testing.T) {
	if migrationPolicy("amd64", "x86_64").Active {
		t.Fatal("aliases treated as migration")
	}
	if !migrationPolicy("amd64", "aarch64").Active {
		t.Fatal("ARM migration not detected")
	}
}

func TestCompiledBundleMigrationPreservesTargetAndInstallsNativePair(t *testing.T) {
	bundle := os.Getenv("WEAZLBACK_TEST_BUNDLE")
	if bundle == "" {
		t.Skip("set WEAZLBACK_TEST_BUNDLE for migration smoke test")
	}
	stage, target := t.TempDir(), t.TempDir()
	sourceHome := "/home/source"
	home := stagedPath(stage, sourceHome)
	for _, root := range []string{home, target} {
		os.MkdirAll(filepath.Join(root, ".config/hypr"), 0700)
	}
	os.WriteFile(filepath.Join(home, ".config/hypr/monitors.conf"), []byte("source-device"), 0600)
	os.WriteFile(filepath.Join(target, ".config/hypr/monitors.conf"), []byte("target-device"), 0600)
	os.WriteFile(filepath.Join(home, ".config/editor.conf"), []byte("portable"), 0600)
	foreign := "arm64"
	if runtime.GOARCH == "arm64" {
		foreign = "amd64"
	}
	foreignFile := filepath.Join(home, ".local/bin/foreign-app")
	os.MkdirAll(filepath.Dir(foreignFile), 0700)
	if err := copyAcrossFilesystems(filepath.Join(bundle, "linux-"+foreign, "weazlback"), foreignFile); err != nil {
		t.Fatal(err)
	}
	restricted := filepath.Join(home, ".cache/yay/slic3r/pkg")
	if err := os.MkdirAll(restricted, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restricted, "portable.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restricted, 0111); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(restricted, 0700)
	r := Restore{Options: Options{RecoveryPath: filepath.Join(bundle, "weazlback-recovery.wzrk")}, Plan: Plan{OriginalHome: sourceHome, TargetHome: target, Migration: migrationPolicy(foreign, runtime.GOARCH)}}
	if err := r.migrateStage(stage); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(restricted); err != nil || info.Mode().Perm() != 0111 {
		t.Fatalf("restored directory mode changed: %v, %v", info, err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".config/hypr/monitors.conf"))
	if string(b) != "target-device" {
		t.Fatal("target device settings not retained")
	}
	b, _ = os.ReadFile(filepath.Join(home, ".config/editor.conf"))
	if string(b) != "portable" {
		t.Fatal("portable config lost")
	}
	if _, err := os.Stat(foreignFile); !os.IsNotExist(err) {
		t.Fatal("foreign binary still active")
	}
	if len(r.MigrationWithheld) != 1 {
		t.Fatal(r.MigrationWithheld)
	}
	for _, tool := range []string{"weazlback", "weazlback-restore"} {
		if err := architecture.ValidateELF(filepath.Join(home, ".weazlback/bin", tool), runtime.GOARCH); err != nil {
			t.Fatal(err)
		}
	}
}
