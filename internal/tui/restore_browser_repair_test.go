package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
)

type quietBrowsers struct{}

func (quietBrowsers) Running(browserrepair.Family, int) bool { return false }

func TestInstalledBundleBrowserRepairRequiresCrossHostBundle(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".config", "chromium")
	for _, path := range []string{filepath.Join(root, "Local State"), filepath.Join(root, "Default", "History"), filepath.Join(root, "SingletonLock")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	options := browserrepair.Options{Home: home, UID: os.Getuid(), Processes: quietBrowsers{}}
	parts := []restoretxn.Component{{Bundle: restoretxn.SystemConfig, Snapshot: restic.Snapshot{Hostname: "source-host"}}}
	_, result := repairInstalledBundleBrowsersWithOptions(parts, "target-host", options)
	if result.Removed != 1 {
		t.Fatalf("cross-host result: %+v", result)
	}

	if err := os.WriteFile(filepath.Join(root, "SingletonLock"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	parts[0].Snapshot.Hostname = "target-host"
	_, result = repairInstalledBundleBrowsersWithOptions(parts, "target-host", options)
	if result.Removed != 0 {
		t.Fatalf("same-host result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "SingletonLock")); err != nil {
		t.Fatal("same-host lock changed")
	}
}
