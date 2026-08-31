package backupmeta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
