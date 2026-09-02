package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRawFixtureIsSparseAndMutable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "raw")
	if err := createFixture("raw", root); err != nil {
		t.Fatal(err)
	}
	logical, allocated := treeBytes(root)
	if logical != 4<<30 {
		t.Fatalf("logical bytes = %d", logical)
	}
	if allocated >= logical/2 {
		t.Fatalf("fixture is not meaningfully sparse: logical=%d allocated=%d", logical, allocated)
	}
	if err := mutateFixture("raw", root); err != nil {
		t.Fatal(err)
	}
}

func TestMedianDuration(t *testing.T) {
	if got := medianDuration([]time.Duration{9, 1, 5}); got != 5 {
		t.Fatalf("odd median = %v", got)
	}
	if got := medianDuration([]time.Duration{8, 2, 6, 4}); got != 5 {
		t.Fatalf("even median = %v", got)
	}
}

func TestSelectedFixturesRejectsUnknown(t *testing.T) {
	if _, err := selectedFixtures("tape"); err == nil {
		t.Fatal("expected unknown fixture error")
	}
}

func TestSelectEngineRejectsUnknown(t *testing.T) {
	if _, err := selectEngine(Options{Engine: "tape"}); err == nil {
		t.Fatal("expected unknown engine error")
	}
}

func TestWritePatternUsesExactSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blob")
	if err := writePattern(path, 17, 1); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != 17 {
		t.Fatalf("size=%d err=%v", info.Size(), err)
	}
}
