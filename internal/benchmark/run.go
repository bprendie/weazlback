package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Run(ctx context.Context, options Options, progress func(string)) (Report, error) {
	selected, err := selectEngine(options)
	if err != nil {
		return Report{}, err
	}
	version, err := selected.version(ctx)
	if err != nil {
		return Report{}, err
	}
	fixtures, err := selectedFixtures(options.Fixture)
	if err != nil {
		return Report{}, err
	}
	if options.WorkDir == "" {
		options.WorkDir = ".weazlback-bench"
	}
	if options.Trials < 1 {
		options.Trials = 1
	}
	if err := os.MkdirAll(options.WorkDir, 0o700); err != nil {
		return Report{}, err
	}
	runRoot, err := os.MkdirTemp(options.WorkDir, "run-")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(runRoot)
	report := Report{
		SchemaVersion: 2,
		Engine:        selected.name(),
		EngineVersion: version,
		StartedAt:     time.Now().UTC(),
		WorkDir:       options.WorkDir,
	}
	for _, fixture := range fixtures {
		progress("creating " + fixture + " fixture")
		result, err := runFixture(ctx, selected, options, runRoot, fixture, progress)
		if err != nil {
			return report, err
		}
		report.Results = append(report.Results, result)
	}
	if options.Output != "" {
		if err := Write(options.Output, report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func runFixture(ctx context.Context, selected engine, options Options, runRoot, fixture string, progress func(string)) (Result, error) {
	root := filepath.Join(runRoot, fixture)
	source := filepath.Join(root, "source")
	repo := filepath.Join(root, "repository")
	remote := options.Repository != ""
	if remote {
		repo = strings.TrimRight(options.Repository, "/") + "/" + fixture
	}
	restore := filepath.Join(root, "restore")
	if err := createFixture(fixture, source); err != nil {
		return Result{}, err
	}
	logical, allocated := treeBytes(source)
	result := Result{Fixture: fixture, LogicalBytes: logical, AllocatedBytes: allocated}
	if proofs, proofErr := proveTree(source); proofErr == nil {
		result.FileCount = int64(len(proofs))
	}
	if err := selected.init(ctx, repo); err != nil {
		return result, err
	}
	progress("initial " + fixture + " backup")
	initialDuration, err := duration(func() error { return selected.backup(ctx, repo, source, "initial") })
	if err != nil {
		return result, fmt.Errorf("initial %s backup: %w", fixture, err)
	}
	result.InitialDuration = initialDuration
	result.InitialRepoBytes = -1
	if !remote {
		result.InitialRepoBytes, _ = treeBytes(repo)
	}
	progress("no-change " + fixture + " backup")
	before := result.InitialRepoBytes
	noChangeDuration, err := duration(func() error { return selected.backup(ctx, repo, source, "no-change") })
	if err != nil {
		return result, err
	}
	result.NoChangeDuration = noChangeDuration
	after := int64(-1)
	if !remote {
		after, _ = treeBytes(repo)
		result.NoChangeRepoGrowth = after - before
	} else {
		result.NoChangeRepoGrowth = -1
	}
	if err := mutateFixture(fixture, source); err != nil {
		return result, err
	}
	progress("changed " + fixture + " backup")
	before = after
	result.ChangedDuration, err = duration(func() error { return selected.backup(ctx, repo, source, "changed") })
	if err != nil {
		return result, err
	}
	if !remote {
		after, _ = treeBytes(repo)
		result.ChangedRepoGrowth = after - before
	} else {
		result.ChangedRepoGrowth = -1
	}
	for trial := 0; trial < options.Trials; trial++ {
		_ = os.RemoveAll(restore)
		progress(fmt.Sprintf("restoring %s (trial %d/%d)", fixture, trial+1, options.Trials))
		d, restoreErr := duration(func() error { return selected.restore(ctx, repo, restore) })
		if restoreErr != nil {
			return result, restoreErr
		}
		result.RestoreTrials = append(result.RestoreTrials, d)
	}
	result.ColdRestore = result.RestoreTrials[0]
	result.RestoreDuration = medianDuration(result.RestoreTrials)
	if len(result.RestoreTrials) > 1 {
		result.WarmRestoreMedian = medianDuration(result.RestoreTrials[1:])
	}
	result.RestoredBytes, result.RestoredAllocated = treeBytes(restore)
	restoredSource := filepath.Join(restore, source)
	if err := verifyTrees(source, restoredSource); err != nil {
		return result, fmt.Errorf("verify %s restore: %w", fixture, err)
	}
	result.RestoreVerified = true
	return result, nil
}

func medianDuration(values []time.Duration) time.Duration {
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	middle := len(copyValues) / 2
	if len(copyValues)%2 == 1 {
		return copyValues[middle]
	}
	return (copyValues[middle-1] + copyValues[middle]) / 2
}

func duration(fn func() error) (time.Duration, error) {
	started := time.Now()
	err := fn()
	if err != nil {
		return 0, err
	}
	return time.Since(started), nil
}

func selectedFixtures(name string) ([]string, error) {
	if name == "" || name == "all" {
		return fixtureNames, nil
	}
	for _, fixture := range fixtureNames {
		if fixture == name {
			return []string{name}, nil
		}
	}
	return nil, fmt.Errorf("fixture must be all, tiny, mixed, raw, qcow2, or metadata")
}

func Write(path string, report Report) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
