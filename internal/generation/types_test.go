package generation

import (
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func TestCatalogRequiresEveryLaneAndSortsByTime(t *testing.T) {
	now := time.Unix(100, 0)
	var points []restic.Snapshot
	for i, profile := range RequiredProfiles {
		points = append(points, restic.Snapshot{ID: profile, Time: now.Add(time.Duration(i) * time.Minute), Tags: []string{"profile:" + profile, "machine:m", "generation:g1", TagComplete}})
	}
	points = append(points, restic.Snapshot{ID: "new-core", Time: now.Add(time.Hour), Tags: []string{"profile:core", "machine:m", "generation:g2", TagComplete}})
	got := Catalog(points)
	if len(got) != 2 || got[0].ID != "g2" || got[0].Complete || !got[1].Complete {
		t.Fatalf("catalog=%+v", got)
	}
	latest, ok := LatestComplete(got, "m")
	if !ok || latest.ID != "g1" {
		t.Fatalf("latest=%+v ok=%t", latest, ok)
	}
}

func TestFailedCompleteGenerationIsNeverSelected(t *testing.T) {
	g := Generation{ID: "bad", Complete: true, Failed: true, MachineID: "m"}
	if _, ok := LatestComplete([]Generation{g}, "m"); ok {
		t.Fatal("failed generation selected")
	}
}
