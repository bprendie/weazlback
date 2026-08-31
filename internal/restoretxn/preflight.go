package restoretxn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func PreflightPlan(ctx context.Context, service Service, plan *Plan) (Preflight, error) {
	var report Preflight
	if plan.ID == "" || plan.Snapshot == "" || len(plan.Items) == 0 || plan.StageRoot == "" || plan.JournalPath == "" {
		return report, fmt.Errorf("restore transaction plan is incomplete")
	}
	if plan.Conflict == "" {
		plan.Conflict = ReplacePreserving
	}
	if err := service.Check(ctx, plan.Repository, false); err != nil {
		return report, fmt.Errorf("repository health: %w", err)
	}
	paths := make([]string, len(plan.Items))
	for i := range plan.Items {
		paths[i] = plan.Items[i].SourcePath
		if crossMachineIdentityPath(plan.Items[i].TargetPath, plan.SourceMachineID, plan.TargetMachineID) {
			return report, fmt.Errorf("cross-machine restore cannot replace identity-bearing path %s", plan.Items[i].TargetPath)
		}
	}
	entries, err := service.FilesAt(ctx, plan.Repository, plan.Snapshot, paths)
	if err != nil {
		return report, fmt.Errorf("read authoritative path metadata: %w", err)
	}
	byPath := map[string]struct{ index int }{}
	for i := range plan.Items {
		byPath[filepath.Clean(plan.Items[i].SourcePath)] = struct{ index int }{i}
	}
	for _, entry := range entries {
		clean := filepath.Clean(entry.Path)
		for path, selected := range byPath {
			if clean == path {
				plan.Items[selected.index].Entry = entry
			}
			if clean == path || strings.HasPrefix(clean, path+string(filepath.Separator)) {
				report.Files++
				report.BytesRequired += entry.Size
				if entry.Type == "symlink" {
					report.Symlinks++
				}
			}
		}
	}
	for _, item := range plan.Items {
		if item.Entry.Path == "" {
			return report, fmt.Errorf("selected repository path is absent: %s", item.SourcePath)
		}
		if boundary, boundaryErr := mountBoundary(item.TargetPath); boundaryErr == nil {
			report.MountBoundaries = append(report.MountBoundaries, boundary)
		}
	}
	if len(plan.Items) > 0 && plan.SourceUID == 0 && plan.SourceGID == 0 {
		plan.SourceUID, plan.SourceGID = plan.Items[0].Entry.UID, plan.Items[0].Entry.GID
	}
	report.CrossFilesystem = differentDevices(plan.StageRoot, plan.Items[0].TargetPath)
	report.OwnershipMappingNeeded = plan.SourceUID != plan.TargetUID || plan.SourceGID != plan.TargetGID
	report.BytesAvailable, err = availableBytesForPreflight(plan.Items[0].TargetPath)
	if err != nil {
		return report, err
	}
	if report.BytesAvailable < report.BytesRequired {
		return report, fmt.Errorf("insufficient destination space: need %d bytes, have %d", report.BytesRequired, report.BytesAvailable)
	}
	return report, nil
}

var availableBytesForPreflight = availableBytes

func crossMachineIdentityPath(path, sourceID, targetID string) bool {
	return sourceID != "" && targetID != "" && sourceID != targetID && strings.HasSuffix(filepath.Clean(path), filepath.Join(".config", "weazlback", "config.json"))
}

func existingAncestor(path string) (string, os.FileInfo, error) {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			return current, info, nil
		}
		if !os.IsNotExist(err) || current == filepath.Dir(current) {
			return "", nil, err
		}
	}
}

func availableBytes(path string) (uint64, error) {
	ancestor, _, err := existingAncestor(path)
	if err != nil {
		return 0, err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(ancestor, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func device(path string) uint64 {
	_, info, err := existingAncestor(path)
	if err != nil {
		return 0
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Dev)
	}
	return 0
}

func differentDevices(left, right string) bool {
	return device(left) != 0 && device(right) != 0 && device(left) != device(right)
}

func mountBoundary(path string) (string, error) {
	ancestor, _, err := existingAncestor(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s dev:%d", ancestor, device(ancestor)), nil
}
