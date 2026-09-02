package freshrestore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
)

func selectLatestPackageSnapshot(snapshots []restic.Snapshot) *restic.Snapshot {
	var matches []restic.Snapshot
	for _, snapshot := range snapshots {
		if restic.Profile(snapshot.Tags) == "packages" && restic.SnapshotHealth(snapshot.Tags) == "healthy" {
			matches = append(matches, snapshot)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Time.After(matches[j].Time) })
	selected := matches[0]
	return &selected
}

func (r *Restore) loadPackageCapsule(ctx context.Context) (packagecapsule.Manifest, string, error) {
	if r.Plan.PackageSnapshot == nil {
		return packagecapsule.Manifest{}, "", errors.New("Package Capsule Restore Point is not selected")
	}
	files, err := r.service.Files(ctx, r.Session.Repository, r.Plan.PackageSnapshot.ID)
	if err != nil {
		return packagecapsule.Manifest{}, "", err
	}
	manifestPath := ""
	for _, file := range files {
		if filepath.Base(file.Path) == packagecapsule.ManifestName {
			manifestPath = file.Path
			break
		}
	}
	if manifestPath == "" {
		return packagecapsule.Manifest{}, "", errors.New("Package Capsule Restore Point has no manifest")
	}
	data, err := r.service.Dump(ctx, r.Session.Repository, r.Plan.PackageSnapshot.ID, manifestPath)
	if err != nil {
		return packagecapsule.Manifest{}, "", err
	}
	manifest, err := packagecapsule.Parse(data)
	if err != nil {
		return packagecapsule.Manifest{}, "", err
	}
	if manifest.MachineID != "" && manifest.MachineID != r.Plan.SourceMachineID {
		return packagecapsule.Manifest{}, "", errors.New("Package Capsule belongs to another machine identity")
	}
	return manifest, manifestPath, nil
}

func resolvePackageDelta(ctx context.Context, manifest packagecapsule.Manifest) (packagecapsule.Delta, error) {
	installed, err := installedPackageVersions(ctx)
	if err != nil {
		return packagecapsule.Delta{}, err
	}
	var compareErr error
	compare := func(left, right string) int {
		if left == right {
			return 0
		}
		result, err := archVersionCompare(ctx, left, right)
		if err != nil {
			compareErr = fmt.Errorf("compare package versions %q and %q: %w", left, right, err)
			return 0
		}
		return result
	}
	delta := packagecapsule.ResolveDelta(manifest, installed, compare)
	return delta, compareErr
}

func archVersionCompare(ctx context.Context, left, right string) (int, error) {
	if left == right {
		return 0, nil
	}
	value, err := exec.CommandContext(ctx, "vercmp", left, right).Output()
	if err != nil {
		return 0, err
	}
	result, err := strconv.Atoi(strings.TrimSpace(string(value)))
	if err != nil {
		return 0, fmt.Errorf("parse vercmp result: %w", err)
	}
	return result, nil
}

func installedPackageVersions(ctx context.Context) (map[string]string, error) {
	output, err := exec.CommandContext(ctx, "pacman", "-Q").Output()
	if err != nil {
		return nil, fmt.Errorf("inventory fresh Omarchy packages: %w", err)
	}
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			result[fields[0]] = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("fresh Omarchy package inventory is empty")
	}
	return result, nil
}
