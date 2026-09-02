package backupmeta

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/inventory"
)

func TestNonCoreDoesNotCreateManifest(t *testing.T) {
	os.Setenv("WEAZLBACK_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	defer os.Unsetenv("WEAZLBACK_CONFIG")
	path, cleanup, err := PrepareApplications(context.Background(), "home")
	defer cleanup()
	if err != nil || path != "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
}

func TestCoreManifestDoesNotCarryPackageArtifacts(t *testing.T) {
	t.Setenv("WEAZLBACK_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	root, cleanup, err := PrepareApplications(context.Background(), "core")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(root, "aur-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("routine Core created an artifact directory: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var manifest inventory.ApplicationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.AURArtifacts) != 0 {
		t.Fatalf("Core embedded %d package artifacts", len(manifest.AURArtifacts))
	}
}
