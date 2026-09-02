package freshrestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
)

func fileBytes(files []restic.FileEntry) uint64 {
	var total uint64
	for _, file := range files {
		if file.Type == "file" {
			total += file.Size
		}
	}
	return total
}

func topLevelTargets(files []restic.FileEntry, oldHome, targetHome string) []string {
	var targets []string
	for _, file := range files {
		rel, err := filepath.Rel(oldHome, file.Path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		first := strings.Split(rel, string(filepath.Separator))[0]
		targets = appendUniqueStrings(targets, filepath.Join(targetHome, first))
	}
	sort.Strings(targets)
	return targets
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func selectProfileSnapshot(snapshots []restic.Snapshot, profile string) (restic.Snapshot, error) {
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
	for _, snapshot := range snapshots {
		for _, tag := range snapshot.Tags {
			if tag == "profile:"+profile {
				return snapshot, nil
			}
		}
	}
	return restic.Snapshot{}, fmt.Errorf("no healthy %s Restore Point found", profile)
}

func snapshotOwner(files []restic.FileEntry, wanted string) (uint32, uint32) {
	for _, file := range files {
		if filepath.Clean(file.Path) == filepath.Clean(wanted) {
			return file.UID, file.GID
		}
	}
	return 0, 0
}

func (r *Restore) Close() { r.Session.Close() }

func (r *Restore) StagePreview(ctx context.Context) (string, error) {
	target := filepath.Join(r.Options.WorkDir, "preview-"+r.Plan.Snapshot.ShortID)
	_ = os.RemoveAll(target)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	if err := r.restoreSelection(ctx, target, true); err != nil {
		return "", err
	}
	originalStage := r.StageDir
	r.StageDir = target
	err := r.validateStage()
	r.StageDir = originalStage
	if err != nil {
		return "", err
	}
	return target, nil
}
