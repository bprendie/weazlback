package catalog

import (
	"container/heap"
	"context"
	"sort"
	"strings"
)

type rankedPath struct {
	record PathRecord
	rank   int
}
type bestPaths []rankedPath

func (h bestPaths) Len() int { return len(h) }
func (h bestPaths) Less(i, j int) bool {
	if h[i].rank != h[j].rank {
		return h[i].rank > h[j].rank
	}
	return h[i].record.Path > h[j].record.Path
}
func (h bestPaths) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *bestPaths) Push(v any)   { *h = append(*h, v.(rankedPath)) }
func (h *bestPaths) Pop() any     { old := *h; v := old[len(old)-1]; *h = old[:len(old)-1]; return v }

// SearchPaths bounds retained results while counting every match. It never mutates
// the catalog, so an immutable loaded catalog may be searched off the UI loop.
func (c Catalog) SearchPaths(ctx context.Context, query, machineID, scope string, limit int) ([]PathRecord, int, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	scope = strings.TrimSuffix(scope, "/")
	if limit < 1 {
		limit = 200
	}
	h := &bestPaths{}
	heap.Init(h)
	count := 0
	i := 0
	for path, record := range c.Paths {
		i++
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
		}
		if scope != "" && path != scope && !strings.HasPrefix(path, scope+"/") {
			continue
		}
		matchPath := path
		if scope != "" {
			matchPath = strings.TrimPrefix(strings.TrimPrefix(path, scope), "/")
		}
		if !MatchPath(matchPath, query) {
			continue
		}
		filtered := PathRecord{Path: record.Path, Name: record.Name}
		for _, v := range record.Versions {
			if machineID == "" || v.MachineID == machineID {
				filtered.Versions = append(filtered.Versions, v)
			}
		}
		if len(filtered.Versions) == 0 {
			continue
		}
		count++
		candidate := rankedPath{record: filtered, rank: score(path, query)}
		if len(*h) < limit {
			heap.Push(h, candidate)
		} else {
			worst := (*h)[0]
			if candidate.rank < worst.rank || candidate.rank == worst.rank && path < worst.record.Path {
				(*h)[0] = candidate
				heap.Fix(h, 0)
			}
		}
	}
	sort.Slice(*h, func(i, j int) bool {
		a, b := (*h)[i], (*h)[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return a.record.Path < b.record.Path
	})
	result := make([]PathRecord, len(*h))
	for i, v := range *h {
		result[i] = v.record
	}
	return result, count, ctx.Err()
}

func MatchPath(path, query string) bool { return fuzzy(strings.ToLower(path), strings.ToLower(query)) }
