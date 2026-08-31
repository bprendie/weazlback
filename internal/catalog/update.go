package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

type ResticService interface {
	Files(context.Context, restic.Repository, string) ([]restic.FileEntry, error)
	FilesAt(context.Context, restic.Repository, string, []string) ([]restic.FileEntry, error)
	Diff(context.Context, restic.Repository, string, string) ([]restic.DiffChange, error)
}

func Refresh(ctx context.Context, v *vault.File, destinationID string, repo restic.Repository, machineID string, profiles ...string) error {
	path, err := Path(destinationID)
	if err != nil {
		return err
	}
	c, err := Load(path, v)
	if err != nil {
		c = New()
	}
	service := restic.NewService(nil)
	snapshots, err := service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if err := Update(ctx, &c, service, repo, snapshots, machineID, profile); err != nil {
			// Catalogs are disposable caches. A pruned or divergent chain is
			// rebuilt from repository truth instead of requiring repair.
			c = New()
			for _, rebuildProfile := range profiles {
				if rebuildErr := Update(ctx, &c, service, repo, snapshots, machineID, rebuildProfile); rebuildErr != nil {
					return rebuildErr
				}
			}
			break
		}
	}
	return Save(path, c, v)
}

func Update(ctx context.Context, c *Catalog, service ResticService, repo restic.Repository, snapshots []restic.Snapshot, machineID, profile string) error {
	var chain []restic.Snapshot
	for _, snapshot := range snapshots {
		if restic.MachineID(snapshot.Tags) == machineID && restic.Profile(snapshot.Tags) == profile {
			chain = append(chain, snapshot)
		}
	}
	if len(chain) == 0 {
		return nil
	}
	sort.Slice(chain, func(i, j int) bool { return chain[i].Time.Before(chain[j].Time) })
	key := ChainKey(machineID, profile)
	latest := c.Chains[key].Latest
	start := 0
	if latest == "" {
		files, err := service.Files(ctx, repo, chain[0].ID)
		if err != nil {
			return fmt.Errorf("catalog baseline: %w", err)
		}
		c.Baseline(chain[0], files)
		latest, start = chain[0].ID, 1
	}
	for i := range chain {
		if chain[i].ID == latest {
			start = i + 1
			break
		}
	}
	if start == 0 && latest != chain[0].ID {
		return fmt.Errorf("catalog chain head %s is no longer present; rebuild required", latest)
	}
	previous := latest
	for _, snapshot := range chain[start:] {
		changes, err := service.Diff(ctx, repo, previous, snapshot.ID)
		if err != nil {
			return fmt.Errorf("catalog diff %s..%s: %w", previous, snapshot.ID, err)
		}
		c.Apply(snapshot, changes)
		var changedPaths []string
		for _, change := range changes {
			if !strings.Contains(change.Modifier, "-") {
				changedPaths = append(changedPaths, change.Path)
			}
		}
		if len(changedPaths) > 0 {
			files, filesErr := service.FilesAt(ctx, repo, snapshot.ID, changedPaths)
			if filesErr != nil {
				return fmt.Errorf("catalog changed metadata: %w", filesErr)
			}
			c.Enrich(snapshot.ID, files)
		}
		previous = snapshot.ID
	}
	return nil
}
