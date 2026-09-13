package packagecapsule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedCacheInspectionHasProgressAndSkipsPartialFiles(t *testing.T) {
	cache := t.TempDir()
	for _, name := range []string{"alpha-1.0-1-any.pkg.tar.zst", "beta-1.0-1-any.pkg.tar.zst", "unfinished-1-any.pkg.tar.zst.part"} {
		if err := os.WriteFile(filepath.Join(cache, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var seen []Progress
	manifest, _, cleanup, err := PrepareArtifacts(Options{StagingRoot: t.TempDir(), ArtifactRoots: []string{cache}, Run: &fakeRunner{}, Progress: func(p Progress) {
		if p.Phase == "inspect cache" {
			seen = append(seen, p)
		}
	}}, Manifest{Architecture: "aarch64", Packages: []Package{{Name: "alpha", Installed: "1.0-1", Source: "official"}}})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[0].Completed != 0 || seen[1].Completed != 1 || seen[2].Completed != 2 || seen[2].Total != 2 || manifest.Summary.Captured != 2 {
		t.Fatalf("progress=%+v captured=%d", seen, manifest.Summary.Captured)
	}
	for _, p := range seen {
		if strings.Contains(p.Package, "unfinished") {
			t.Fatal("partial archive inspected")
		}
	}
}
