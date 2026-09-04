package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

type CaptureLane func(profile, generationID string) error

func ExecuteParallel(ctx context.Context, service restic.Service, repo restic.Repository, machineID, action, wanted string, capture CaptureLane) (string, error) {
	points, err := service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		return "", err
	}
	id := wanted
	if action == "retry" {
		if id == "" {
			id = latestIncomplete(Catalog(points), machineID)
		}
		if id == "" {
			return "", errors.New("no incomplete System Snapshot to retry")
		}
	} else if id == "" {
		id, err = NewID(time.Now())
		if err != nil {
			return "", err
		}
	}
	existing := members(points, id)
	if _, ok := existing["generation-ledger"]; !ok {
		if err := capture("generation-ledger", id); err != nil {
			return id, err
		}
	}
	type laneError struct {
		profile string
		err     error
	}
	errorsOut := make(chan laneError, len(RequiredProfiles))
	var group sync.WaitGroup
	for _, profile := range RequiredProfiles {
		if _, ok := existing[profile]; ok {
			continue
		}
		profile := profile
		group.Add(1)
		go func() {
			defer group.Done()
			if err := capture(profile, id); err != nil {
				errorsOut <- laneError{profile, err}
			}
		}()
	}
	group.Wait()
	close(errorsOut)
	var failures []string
	for failure := range errorsOut {
		failures = append(failures, failure.profile+": "+failure.err.Error())
	}
	if len(failures) > 0 {
		mark(ctx, service, repo, id, TagFailed)
		return id, fmt.Errorf("System Snapshot incomplete: %s", strings.Join(failures, "; "))
	}
	points, err = service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		return id, err
	}
	existing = members(points, id)
	if !HasAll(Generation{Members: existing}, RequiredProfiles) {
		return id, errors.New("generation validation failed: required lanes are missing")
	}
	if err := service.TagSnapshots(ctx, repo, snapshotIDs(existing), []string{TagComplete}, []string{TagFailed, TagAbandoned}); err != nil {
		return id, err
	}
	return id, nil
}

func Execute(ctx context.Context, service restic.Service, repo restic.Repository, machineID, action, wanted string, capture CaptureLane) (string, error) {
	points, err := service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		return "", err
	}
	id := wanted
	if action == "retry" {
		if id == "" {
			id = latestIncomplete(Catalog(points), machineID)
		}
		if id == "" {
			return "", errors.New("no incomplete System Snapshot to retry")
		}
	} else if id == "" {
		id, err = NewID(time.Now())
		if err != nil {
			return "", err
		}
	}
	existing := members(points, id)
	lanes := append([]string{"generation-ledger"}, RequiredProfiles...)
	for _, profile := range lanes {
		if _, ok := existing[profile]; ok {
			continue
		}
		if err := capture(profile, id); err != nil {
			mark(ctx, service, repo, id, TagFailed)
			return id, fmt.Errorf("System Snapshot %s incomplete at %s: %w", id, profile, err)
		}
	}
	points, err = service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		return id, err
	}
	existing = members(points, id)
	if !HasAll(Generation{Members: existing}, RequiredProfiles) {
		return id, errors.New("generation validation failed: required lanes are missing")
	}
	if err := service.TagSnapshots(ctx, repo, snapshotIDs(existing), []string{TagComplete}, []string{TagFailed, TagAbandoned}); err != nil {
		return id, err
	}
	return id, nil
}

func members(points []restic.Snapshot, id string) map[string]restic.Snapshot {
	result := map[string]restic.Snapshot{}
	for _, point := range points {
		if ID(point.Tags) != id {
			continue
		}
		profile := restic.Profile(point.Tags)
		if old, ok := result[profile]; !ok || point.Time.After(old.Time) {
			result[profile] = point
		}
	}
	return result
}

func snapshotIDs(points map[string]restic.Snapshot) []string {
	result := make([]string, 0, len(points))
	for _, point := range points {
		result = append(result, point.ID)
	}
	return result
}

func latestIncomplete(gens []Generation, machineID string) string {
	for _, g := range gens {
		if g.MachineID == machineID && !g.Complete && !g.Abandoned {
			return g.ID
		}
	}
	return ""
}

func mark(ctx context.Context, service restic.Service, repo restic.Repository, id, tag string) {
	points, _ := service.Snapshots(ctx, repo)
	selected := members(points, id)
	if len(selected) > 0 {
		_ = service.TagSnapshots(ctx, repo, snapshotIDs(selected), []string{tag}, nil)
	}
}
