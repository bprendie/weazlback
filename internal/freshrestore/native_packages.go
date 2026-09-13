package freshrestore

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/targetpackages"
)

type NativePackagePlan = targetpackages.Plan

func (r *Restore) configureNativePackages(ctx context.Context, snapshots []restic.Snapshot) error {
	if !r.Plan.Migration.Active {
		return nil
	}
	run := packagecapsule.ExecRunner{Context: ctx, Quiet: true}
	env, err := targetpackages.CurrentEnvironment(run)
	if err != nil {
		return fmt.Errorf("inspect target package environment: %w", err)
	}
	r.Plan.Migration.TargetFingerprint = env.Fingerprint
	previous, err := LoadJournal(filepath.Join(r.Options.WorkDir, r.Session.Destination.ID+"-"+r.Options.Scope+"-journal.json"))
	if err != nil {
		return err
	}
	if previous.Migration.Active && (previous.Migration.TargetArchitecture != r.Plan.Migration.TargetArchitecture || previous.Migration.TargetFingerprint != env.Fingerprint || previous.TargetHome != r.Plan.TargetHome) {
		return fmt.Errorf("target architecture/platform changed since the saved plan; use a new restore workspace")
	}
	native := targetpackages.Resolve(*r.Plan.Applications, env.Architecture, env.Fingerprint, run)
	if previous.NativePlan != nil {
		if previous.NativePlan.InventoryDigest != native.InventoryDigest {
			return fmt.Errorf("source application inventory changed since saved plan")
		}
		for name, version := range previous.NativePlan.Versions {
			output, err := run.Output("pacman", "-Si", "--", name)
			if err != nil || targetpackages.Fields(output)["Version"] != version {
				return fmt.Errorf("target package %s changed since saved plan; use a new workspace to review current versions", name)
			}
		}
		native = *previous.NativePlan
	}
	r.Plan.NativePlan = &native
	r.Plan.Official, r.Plan.AUR, r.Plan.Flatpak = native.Official, nil, native.Flatpak
	r.Plan.PackageReview = append(r.Plan.PackageReview, native.Review...)
	for _, unit := range append(append([]string{}, r.Plan.SystemServices...), r.Plan.UserServices...) {
		r.Plan.PackageReview = append(r.Plan.PackageReview, unit+": source service not enabled; review against target hardware and native applications")
	}
	r.Plan.SystemServices, r.Plan.UserServices, r.Plan.LocalApps = nil, nil, nil
	r.Plan.ArtifactFiles = nil
	r.Plan.PackageSnapshot = nil
	r.Plan.PackageCapsule = nil
	r.Plan.PackageDelta = packagecapsule.Delta{}
	// Exact inventory matches win; older sets may supply individual matching native
	// artifacts, with every missing application remaining in the download lane.
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
	var selected *restic.Snapshot
	var capsule packagecapsule.Manifest
	manifestPath := ""
	for _, point := range snapshots {
		if restic.Profile(point.Tags) != "packages-"+architecture.Go(env.Architecture) || restic.SnapshotHealth(point.Tags) != "healthy" {
			continue
		}
		if previous.NativePlan != nil && (previous.PackageSnapshotID == "" || previous.PackageSnapshotID != point.ID) {
			continue
		}
		r.Plan.PackageSnapshot = &point
		candidate, path, err := r.loadPackageCapsule(ctx)
		if err != nil {
			if previous.PackageSnapshotID == point.ID {
				return err
			}
			continue
		}
		if !architecture.Compatible(candidate.Architecture, env.Architecture) || candidate.TargetFingerprint != env.Fingerprint {
			continue
		}
		if selected == nil || candidate.SourceInventoryDigest == native.InventoryDigest {
			copy := point
			selected = &copy
			capsule = candidate
			manifestPath = path
			if candidate.SourceInventoryDigest == native.InventoryDigest {
				break
			}
		}
	}
	r.Plan.PackageSnapshot = selected
	if previous.NativePlan != nil && previous.PackageSnapshotID != "" && selected == nil {
		return fmt.Errorf("pinned ARM capsule unavailable; retain it or start a new restore workspace to re-plan downloads")
	}
	if selected == nil {
		return nil
	}
	r.Plan.PackageCapsule = &capsule
	r.Plan.PackageManifestPath = manifestPath
	r.Plan.PackagePreparedAt = capsule.CapturedAt
	if capsule.SourceInventoryDigest != native.InventoryDigest {
		r.Plan.PackageReview = append(r.Plan.PackageReview, "Prepared package inventory differs from selected snapshot; uncovered applications download natively")
	}
	r.applyNativeCapsule(native, capsule, env.Architecture, run)
	return nil
}

func (r *Restore) applyNativeCapsule(native NativePackagePlan, capsule packagecapsule.Manifest, target string, run packagecapsule.Runner) {
	needed := map[string]bool{}
	foreign := map[string]bool{}
	for _, name := range native.Build {
		foreign[name] = true
		needed[name] = true
	}
	for _, n := range native.Official {
		needed[n] = true
	}
	for _, pkg := range capsule.Packages {
		if !needed[pkg.Name] || !pkg.Compatible || !architecture.Compatible(pkg.Architecture, target) || pkg.Artifact == "" {
			continue
		}
		// Only reuse exact versions available in the target repositories; this keeps
		// stale capsules from forcing an obsolete dependency graph onto a new image.
		if !foreign[pkg.Name] {
			out, err := run.Output("pacman", "-Si", "--", pkg.Name)
			if err != nil || targetpackages.Fields(out)["Version"] != pkg.ArtifactVersion {
				continue
			}
		} else if pkg.Source != "foreign" {
			continue
		}
		r.Plan.PackageDelta.Local = append(r.Plan.PackageDelta.Local, packagecapsule.Install{Name: pkg.Name, Version: pkg.ArtifactVersion, Source: pkg.Source, Artifact: pkg.Artifact})
		delete(needed, pkg.Name)
		if foreign[pkg.Name] {
			var review []string
			for _, item := range r.Plan.PackageReview {
				if !strings.HasPrefix(item, pkg.Name+":") {
					review = append(review, item)
				}
			}
			r.Plan.PackageReview = review
		}
	}
	r.Plan.Official = nil
	for _, n := range native.Official {
		if needed[n] {
			r.Plan.Official = append(r.Plan.Official, n)
		}
	}
}
