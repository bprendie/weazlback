package restoretxn

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (e Engine) Rollback(plan Plan) error {
	journal, err := loadJournal(plan.JournalPath, e.Cryptor, plan)
	if err != nil {
		return err
	}
	var targets []string
	for target := range journal.Paths {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool { return len(targets[i]) > len(targets[j]) })
	for _, target := range targets {
		state := journal.Paths[target]
		if state.State != "placed" && state.State != "deleted" && state.State != "created-dir" {
			continue
		}
		if err := rollbackOne(target, state.Rollback); err != nil {
			return fmt.Errorf("rollback %s: %w", target, err)
		}
		state.State = "rolled-back"
		journal.Paths[target] = state
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return err
		}
	}
	journal.Stage = "rolled-back"
	return saveJournal(plan.JournalPath, e.Cryptor, journal)
}

func (e Engine) ApplyExactDeletions(plan Plan, paths []string) error {
	journal, err := loadJournal(plan.JournalPath, e.Cryptor, plan)
	if err != nil {
		return err
	}
	for _, target := range topLevelDeletions(paths) {
		state := journal.Paths[target]
		if state.State == "deleted" {
			continue
		}
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		}
		rollback := target + ".weazlback-rewind-" + plan.ID
		if _, err := os.Lstat(rollback); err == nil {
			return fmt.Errorf("exact rewind rollback path already exists: %s", rollback)
		}
		state = PathState{Target: target, Rollback: rollback, State: "deleting"}
		journal.Paths[target] = state
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return err
		}
		if err := os.Rename(target, rollback); err != nil {
			return fmt.Errorf("preserve exact-rewind deletion %s: %w", target, err)
		}
		state.State = "deleted"
		journal.Paths[target] = state
		if err := saveJournal(plan.JournalPath, e.Cryptor, journal); err != nil {
			return err
		}
	}
	journal.Stage = "rewound"
	return saveJournal(plan.JournalPath, e.Cryptor, journal)
}

func topLevelDeletions(paths []string) []string {
	sorted := append([]string(nil), paths...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) < len(sorted[j]) })
	var result []string
	for _, path := range sorted {
		covered := false
		for _, parent := range result {
			if path == parent || strings.HasPrefix(path, parent+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, path)
		}
	}
	return result
}
