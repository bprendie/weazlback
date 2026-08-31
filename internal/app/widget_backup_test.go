package app

import (
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/contracts"
)

func TestAggregateUsesHomeWhileCoreDiscovers(t *testing.T) {
	status := contracts.Status{
		Progress: &contracts.Progress{},
		Profiles: []contracts.ProfileProgress{
			{Profile: "CORE", State: "discovering"},
			{Profile: "HOME", State: "complete", Bytes: 100, TotalBytes: 100, Files: 9, Total: 10},
		},
	}
	aggregateWidgetProgress(&status, time.Now())
	if status.Progress.Percent != 0.99 {
		t.Fatalf("percent = %v, want 0.99 until Core commits", status.Progress.Percent)
	}
	if status.Progress.TotalFiles != 10 {
		t.Fatalf("total files = %d, want Home denominator 10", status.Progress.TotalFiles)
	}
}

func TestAggregateCompletesAfterEveryLane(t *testing.T) {
	status := contracts.Status{
		Progress: &contracts.Progress{Percent: 0.99},
		Profiles: []contracts.ProfileProgress{
			{Profile: "CORE", State: "complete", Percent: 1},
			{Profile: "HOME", State: "complete", Percent: 1, Bytes: 100, TotalBytes: 100},
		},
	}
	aggregateWidgetProgress(&status, time.Now())
	if status.Progress.Percent != 1 {
		t.Fatalf("percent = %v, want 1", status.Progress.Percent)
	}
}

func TestAggregateCompletesZeroByteHeavy(t *testing.T) {
	status := contracts.Status{
		Progress: &contracts.Progress{},
		Profiles: []contracts.ProfileProgress{{Profile: "HEAVY", State: "complete", Percent: 1}},
	}
	aggregateWidgetProgress(&status, time.Now())
	if status.Progress.Percent != 1 {
		t.Fatalf("percent = %v, want 1 for completed zero-byte lane", status.Progress.Percent)
	}
}
