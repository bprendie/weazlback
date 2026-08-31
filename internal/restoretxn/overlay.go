package restoretxn

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (e Engine) overlayDirectory(plan Plan, journal *Journal, sourceRoot, targetRoot string, progress func(Progress)) ([]string, []string, error) {
	totalFiles, totalBytes := pathTotals(sourceRoot)
	var doneFiles, doneBytes uint64
	var placed, rollbacks []string
	started := time.Now()
	err := filepath.Walk(sourceRoot, func(source string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceRoot, source)
		if err != nil {
			return err
		}
		target := filepath.Join(targetRoot, relative)
		if info.IsDir() {
			if existing, statErr := os.Lstat(target); statErr == nil && existing.IsDir() {
				return nil
			}
			rollback := plannedRollback(target, e.Now())
			if rollback != "" {
				if err := os.Rename(target, rollback); err != nil {
					return err
				}
			}
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				_ = rollbackOne(target, rollback)
				return err
			}
			journal.Paths[target] = PathState{Target: target, Rollback: rollback, State: "created-dir"}
			return saveJournal(plan.JournalPath, e.Cryptor, *journal)
		}
		state := journal.Paths[target]
		if state.State == "placed" {
			return nil
		}
		rollback := plannedRollback(target, e.Now())
		state = PathState{Source: source, Target: target, Rollback: rollback, State: "placing"}
		journal.Paths[target] = state
		if err := saveJournal(plan.JournalPath, e.Cryptor, *journal); err != nil {
			return err
		}
		if err := placeAtomic(source, target, rollback); err != nil {
			return err
		}
		if plan.SourceUID != plan.TargetUID || plan.SourceGID != plan.TargetGID {
			if err := mapOwnership(target, plan); err != nil {
				_ = rollbackOne(target, rollback)
				return err
			}
		}
		state.State = "placed"
		journal.Paths[target] = state
		if err := saveJournal(plan.JournalPath, e.Cryptor, *journal); err != nil {
			_ = rollbackOne(target, rollback)
			return err
		}
		placed = append(placed, target)
		if rollback != "" {
			rollbacks = append(rollbacks, rollback)
		}
		doneFiles++
		if info.Mode().IsRegular() {
			doneBytes += uint64(info.Size())
		}
		emitPlacementProgress(progress, started, doneFiles, totalFiles, doneBytes, totalBytes)
		return nil
	})
	if err != nil {
		return placed, rollbacks, fmt.Errorf("safe overlay: %w", err)
	}
	return placed, rollbacks, nil
}
