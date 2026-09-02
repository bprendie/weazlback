package freshrestore

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBtrfsPrivateStageDisablesCompressionAndCleansUp(t *testing.T) {
	mount, err := resolveMount(".")
	if err != nil || mount.filesystem != "btrfs" {
		t.Skip("test workspace is not Btrfs")
	}
	root, err := os.MkdirTemp(".", ".turbo-stage-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	stage := filepath.Join(root, "stage")
	if out, err := exec.Command("btrfs", "subvolume", "create", stage).CombinedOutput(); err != nil {
		t.Fatalf("create: %v: %s", err, out)
	}
	if out, err := exec.Command("btrfs", "property", "set", stage, "compression", "none").CombinedOutput(); err != nil {
		t.Fatalf("property: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(stage, "proof"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("btrfs", "property", "get", stage, "compression").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "none") {
		t.Fatalf("compression=%q err=%v", out, err)
	}
	if err := deleteBtrfsSubvolume(stage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("stage remains: %v", err)
	}
}

func TestSyncPathFilesystemsAcceptsPlacedTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("durable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncPathFilesystems([]string{dir}); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryLockErrorIsNarrow(t *testing.T) {
	for _, message := range []string{
		"unable to create lock in backend: repository is already locked",
		"repository is already locked by PID 85336",
	} {
		if !repositoryLockError(errors.New(message)) {
			t.Fatalf("lock error not recognized: %q", message)
		}
	}
	if repositoryLockError(errors.New("wrong password or no key found")) {
		t.Fatal("authentication failure was treated as a stale lock")
	}
}
