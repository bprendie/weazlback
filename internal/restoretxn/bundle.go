package restoretxn

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

type Bundle string

const (
	SystemConfig  Bundle = "system-config"
	PersonalFiles Bundle = "personal-files"
	HeavyData     Bundle = "vms-containers"
)

type Component struct {
	Bundle    Bundle
	MachineID string
	Profile   string
	Snapshot  restic.Snapshot
	Paths     []string
}

func ComposeNearest(snapshots []restic.Snapshot, machineID string, requested time.Time, profiles map[Bundle]string) ([]Component, error) {
	var result []Component
	for bundle, profile := range profiles {
		var candidates []restic.Snapshot
		for _, snapshot := range snapshots {
			if restic.MachineID(snapshot.Tags) == machineID && restic.Profile(snapshot.Tags) == profile && restic.SnapshotHealth(snapshot.Tags) == "healthy" {
				candidates = append(candidates, snapshot)
			}
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no healthy %s Restore Point for machine %s", profile, machineID)
		}
		sort.Slice(candidates, func(i, j int) bool {
			left, right := distance(candidates[i].Time, requested), distance(candidates[j].Time, requested)
			if left == right {
				return candidates[i].Time.Before(candidates[j].Time)
			}
			return left < right
		})
		result = append(result, Component{Bundle: bundle, MachineID: machineID, Profile: profile, Snapshot: candidates[0], Paths: append([]string(nil), candidates[0].Paths...)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Bundle < result[j].Bundle })
	return result, nil
}

func distance(left, right time.Time) time.Duration {
	value := left.Sub(right)
	if value < 0 {
		return -value
	}
	return value
}

// ExactDeletions returns live paths absent from repositoryPaths and proven to
// reside beneath an approved bundle boundary. Symlinks are never followed.
func ExactDeletions(boundary, sourceBoundary, targetBoundary string, repositoryPaths []string) ([]string, error) {
	boundary, sourceBoundary, targetBoundary = filepath.Clean(boundary), filepath.Clean(sourceBoundary), filepath.Clean(targetBoundary)
	if !inside(boundary, targetBoundary) {
		return nil, fmt.Errorf("exact rewind boundary escapes approved target: %s", boundary)
	}
	expected := map[string]bool{}
	for _, source := range repositoryPaths {
		relative, err := filepath.Rel(sourceBoundary, filepath.Clean(source))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		expected[filepath.Join(targetBoundary, relative)] = true
	}
	var deletions []string
	err := filepath.Walk(boundary, func(path string, _ os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != boundary && !expected[path] {
			deletions = append(deletions, path)
		}
		return nil
	})
	sort.Slice(deletions, func(i, j int) bool {
		if len(deletions[i]) == len(deletions[j]) {
			return deletions[i] < deletions[j]
		}
		return len(deletions[i]) > len(deletions[j])
	})
	return deletions, err
}

func inside(path, boundary string) bool {
	relative, err := filepath.Rel(boundary, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
