package tui

import (
	"fmt"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func TestRestoreTree500kNavigationBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("large tree measurement")
	}
	m := restoreFixture()
	m.restoreEntries = nil
	for i := 0; i < 500000; i++ {
		m.restoreEntries = append(m.restoreEntries, restic.FileEntry{Path: fmt.Sprintf("/home/alice/dir-%04d/file-%06d", i/500, i), Type: "file"})
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	m.buildRestoreTree()
	m.filterRestoreTree()
	build := time.Since(start)
	runtime.ReadMemStats(&after)
	var samples []time.Duration
	for i := 0; i < 40; i++ {
		start = time.Now()
		m.moveRestoreTree(true)
		m.moveRestoreTree(false)
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p95 := samples[37]
	t.Logf("500k entries: tree build %s, index allocated %.1f MiB, warm expand/collapse p95 %s", build, float64(after.TotalAlloc-before.TotalAlloc)/(1<<20), p95)
	if p95 > 50*time.Millisecond {
		t.Fatalf("warm navigation exceeds 50ms: %s", p95)
	}
}
