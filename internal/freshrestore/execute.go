package freshrestore

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

func (r *Restore) Execute(ctx context.Context, stdout io.Writer) (Report, error) {
	emitProgress(r.Options.Progress, RestoreProgress{Phase: "starting"})
	report := Report{SnapshotID: r.Plan.Snapshot.ID, Hostname: r.Plan.Hostname,
		ManualReview:  append([]string(nil), r.Plan.Applications.ManualReview...),
		PackageErrors: append([]string(nil), r.Journal.PackageErrors...), CapsuleInstalled: append([]string(nil), r.Journal.CapsuleInstalled...),
		CapsuleFallback: append([]string(nil), r.Journal.CapsuleFallback...), CapsuleFallbackReason: r.Journal.CapsuleFallbackReason,
		HeavyDeferred: r.Plan.Scope != "everything", JournalPath: r.JournalPath,
		Engine: r.Journal.Engine, FallbackReason: r.Journal.FallbackReason, Qualification: r.Journal.Qualification, Timing: r.Journal.Timing}
	if r.Journal.Timing.StartedAt.IsZero() {
		r.Journal.Timing.StartedAt = time.Now().UTC()
	}
	if r.Plan.PackageCapsule != nil {
		report.ManualReview = append(report.ManualReview, r.Plan.PackageCapsule.ManualReview...)
	}
	if r.Options.DryRun {
		return report, nil
	}
	if err := r.repairInconsistentJournal(); err != nil {
		return report, err
	}
	currentHostname, _ := os.Hostname()
	if r.Plan.includesCore() && r.Plan.Hostname != currentHostname || len(r.Plan.Official)+len(r.Plan.AUR)+len(r.Plan.SystemServices)+len(r.Plan.PackageDelta.Local) > 0 {
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
		if r.Plan.Scope != "applications" && r.Plan.includesCore() {
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
			installed, fallback, capsuleErrors := r.reconcilePackageCapsule(ctx)
			report.CapsuleInstalled, report.CapsuleFallback = installed, fallback
			report.CapsuleFallbackReason = r.Plan.CapsuleFallbackReason
			report.PackageErrors = append(capsuleErrors, reconcileApplicationLanes(ctx, r.Plan, r.Options.Progress, true, true, 0)...)
			r.Journal.CapsuleInstalled, r.Journal.CapsuleFallback = append([]string(nil), installed...), append([]string(nil), fallback...)
			r.Journal.CapsuleFallbackReason = report.CapsuleFallbackReason
			r.Journal.PackageErrors = append([]string(nil), report.PackageErrors...)
			r.Journal.Timing.PackagesDoneAt = time.Now().UTC()
			r.Journal.Timing.UsableAt = r.Journal.Timing.PackagesDoneAt
			r.Journal.Timing.TimeToUsable = r.Journal.Timing.UsableAt.Sub(r.Journal.Timing.StartedAt)
			if err := r.advance("packages_reconciled"); err != nil {
				return report, err
			}
		}
	} else if !stageAtLeast(r.Journal.Stage, "packages_reconciled") {
		stageErrors := make(chan error, 1)
		go func() { stageErrors <- r.stageWithFallback(ctx) }()
		installed, fallback, capsuleErrors := r.reconcilePackageCapsule(ctx)
		report.CapsuleInstalled, report.CapsuleFallback = installed, fallback
		report.CapsuleFallbackReason = r.Plan.CapsuleFallbackReason
		report.PackageErrors = append(capsuleErrors, reconcileApplicationLanes(ctx, r.Plan, r.Options.Progress, true, false, 0)...)
		packagesDoneAt := time.Now().UTC()
		if err := <-stageErrors; err != nil {
			return report, err
		}
		if err := r.advance("core_staged"); err != nil {
			return report, err
		}
		r.Journal.PackageErrors = append([]string(nil), report.PackageErrors...)
		r.Journal.CapsuleInstalled, r.Journal.CapsuleFallback = append([]string(nil), installed...), append([]string(nil), fallback...)
		r.Journal.CapsuleFallbackReason = report.CapsuleFallbackReason
		r.Journal.Timing.PackagesDoneAt = packagesDoneAt
		if err := r.advance("packages_reconciled"); err != nil {
			return report, err
		}
	} else if !stageAtLeast(r.Journal.Stage, "core_staged") {
		if err := r.stageWithFallback(ctx); err != nil {
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
		if r.Journal.Timing.UsableAt.IsZero() {
			r.Journal.Timing.UsableAt = time.Now().UTC()
			r.Journal.Timing.TimeToUsable = r.Journal.Timing.UsableAt.Sub(r.Journal.Timing.StartedAt)
		}
	}
	report.RestoredPaths = append([]string(nil), r.Journal.CommittedPaths...)
	if r.Plan.Scope != "applications" && !stageAtLeast(r.Journal.Stage, "browser_compatibility") {
		result, issues := r.repairBrowserCompatibility()
		r.Journal.BrowserRepair, r.Journal.BrowserIssues = result, issues
		if err := r.advance("browser_compatibility"); err != nil {
			return report, err
		}
	}
	report.BrowserRepair = r.Journal.BrowserRepair
	report.BrowserIssues = append([]string(nil), r.Journal.BrowserIssues...)
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
	report.Complete = len(report.PackageErrors) == 0 && len(report.BrowserIssues) == 0
	if err := syncPathFilesystems(r.Journal.CommittedPaths); err != nil {
		return report, fmt.Errorf("durability sync: %w", err)
	}
	r.Journal.Timing.DurableAt = time.Now().UTC()
	r.Journal.Timing.TimeToDurable = r.Journal.Timing.DurableAt.Sub(r.Journal.Timing.StartedAt)
	if err := SaveJournal(r.JournalPath, r.Journal); err != nil {
		return report, err
	}
	if err := r.cleanupTurboStage(); err != nil {
		return report, fmt.Errorf("Turbo staging cleanup: %w", err)
	}
	if r.Options.TurboPolicy.Recompress && r.Journal.Qualification.TargetFilesystem == "btrfs" {
		r.Journal.Recompression = "running"
		if err := SaveJournal(r.JournalPath, r.Journal); err != nil {
			return report, err
		}
		if err := recompressBtrfs(ctx, r.Journal.CommittedPaths); err != nil {
			r.Journal.Recompression = "failed: " + err.Error()
			report.ManualReview = append(report.ManualReview, "optional Btrfs recompression failed")
		} else {
			r.Journal.Recompression = "complete"
		}
		_ = SaveJournal(r.JournalPath, r.Journal)
	}
	report.Engine, report.FallbackReason = r.Journal.Engine, r.Journal.FallbackReason
	report.Qualification, report.Timing = r.Journal.Qualification, r.Journal.Timing
	report.Recompression = r.Journal.Recompression
	if report.Complete {
		if err := r.advance("complete"); err != nil {
			return report, err
		}
	}
	return report, nil
}
