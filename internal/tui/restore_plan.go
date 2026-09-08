package tui

import (
	"fmt"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m Model) transactionPlans(repo restic.Repository, alternate string) ([]restoretxn.Plan, error) {
	if len(m.restoreBasket) == 0 {
		return nil, fmt.Errorf("restore basket is empty")
	}
	root, err := privateRestoreTarget()
	if err != nil {
		return nil, err
	}
	groups := map[string][]restoretxn.Item{}
	metadata := map[string]restoreBasketItem{}
	for path, basket := range normalizedBasket(m.restoreBasket) {
		if active := m.cfg.Active(); active != nil && basket.Destination != "" && basket.Destination != active.ID {
			return nil, fmt.Errorf("basket belongs to a different destination; return to its repository")
		}
		target, targetErr := m.restoreTarget(path, alternate)
		if targetErr != nil {
			return nil, targetErr
		}
		groups[basket.Snapshot] = append(groups[basket.Snapshot], restoretxn.Item{SourcePath: path, TargetPath: target})
		metadata[basket.Snapshot] = basket
	}
	if err := validateRestoreTargets(groups); err != nil {
		return nil, err
	}
	operationID := randomOperationID()
	journalRoot := filepath.Join(filepath.Dir(root), "journals")
	var plans []restoretxn.Plan
	for snapshot, items := range groups {
		sort.Slice(items, func(i, j int) bool { return items[i].SourcePath < items[j].SourcePath })
		meta := metadata[snapshot]
		plans = append(plans, restoretxn.Plan{ID: operationID + "-" + shortID(snapshot), Snapshot: snapshot,
			SourceMachineID: meta.MachineID, TargetMachineID: m.cfg.Machine.ID, Repository: repo, Items: items,
			StageRoot: filepath.Join(root, shortID(snapshot)), JournalPath: filepath.Join(journalRoot, operationID+"-"+shortID(snapshot)+".enc"),
			Conflict: m.restoreConflict, StagingOnly: m.restoreTargetMode == "staging", TargetUID: uint32(os.Getuid()), TargetGID: uint32(os.Getgid())})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Snapshot < plans[j].Snapshot })
	return plans, nil
}

func normalizedBasket(input map[string]restoreBasketItem) map[string]restoreBasketItem {
	cleaned := make(map[string]restoreBasketItem, len(input))
	paths := make([]string, 0, len(input))
	for path, item := range input {
		path = filepath.Clean(path)
		item.Path = path
		cleaned[path] = item
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) < len(paths[j]) })
	result := map[string]restoreBasketItem{}
	for _, path := range paths {
		item := cleaned[path]
		covered := false
		for parent, selected := range result {
			if selected.Snapshot == item.Snapshot && strings.HasPrefix(path, parent+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			result[path] = item
		}
	}
	return result
}

func (m Model) restoreTarget(source, alternate string) (string, error) {
	if m.restoreTargetMode == "staging" {
		return source, nil
	}
	if m.restoreTargetMode == "alternate" {
		if strings.TrimSpace(alternate) == "" {
			return "", fmt.Errorf("alternate destination is required")
		}
		return filepath.Join(alternate, filepath.Base(source)), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(source), "/"), "/")
	if len(parts) >= 3 && parts[0] == "home" {
		return filepath.Join(home, filepath.Join(parts[2:]...)), nil
	}
	return source, nil
}
