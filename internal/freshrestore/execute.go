package freshrestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func (r *Restore) Execute(ctx context.Context, stdout io.Writer) (Report, error) {
	emitProgress(r.Options.Progress, RestoreProgress{Phase: "starting"})
	report := Report{SnapshotID: r.Plan.Snapshot.ID, Hostname: r.Plan.Hostname,
		ManualReview:  append([]string(nil), r.Plan.Applications.ManualReview...),
		PackageErrors: append([]string(nil), r.Journal.PackageErrors...), HeavyDeferred: r.Plan.Scope != "everything", JournalPath: r.JournalPath}
	if r.Options.DryRun {
		return report, nil
	}
	if err := r.repairInconsistentJournal(); err != nil {
		return report, err
	}
	currentHostname, _ := os.Hostname()
	if r.Plan.Hostname != currentHostname || len(r.Plan.Official)+len(r.Plan.AUR)+len(r.Plan.SystemServices) > 0 {
		if err := AuthorizeSudo(); err != nil {
			return report, fmt.Errorf("sudo authorization: %w", err)
		}
		stopSudo := keepSudoAlive(ctx)
		defer stopSudo()
	}
	if !stageAtLeast(r.Journal.Stage, "plan_confirmed") {
		if err := r.advance("plan_confirmed"); err != nil {
			return report, err
		}
	}
	if !stageAtLeast(r.Journal.Stage, "hostname_applied") {
		if r.Plan.Scope != "applications" {
			if err := ApplyHostname(r.Plan.Hostname); err != nil {
				return report, fmt.Errorf("apply hostname: %w", err)
			}
		}
		if err := r.advance("hostname_applied"); err != nil {
			return report, err
		}
	}
	if r.Plan.Scope == "applications" {
		if !stageAtLeast(r.Journal.Stage, "packages_reconciled") {
			report.PackageErrors = reconcileApplicationLanes(ctx, r.Plan, r.Options.Progress, true, true, 0)
			r.Journal.PackageErrors = append([]string(nil), report.PackageErrors...)
			if err := r.advance("packages_reconciled"); err != nil {
				return report, err
			}
		}
	} else if !stageAtLeast(r.Journal.Stage, "packages_reconciled") {
		stageErrors := make(chan error, 1)
		go func() { stageErrors <- r.stageCore(ctx) }()
		report.PackageErrors = reconcileApplicationLanes(ctx, r.Plan, r.Options.Progress, true, false, 0)
		if err := <-stageErrors; err != nil {
			return report, err
		}
		if err := r.advance("core_staged"); err != nil {
			return report, err
		}
		r.Journal.PackageErrors = append([]string(nil), report.PackageErrors...)
		if err := r.advance("packages_reconciled"); err != nil {
			return report, err
		}
	} else if !stageAtLeast(r.Journal.Stage, "core_staged") {
		if err := r.stageCore(ctx); err != nil {
			return report, err
		}
		if err := r.advance("core_staged"); err != nil {
			return report, err
		}
	}
	if r.Plan.Scope != "applications" && !stageAtLeast(r.Journal.Stage, "core_committed") {
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "core placement"})
		paths, err := r.commitSelected()
		if err != nil {
			return report, err
		}
		report.RestoredPaths = paths
		if err := r.advance("core_committed"); err != nil {
			return report, err
		}
	}
	if r.Plan.Scope != "applications" {
		if err := r.validateCommit(); err != nil {
			return report, err
		}
	}
	report.RestoredPaths = append([]string(nil), r.Journal.CommittedPaths...)
	if r.Plan.Scope != "applications" && !stageAtLeast(r.Journal.Stage, "user_state_reconciled") {
		offset := len(r.Plan.Official) + len(r.Plan.AUR) + len(r.Plan.SystemServices)
		report.PackageErrors = append(report.PackageErrors, reconcileApplicationLanes(ctx, r.Plan, r.Options.Progress, false, true, offset)...)
		r.Journal.PackageErrors = append([]string(nil), report.PackageErrors...)
		if err := r.advance("user_state_reconciled"); err != nil {
			return report, err
		}
	}
	if r.Plan.Scope == "everything" && !stageAtLeast(r.Journal.Stage, "heavy_committed") {
		heavyPaths, err := r.restoreAndCommitHeavy(ctx)
		if err != nil {
			return report, err
		}
		report.RestoredPaths = append(report.RestoredPaths, heavyPaths...)
		if err := r.advance("heavy_committed"); err != nil {
			return report, err
		}
	}
	if err := r.advance("system_validated"); err != nil {
		return report, err
	}
	if r.Options.AdoptSourceIdentity {
		if err := r.adoptSourceIdentity(); err != nil {
			return report, fmt.Errorf("adopt source machine identity: %w", err)
		}
	}
	if r.Options.PersistTargetIdentity && !r.Options.AdoptSourceIdentity {
		if err := r.persistTargetIdentity(); err != nil {
			return report, fmt.Errorf("persist target machine identity: %w", err)
		}
	}
	report.Complete = len(report.PackageErrors) == 0
	if report.Complete {
		if err := r.advance("complete"); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (r *Restore) stageCore(ctx context.Context) error {
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
	if err := r.restorePoint(ctx, "Heavy", r.Plan.HeavySnapshot.ID, target); err != nil {
		return nil, err
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

func (r *Restore) restorePoint(ctx context.Context, label, snapshot, target string) error {
	started := time.Now()
	return r.service.RestoreWithProgress(ctx, r.Session.Repository, snapshot, target, nil, func(value restic.RestoreProgress) {
		total, completed := int(value.TotalFiles), int(value.FilesRestored)
		if value.MessageType == "summary" && total > 0 {
			completed = total
		}
		elapsed := time.Since(started).Seconds()
		bytesRate, filesRate := 0.0, 0.0
		if elapsed > 0 {
			bytesRate = float64(value.BytesRestored) / elapsed
			filesRate = float64(value.FilesRestored) / elapsed
		}
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "filesystem", Lane: label, Current: "decrypting and extracting", Completed: completed, Total: total,
			BytesDone: value.BytesRestored, BytesTotal: value.TotalBytes, BytesPerSecond: bytesRate, FilesPerSecond: filesRate})
	})
}

func (r *Restore) expectedTargets() []string {
	if r.Plan.Scope != "" && r.Plan.Scope != "core" {
		return append([]string(nil), r.Plan.PlacementPaths...)
	}
	var targets []string
	for _, profile := range r.Session.Config.Profiles {
		if profile.Name == "core" {
			for _, original := range profile.Includes {
				if target, err := mapHomePath(original, r.Plan.OriginalHome, r.Plan.TargetHome); err == nil {
					targets = append(targets, target)
				}
			}
		}
	}
	return targets
}

func (r *Restore) commitSelected() ([]string, error) {
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
