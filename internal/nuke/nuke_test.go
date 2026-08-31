package nuke

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
)

func TestDeleteLocalPreservesConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(filepath.Join(repository, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "config"), []byte("ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRepository(config.Destination{Kind: "local", Repository: repository}, nil, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(repository)
	if err != nil || len(entries) != 0 {
		t.Fatalf("repository was not preserved empty: entries=%d err=%v", len(entries), err)
	}
}

func TestDeleteLocalCanRemoveExactDirectory(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRepository(config.Destination{Kind: "local", Repository: repository}, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(repository); !os.IsNotExist(err) {
		t.Fatalf("repository still exists: %v", err)
	}
}

func TestDeleteLocalRejectsBroadTargets(t *testing.T) {
	for _, path := range []string{"/", "/tmp"} {
		if err := DeleteRepository(config.Destination{Kind: "local", Repository: path}, nil, true); err == nil {
			t.Fatalf("unsafe path %q accepted", path)
		}
	}
}
