package freshrestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
)

func TestNewestHealthyPackageCapsuleIsSelectedIndependently(t *testing.T) {
	points := []restic.Snapshot{
		{ID: "old", Time: unixTime(1), Tags: []string{"profile:packages"}},
		{ID: "incomplete", Time: unixTime(3), Tags: []string{"profile:packages", "incomplete"}},
		{ID: "new", Time: unixTime(2), Tags: []string{"profile:packages"}},
	}
	selected := selectLatestPackageSnapshot(points)
	if selected == nil || selected.ID != "new" {
		t.Fatalf("selected=%+v", selected)
	}
}

func TestCapsuleUsesOneCoordinatedPacmanTransaction(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	writeCommand(t, dir, "sudo", "printf '%s\n' \"$*\" >> \"$COMMAND_LOG\"\nexit 0\n")
	t.Setenv("PATH", dir)
	t.Setenv("COMMAND_LOG", logPath)
	r := Restore{Options: Options{Progress: func(RestoreProgress) {}}, Plan: Plan{PackageDelta: packagecapsule.Delta{Local: []packagecapsule.Install{
		{Name: "one", Source: "official", Artifact: "one"}, {Name: "two", Source: "foreign", Artifact: "two"},
	}}}}
	installed, fallback, failures := r.installVerifiedCapsule(context.Background(), map[string]string{"one": "/capsule/one.pkg", "two": "/capsule/two.pkg"})
	if len(installed) != 2 || len(fallback) != 0 || len(failures) != 0 {
		t.Fatalf("installed=%v fallback=%v failures=%v", installed, fallback, failures)
	}
	logged, _ := os.ReadFile(logPath)
	if lines := strings.Count(strings.TrimSpace(string(logged)), "\n") + 1; lines != 1 {
		t.Fatalf("wanted one transaction, got %d: %s", lines, logged)
	}
	if !strings.Contains(string(logged), "pacman -U --needed --noconfirm -- /capsule/one.pkg /capsule/two.pkg") {
		t.Fatalf("transaction=%s", logged)
	}
}

func TestRejectedCapsuleTransactionFallsBackAsAGroup(t *testing.T) {
	dir := t.TempDir()
	writeCommand(t, dir, "sudo", "exit 1\n")
	t.Setenv("PATH", dir)
	r := Restore{Options: Options{Progress: func(RestoreProgress) {}}, Plan: Plan{PackageDelta: packagecapsule.Delta{Local: []packagecapsule.Install{
		{Name: "one", Source: "official"}, {Name: "two", Source: "foreign"},
	}}}}
	installed, fallback, failures := r.installVerifiedCapsule(context.Background(), map[string]string{"one": "/one", "two": "/two"})
	if len(installed) != 0 || len(fallback) != 2 || len(failures) != 0 {
		t.Fatalf("installed=%v fallback=%v failures=%v", installed, fallback, failures)
	}
	if len(r.Plan.Official) != 1 || r.Plan.Official[0] != "one" || len(r.Plan.AUR) != 1 || r.Plan.AUR[0] != "two" {
		t.Fatalf("official=%v foreign=%v", r.Plan.Official, r.Plan.AUR)
	}
}

func TestArchVersionComparisonUsesEpochAndPackageRelease(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{{"2:1.0-1", "1:9.9-9", 1}, {"1.0-2", "1.0-1", 1}, {"1.0-1", "1.0-1", 0}} {
		got, err := archVersionCompare(context.Background(), test.left, test.right)
		if err != nil {
			t.Fatal(err)
		}
		if got < 0 && test.want >= 0 || got > 0 && test.want <= 0 || got == 0 && test.want != 0 {
			t.Fatalf("vercmp %s %s = %d want sign %d", test.left, test.right, got, test.want)
		}
	}
}

func writeCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func unixTime(value int64) time.Time { return time.Unix(value, 0) }
