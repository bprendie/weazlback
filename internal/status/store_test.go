package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/contracts"
)

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "status.json")}
	want := contracts.Status{State: "backing-up", Progress: &contracts.Progress{Percent: .42, Files: 21, TotalFiles: 50}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Progress == nil || got.Progress.Percent != .42 || got.Progress.TotalFiles != 50 {
		t.Fatalf("status=%#v", got)
	}
}

func TestWidgetStatusNeverPersistsManifestFilename(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "status.json")}
	if err := store.Save(contracts.Status{State: "incomplete", Manifest: "/home/bob/private-skipped-files.json"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(store.Path)
	if strings.Contains(string(raw), "private-skipped") || strings.Contains(string(raw), "/home/bob") {
		t.Fatalf("status leaked filename: %s", raw)
	}
}
