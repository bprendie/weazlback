package freshrestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (r *Restore) stageCoreStandard(ctx context.Context) error {
	emitProgress(r.Options.Progress, RestoreProgress{Phase: r.Plan.Scope + " staging"})
	_ = os.RemoveAll(r.StageDir)
	if err := os.MkdirAll(r.StageDir, 0o700); err != nil {
		return err
	}
	if err := r.restoreSelection(ctx, r.StageDir, false); err != nil {
		return err
	}
	return r.validateStage()
}

func (r *Restore) restoreSelection(ctx context.Context, target string, includeHeavy bool) error {
	if r.Plan.HomeSnapshot != nil {
		if err := r.restorePoint(ctx, "Home", r.Plan.HomeSnapshot.ID, target); err != nil {
			return err
		}
	}
	if !r.Plan.includesCore() {
		_, err := removeWithheldCore(target, r.Plan.OriginalHome, r.Plan.ScopeDecision.WithheldClaims)
		return err
	}
	if includeHeavy && r.Plan.HeavySnapshot != nil {
		if err := r.restorePoint(ctx, "Heavy", r.Plan.HeavySnapshot.ID, target); err != nil {
			return err
		}
	}
	return r.restorePoint(ctx, "Core overlay", r.Plan.Snapshot.ID, target)
}

func (r *Restore) restoreAndCommitHeavy(ctx context.Context) ([]string, error) {
	if r.Plan.HeavySnapshot == nil {
		return nil, nil
	}
	target := r.StageDir + "-heavy"
	_ = os.RemoveAll(target)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return nil, err
	}
	var restoreErr error
	if r.Journal.Engine == EngineTurbo {
		restoreErr = r.restoreEmbeddedPoint(ctx, "Heavy", r.Plan.HeavySnapshot.ID, target)
	} else {
		restoreErr = r.restorePoint(ctx, "Heavy", r.Plan.HeavySnapshot.ID, target)
	}
	if restoreErr != nil {
		return nil, restoreErr
	}
	var restored []string
	for _, path := range r.Plan.HeavyPlacementPaths {
		if contains(r.Journal.CommittedPaths, path) {
			restored = append(restored, path)
			continue
		}
		rel, err := filepath.Rel(r.Plan.TargetHome, path)
		if err != nil {
			return restored, err
		}
		source := stagedPath(target, filepath.Join(r.Plan.OriginalHome, rel))
		if err := r.placeJournaled(source, path); err != nil {
			return restored, err
		}
		restored = append(restored, path)
	}
	return restored, nil
}

func (r *Restore) expectedTargets() []string {
	if !r.Plan.includesCore() && !r.Plan.includesHome() {
		return nil
	}
	if r.Plan.Scope != "" && r.Plan.Scope != "core" {
		return append([]string(nil), r.Plan.PlacementPaths...)
	}
	var targets []string
	for _, profile := range r.Session.Config.Profiles {
		if profile.Name != "core" {
			continue
		}
		for _, original := range profile.Includes {
			if target, err := mapHomePath(original, r.Plan.OriginalHome, r.Plan.TargetHome); err == nil {
				targets = append(targets, target)
			}
		}
	}
	return targets
}

func (r *Restore) commitSelected() ([]string, error) {
	if !r.Plan.includesCore() && !r.Plan.includesHome() {
		return nil, nil
	}
	if r.Plan.Scope == "" || r.Plan.Scope == "core" {
		return r.commitCore()
	}
	var restored []string
	for _, target := range r.Plan.PlacementPaths {
		rel, err := filepath.Rel(r.Plan.TargetHome, target)
		if err != nil {
			return restored, err
		}
		source := stagedPath(r.StageDir, filepath.Join(r.Plan.OriginalHome, rel))
		if contains(r.Journal.CommittedPaths, target) {
			restored = append(restored, target)
			continue
		}
		if err := r.placeJournaled(source, target); err != nil {
			return restored, err
		}
		restored = append(restored, target)
	}
	return restored, nil
}

func (r *Restore) advance(stage string) error {
	r.Journal.Stage = stage
	return SaveJournal(r.JournalPath, r.Journal)
}

func (r *Restore) validateStage() error {
	if r.Plan.OriginalHome == "" {
		return errors.New("Core profile does not identify the original home directory")
	}
	for _, target := range r.Plan.PlacementPaths {
		rel, err := filepath.Rel(r.Plan.TargetHome, target)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(stagedPath(r.StageDir, filepath.Join(r.Plan.OriginalHome, rel))); err != nil {
			return fmt.Errorf("staged Home path %s is missing: %w", target, err)
		}
	}
	if !r.Plan.includesCore() {
		return nil
	}
	for _, profile := range r.Session.Config.Profiles {
		if profile.Name != "core" {
			continue
		}
		for _, path := range profile.Includes {
			if _, err := os.Lstat(stagedPath(r.StageDir, path)); err != nil {
				return fmt.Errorf("staged Core path %s is missing: %w", path, err)
			}
		}
	}
	for _, app := range r.Plan.LocalApps {
		binDir := stagedPath(r.StageDir, filepath.Join(r.Plan.OriginalHome, "."+app, "bin"))
		entries, err := os.ReadDir(binDir)
		if err != nil {
			return fmt.Errorf("Weazl application %s payload is missing: %w", app, err)
		}
		found := false
		for _, entry := range entries {
			if info, infoErr := entry.Info(); infoErr == nil && !entry.IsDir() && info.Mode().Perm()&0o111 != 0 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Weazl application %s has no executable payload", app)
		}
	}
	return nil
}
