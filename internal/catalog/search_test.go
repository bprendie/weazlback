package catalog

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestBoundedSearchScopeCountsAndCancellation(t *testing.T) {
	c := New()
	for i := 0; i < 1000; i++ {
		p := fmt.Sprintf("/home/alice/Pictures/image-%04d.png", i)
		c.Paths[p] = &PathRecord{Path: p, Versions: []Version{{MachineID: "alice"}}}
	}
	got, count, err := c.SearchPaths(context.Background(), "png", "alice", "/home/alice/Pictures", 200)
	if err != nil || len(got) != 200 || count != 1000 {
		t.Fatalf("shown=%d total=%d err=%v", len(got), count, err)
	}
	got, _, _ = c.SearchPaths(context.Background(), "png", "alice", "/home/alice/Picture", 200)
	if len(got) != 0 {
		t.Fatal("scope crossed sibling boundary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.SearchPaths(ctx, "", "", "", 200); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestSearchPathsPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("large query measurement")
	}
	for _, count := range []int{100000, 500000} {
		c := New()
		for i := 0; i < count; i++ {
			p := fmt.Sprintf("/home/alice/Code/document-%06d.txt", i)
			c.Paths[p] = &PathRecord{Path: p, Versions: []Version{{MachineID: "alice"}}}
		}
		for _, query := range []string{"document", "document-099999"} {
			var samples []time.Duration
			for range 12 {
				start := time.Now()
				_, _, err := c.SearchPaths(context.Background(), query, "alice", "", 200)
				if err != nil {
					t.Fatal(err)
				}
				samples = append(samples, time.Since(start))
			}
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			t.Logf("%d paths %q: p95 %s", count, query, samples[11])
			if count == 100000 && samples[11] > 100*time.Millisecond {
				t.Fatalf("100k query budget exceeded: %s", samples[11])
			}
		}
	}
}
