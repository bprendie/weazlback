package restoretxn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

type Engine struct {
	Service Service
	Cryptor Cryptor
	Now     func() time.Time
}

func (e Engine) Run(ctx context.Context, plan Plan, progress func(Progress)) (Result, error) {
	result := Result{JournalPath: plan.JournalPath, StagedAt: plan.StageRoot}
	if e.Service == nil || e.Cryptor == nil {
		return result, fmt.Errorf("restore transaction engine is not initialized")
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if _, err := PreflightPlan(ctx, e.Service, &plan); err != nil {
		return result, err
	}
	journal, err := loadJournal(plan.JournalPath, e.Cryptor, plan)
	if err != nil {
		return result, err
	}
	if journal.Stage == "planned" {
		if _, statErr := os.Stat(plan.JournalPath); os.IsNotExist(statErr) {
			if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
				return result, err
			}
		}
		if err := os.MkdirAll(plan.StageRoot, 0o700); err != nil {
			return result, err
		}
		includes := make([]string, len(plan.Items))
		for i := range plan.Items {
			includes[i] = plan.Items[i].SourcePath
		}
		started := e.Now()
		err = e.Service.RestoreWithProgress(ctx, plan.Repository, plan.Snapshot, plan.StageRoot, includes, func(value restic.RestoreProgress) {
			if progress == nil {
				return
			}
			elapsed := e.Now().Sub(started)
			rate := float64(0)
			if elapsed > 0 {
				rate = float64(value.BytesRestored) / elapsed.Seconds()
			}
			progress(Progress{Phase: "extraction", FilesDone: value.FilesRestored, FilesTotal: value.TotalFiles,
				BytesDone: value.BytesRestored, BytesTotal: value.TotalBytes, BytesPerSecond: rate, Elapsed: elapsed})
		})
		if err != nil {
			return result, fmt.Errorf("extract selected paths: %w", err)
		}
		journal.Stage = "extracted"
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return result, err
		}
	}
	if journal.Stage == "extracted" {
		if err := validateStaged(plan); err != nil {
			return result, err
		}
		journal.Stage = "validated"
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return result, err
		}
	}
	if plan.StagingOnly {
		return result, nil
	}
	totalFiles, totalBytes := stagedTotals(plan)
	var doneFiles, doneBytes uint64
	started := e.Now()
	for _, item := range plan.Items {
		state := journal.Paths[item.TargetPath]
		if state.State == "placed" || state.State == "skipped" {
			if state.State == "placed" {
				result.Placed = append(result.Placed, item.TargetPath)
			}
			continue
		}
		staged := stagedPath(plan.StageRoot, item.SourcePath)
		if plan.Conflict == OverlayPreserving {
			if info, statErr := os.Lstat(staged); statErr == nil && info.IsDir() {
				placed, rollbacks, overlayErr := e.overlayDirectory(plan, &journal, staged, item.TargetPath, progress)
				result.Placed = append(result.Placed, placed...)
				result.Rollback = append(result.Rollback, rollbacks...)
				if overlayErr != nil {
					return result, overlayErr
				}
				continue
			}
		}
		if state.State == "placing" {
			if validatePair(staged, item.TargetPath) == nil {
				state.State = "placed"
				journal.Paths[item.TargetPath] = state
				if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
					return result, err
				}
				result.Placed = append(result.Placed, item.TargetPath)
				if state.Rollback != "" {
					result.Rollback = append(result.Rollback, state.Rollback)
				}
				continue
			}
			if _, rollbackErr := os.Lstat(state.Rollback); rollbackErr == nil {
				if err := rollbackOne(item.TargetPath, state.Rollback); err != nil {
					return result, err
				}
			}
		}
		files, bytes := pathTotals(staged)
		if plan.Conflict == SkipExisting {
			if _, statErr := os.Lstat(item.TargetPath); statErr == nil {
				journal.Paths[item.TargetPath] = PathState{Source: staged, Target: item.TargetPath, State: "skipped"}
				result.Skipped = append(result.Skipped, item.TargetPath)
				if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
					return result, err
				}
				continue
			}
		}
		rollback := plannedRollback(item.TargetPath, e.Now())
		journal.Paths[item.TargetPath] = PathState{Source: staged, Target: item.TargetPath, Rollback: rollback, State: "placing"}
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return result, err
		}
		if err := placeAtomic(staged, item.TargetPath, rollback); err != nil {
			return result, err
		}
		if plan.SourceUID != plan.TargetUID || plan.SourceGID != plan.TargetGID {
			if mapErr := mapOwnership(item.TargetPath, plan); mapErr != nil {
				_ = rollbackOne(item.TargetPath, rollback)
				return result, mapErr
			}
		}
		if err := validatePair(staged, item.TargetPath); err != nil {
			_ = rollbackOne(item.TargetPath, rollback)
			return result, err
		}
		journal.Paths[item.TargetPath] = PathState{Source: staged, Target: item.TargetPath, Rollback: rollback, State: "placed"}
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			_ = rollbackOne(item.TargetPath, rollback)
			return result, err
		}
		result.Placed = append(result.Placed, item.TargetPath)
		if rollback != "" {
			result.Rollback = append(result.Rollback, rollback)
		}
		doneFiles, doneBytes = doneFiles+files, doneBytes+bytes
		emitPlacementProgress(progress, started, doneFiles, totalFiles, doneBytes, totalBytes)
	}
	journal.Stage = "placed"
	if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
		return result, err
	}
	return result, nil
}

func stagedPath(root, path string) string {
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator)))
}

func emitPlacementProgress(callback func(Progress), started time.Time, files, totalFiles, bytes, totalBytes uint64) {
	if callback == nil {
		return
	}
	elapsed := time.Since(started)
	rate := float64(0)
	if elapsed > 0 {
		rate = float64(bytes) / elapsed.Seconds()
	}
	remaining := time.Duration(0)
	if rate > 0 && totalBytes > bytes {
		remaining = time.Duration(float64(totalBytes-bytes)/rate) * time.Second
	}
	callback(Progress{Phase: "placement", FilesDone: files, FilesTotal: totalFiles, BytesDone: bytes, BytesTotal: totalBytes,
		BytesPerSecond: rate, Elapsed: elapsed, EstimatedRemain: remaining})
}
