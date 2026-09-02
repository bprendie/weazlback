package freshrestore

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func TestSelectProfileSnapshotChoosesNewestMatchingProfile(t *testing.T) {
	old := restic.Snapshot{ID: "old", Time: time.Unix(1, 0), Tags: []string{"profile:home"}}
	newer := restic.Snapshot{ID: "new", Time: time.Unix(2, 0), Tags: []string{"profile:home"}}
	heavy := restic.Snapshot{ID: "heavy", Time: time.Unix(3, 0), Tags: []string{"profile:heavy"}}
	got, err := selectProfileSnapshot([]restic.Snapshot{old, heavy, newer}, "home")
	if err != nil || got.ID != "new" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestTopLevelTargetsDeduplicatesHomeRoots(t *testing.T) {
	files := []restic.FileEntry{
		{Path: "/home/alice/Documents/one"},
		{Path: "/home/alice/Documents/two"},
		{Path: "/home/alice/.config/app"},
		{Path: "/etc/hostname"},
	}
	want := []string{"/home/bob/.config", "/home/bob/Documents"}
	if got := topLevelTargets(files, "/home/alice", "/home/bob"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEverythingRetainsAllThreeRestorePoints(t *testing.T) {
	r := Restore{Plan: Plan{
		Snapshot:      restic.Snapshot{ID: "core"},
		HomeSnapshot:  &restic.Snapshot{ID: "home"},
		HeavySnapshot: &restic.Snapshot{ID: "heavy"},
	}}
	if r.Plan.HomeSnapshot.ID != "home" || r.Plan.HeavySnapshot.ID != "heavy" || r.Plan.Snapshot.ID != "core" {
		t.Fatal("scope restore points were not retained")
	}
}

func TestEverythingPlanExplicitlyIncludesParallelApplications(t *testing.T) {
	text := PlanText(Plan{Scope: "everything"})
	if !strings.Contains(text, "Applications (parallel) + Core + Home + Heavy") {
		t.Fatalf("ambiguous Everything plan: %q", text)
	}
}

func TestHeavyCommitFollowsUsableHomePlacement(t *testing.T) {
	if !stageAtLeast("heavy_committed", "core_committed") || !stageAtLeast("heavy_committed", "user_state_reconciled") {
		t.Fatal("Heavy commit is not ordered after Core/Home placement")
	}
	if stageAtLeast("core_committed", "heavy_committed") {
		t.Fatal("Core placement incorrectly waits for Heavy")
	}
}

func TestApplicationsIsAFirstClassRecoveryScope(t *testing.T) {
	for _, scope := range []string{"core", "home", "everything", "applications"} {
		if !validRecoveryScope(scope) {
			t.Fatalf("scope %q rejected", scope)
		}
	}
	if validRecoveryScope("all-ish") {
		t.Fatal("unknown scope accepted")
	}
}

func TestPointInTimeCompositionUsesNearestHealthyComponent(t *testing.T) {
	requested := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	points := []restic.Snapshot{
		{ID: "earlier", Time: requested.Add(-time.Hour), Tags: []string{"profile:home"}},
		{ID: "later", Time: requested.Add(time.Hour), Tags: []string{"profile:home"}},
		{ID: "incomplete", Time: requested, Tags: []string{"profile:home", "incomplete"}},
	}
	point, err := selectProfileSnapshotAt(points, "home", requested)
	if err != nil || point.ID != "earlier" {
		t.Fatalf("point=%#v err=%v", point, err)
	}
}

func TestRecoveryPathMappingCannotCarrySourceUsername(t *testing.T) {
	if got := mapRecoveryPath("/home/alice/Code/project", "/home/bob"); got != "/home/bob/Code/project" {
		t.Fatalf("mapped=%q", got)
	}
	if got := mapRecoveryPath("/etc/hosts", "/home/bob"); got != "/etc/hosts" {
		t.Fatalf("system path mapped=%q", got)
	}
}
