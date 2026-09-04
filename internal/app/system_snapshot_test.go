package app

import (
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/restic"
)

func TestRetrySelectsNewestIncompleteGeneration(t *testing.T) {
	gens := []generation.Generation{
		{ID: "complete", MachineID: "m", Complete: true, EndedAt: time.Unix(3, 0)},
		{ID: "other", MachineID: "other", EndedAt: time.Unix(4, 0)},
		{ID: "retry", MachineID: "m", Failed: true, EndedAt: time.Unix(2, 0)},
	}
	if got := latestIncompleteID(gens, "m"); got != "retry" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerationMembersRetainNewestLaneAttempt(t *testing.T) {
	points := []restic.Snapshot{
		{ID: "old", Time: time.Unix(1, 0), Tags: []string{"generation:g", "profile:home"}},
		{ID: "new", Time: time.Unix(2, 0), Tags: []string{"generation:g", "profile:home"}},
	}
	if got := generationMembers(points, "g")["home"].ID; got != "new" {
		t.Fatalf("got %q", got)
	}
}

func TestScrubSelectionRejectsHealthyCompleteGeneration(t *testing.T) {
	_, err := selectGeneration([]generation.Generation{{ID: "safe", Complete: true}}, "safe", false)
	if err == nil {
		t.Fatal("healthy complete generation selected for scrub")
	}
}
