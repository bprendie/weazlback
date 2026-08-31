package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

type fakeService struct {
	files map[string][]restic.FileEntry
	diffs map[string][]restic.DiffChange
}

func (f fakeService) Files(_ context.Context, _ restic.Repository, snapshot string) ([]restic.FileEntry, error) {
	return f.files[snapshot], nil
}

func (f fakeService) FilesAt(_ context.Context, _ restic.Repository, snapshot string, paths []string) ([]restic.FileEntry, error) {
	var result []restic.FileEntry
	for _, file := range f.files[snapshot] {
		for _, path := range paths {
			if file.Path == path {
				result = append(result, file)
			}
		}
	}
	return result, nil
}

func (f fakeService) Diff(_ context.Context, _ restic.Repository, source, target string) ([]restic.DiffChange, error) {
	return f.diffs[source+".."+target], nil
}

func TestUpdateBuildsOldestBaselineAndReplaysDiffs(t *testing.T) {
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	t0 := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	points := []restic.Snapshot{
		{ID: "one", Time: t0, Tags: []string{"profile:home", "machine:" + machine}},
		{ID: "two", Time: t0.Add(time.Hour), Tags: []string{"profile:home", "machine:" + machine}},
		{ID: "three", Time: t0.Add(2 * time.Hour), Tags: []string{"profile:home", "machine:" + machine}},
	}
	service := fakeService{files: map[string][]restic.FileEntry{
		"one": {{Path: "/home/alice/old.txt", Type: "file", Size: 4}},
		"two": {{Path: "/home/alice/new.txt", Type: "file", Size: 9}},
	}, diffs: map[string][]restic.DiffChange{
		"one..two":   {{Path: "/home/alice/new.txt", Modifier: "+"}},
		"two..three": {{Path: "/home/alice/old.txt", Modifier: "-"}},
	}}
	c := New()
	if err := Update(context.Background(), &c, service, restic.Repository{}, points, machine, "home"); err != nil {
		t.Fatal(err)
	}
	if c.Chains[ChainKey(machine, "home")].Latest != "three" || len(c.Paths["/home/alice/old.txt"].Versions) != 2 {
		t.Fatalf("catalog=%#v", c)
	}
	results := c.Search("oldtxt", machine, 10)
	if len(results) != 1 || results[0].Path != "/home/alice/old.txt" {
		t.Fatalf("results=%#v", results)
	}
}

func TestCatalogIsVaultEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(filepath.Join(dir, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	c := New()
	c.Paths["/home/alice/secret-name.txt"] = &PathRecord{Path: "/home/alice/secret-name.txt", Name: "secret-name.txt"}
	path := filepath.Join(dir, "catalog")
	if err := Save(path, c, v); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "secret-name") {
		t.Fatal("catalog leaked a plaintext filename")
	}
	loaded, err := Load(path, v)
	if err != nil || loaded.Paths["/home/alice/secret-name.txt"] == nil {
		t.Fatalf("load err=%v catalog=%#v", err, loaded)
	}
}

func TestCorruptCatalogIsRejectedAsDisposableCache(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(filepath.Join(dir, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "catalog")
	if err := os.WriteFile(path, []byte("corrupt plaintext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, v); err == nil {
		t.Fatal("corrupt catalog was accepted")
	}
}

func TestStalePlocateResultsAreDirectlyVerified(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.txt")
	missing := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(live, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "plocate")
	body := "#!/bin/sh\nprintf '" + live + "\\0" + missing + "\\0'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	hints, err := LivePathHints("live", 10)
	if err != nil || len(hints) != 1 || hints[0] != live {
		t.Fatalf("hints=%v err=%v", hints, err)
	}
}
