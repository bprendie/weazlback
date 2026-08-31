package freshrestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/restic"
)

func (r *Restore) restoreApplicationManifest(ctx context.Context) (inventory.ApplicationManifest, error) {
	var manifest inventory.ApplicationManifest
	files, err := r.service.Files(ctx, r.Session.Repository, r.Plan.Snapshot.ID)
	if err != nil {
		return manifest, err
	}
	manifestPath := ""
	for _, file := range files {
		if filepath.Base(file.Path) == backupmeta.ManifestName {
			manifestPath = file.Path
			break
		}
	}
	if manifestPath == "" {
		return manifest, errors.New("Core Restore Point has no application manifest")
	}
	metaDir := filepath.Join(r.Options.WorkDir, ".manifest")
	_ = os.RemoveAll(metaDir)
	include := filepath.Dir(manifestPath)
	if err := r.service.Restore(ctx, r.Session.Repository, r.Plan.Snapshot.ID, metaDir, []string{include}); err != nil {
		return manifest, err
	}
	b, err := os.ReadFile(stagedPath(metaDir, manifestPath))
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return manifest, err
	}
	if err := inventory.ValidateApplications(manifest); err != nil {
		return manifest, err
	}
	r.Plan.ArtifactFiles = map[string]string{}
	for _, artifact := range manifest.AURArtifacts {
		r.Plan.ArtifactFiles[artifact.Package] = stagedPath(metaDir, filepath.Join(include, "aur-artifacts", artifact.File))
	}
	return manifest, nil
}

func selectCoreSnapshot(snapshots []restic.Snapshot, selected string) (restic.Snapshot, error) {
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
	for _, snapshot := range snapshots {
		if selected != "" && selected != "latest" && snapshot.ID != selected && snapshot.ShortID != selected {
			continue
		}
		for _, tag := range snapshot.Tags {
			if tag == "profile:core" {
				return snapshot, nil
			}
		}
	}
	return restic.Snapshot{}, errors.New("no matching healthy Core Restore Point found")
}

func selectProfileSnapshotAt(snapshots []restic.Snapshot, profile string, requested time.Time) (restic.Snapshot, error) {
	var matches []restic.Snapshot
	for _, snapshot := range snapshots {
		if restic.Profile(snapshot.Tags) == profile && restic.SnapshotHealth(snapshot.Tags) == "healthy" {
			matches = append(matches, snapshot)
		}
	}
	if len(matches) == 0 {
		return restic.Snapshot{}, fmt.Errorf("no matching healthy %s Restore Point found", profile)
	}
	sort.Slice(matches, func(i, j int) bool {
		left, right := distance(matches[i].Time, requested), distance(matches[j].Time, requested)
		if left == right {
			return matches[i].Time.Before(matches[j].Time)
		}
		return left < right
	})
	return matches[0], nil
}

func distance(left, right time.Time) time.Duration {
	value := left.Sub(right)
	if value < 0 {
		return -value
	}
	return value
}
