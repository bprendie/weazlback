package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDistinguishesOwned0600AndUnreadable(t *testing.T) {
	dir := t.TempDir()
	readable := filepath.Join(dir, "readable")
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(readable, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("no"), 0o000); err != nil {
		t.Fatal(err)
	}
	report := Scan([]string{dir}, nil)
	if report.Unreadable != 1 || len(report.Samples) != 1 || report.Samples[0] != blocked {
		t.Fatalf("report=%#v", report)
	}
}

func TestScanHonorsDoubleStarExcludes(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, ".cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "blocked"), []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	report := Scan([]string{dir}, []string{filepath.Join(dir, ".cache", "**")})
	if report.Unreadable != 0 {
		t.Fatalf("report=%#v", report)
	}
}
