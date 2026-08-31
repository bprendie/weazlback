package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func largeCatalog() Catalog {
	c := New()
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	files := make([]restic.FileEntry, 100000)
	for i := range files {
		files[i] = restic.FileEntry{Path: fmt.Sprintf("/home/test/tree/%06d/document-%06d.txt", i/100, i), Type: "file", Size: uint64(i)}
	}
	c.Baseline(restic.Snapshot{ID: "point-00", Time: base, Tags: []string{"machine:" + machine, "profile:home"}}, files)
	for point := 1; point < 25; point++ {
		var changes []restic.DiffChange
		for changed := 0; changed < 100; changed++ {
			changes = append(changes, restic.DiffChange{Path: files[point*100+changed].Path, Modifier: "M"})
		}
		c.Apply(restic.Snapshot{ID: fmt.Sprintf("point-%02d", point), Time: base.Add(time.Duration(point) * time.Hour), Tags: []string{"machine:" + machine, "profile:home"}}, changes)
	}
	return c
}

func BenchmarkSearch100kPaths25Points(b *testing.B) {
	c := largeCatalog()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := c.Search("document-099999", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 25); len(got) != 1 {
			b.Fatalf("results=%d", len(got))
		}
	}
}

func BenchmarkBaseline100kPaths(b *testing.B) {
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	files := make([]restic.FileEntry, 100000)
	for i := range files {
		files[i] = restic.FileEntry{Path: fmt.Sprintf("/home/test/tree/%06d/document-%06d.txt", i/100, i), Type: "file"}
	}
	point := restic.Snapshot{ID: "point-00", Time: time.Now(), Tags: []string{"machine:" + machine, "profile:home"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New()
		c.Baseline(point, files)
	}
}

func BenchmarkFuzzySearch100kPaths25Points(b *testing.B) {
	c := largeCatalog()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := c.Search("dcmnt99999", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 25); len(got) == 0 {
			b.Fatal("fuzzy query returned no result")
		}
	}
}

func TestSearch100kPathsMeetsWarmTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("large catalog performance gate")
	}
	c := largeCatalog()
	start := time.Now()
	results := c.Search("document-099999", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 25)
	if elapsed := time.Since(start); len(results) != 1 || elapsed > 100*time.Millisecond {
		t.Fatalf("results=%d elapsed=%s (target <=100ms)", len(results), elapsed)
	}
}
