package targetpackages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
)

func Prepare(ctx context.Context, root, staging, machine string, source inventory.ApplicationManifest, download, build bool, progress func(packagecapsule.Progress)) (packagecapsule.Manifest, string, func(), error) {
	empty := func(err error) (packagecapsule.Manifest, string, func(), error) {
		return packagecapsule.Manifest{}, "", func() {}, err
	}
	env, err := LoadEnvironment(root)
	if err != nil {
		return empty(err)
	}
	base := packagecapsule.ExecRunner{Context: ctx, Quiet: progress != nil && !build}
	run := IsolatedRunner{Base: base, Root: root, Environment: env}
	if download {
		if progress != nil {
			progress(packagecapsule.Progress{Phase: "syncing", Package: "target repositories", Source: "official"})
		}
		if err := run.Initialize(); err != nil {
			return empty(err)
		}
	}
	plan := resolveWithProgress(source, env.Architecture, env.Fingerprint, run, progress, heldPackages(env.Config))
	manifest := packagecapsule.Manifest{MachineID: machine, Architecture: env.Architecture, Platform: env.Platform, PackageFamily: "pacman", TargetFingerprint: env.Fingerprint, SourceInventoryDigest: plan.InventoryDigest, Flatpaks: source.PackagePlan.Flatpak}
	manifest.ManualReview = append(manifest.ManualReview, plan.Review...)
	for _, name := range plan.Official {
		manifest.Packages = append(manifest.Packages, packagecapsule.Package{Name: name, Installed: plan.Versions[name], Source: "official", Reason: "explicit"})
	}
	if download && len(plan.Official) > 0 {
		if err := downloadOfficial(ctx, run, plan.Official, progress); err != nil {
			if ctx.Err() != nil {
				return empty(ctx.Err())
			}
			manifest.Exceptions = append(manifest.Exceptions, packagecapsule.Exception{Code: "download-failed", Detail: err.Error()})
		}
	}
	roots := []string{filepath.Join(root, "cache")}
	if build {
		if !architecture.Compatible(env.Architecture, architecture.Native()) {
			return empty(fmt.Errorf("source builds require the native %s target; transfer the environment and source inventory there", env.Architecture))
		}
		current, err := CurrentEnvironment(base)
		if err != nil {
			return empty(err)
		}
		if current.Fingerprint != env.Fingerprint {
			return empty(fmt.Errorf("build host repositories/platform differ from exported target"))
		}
		for _, name := range plan.Build {
			if Hardware(name) || !SafeName(name) {
				continue
			}
			if err := os.MkdirAll(filepath.Join(root, "build"), 0700); err != nil {
				return empty(err)
			}
			dir := filepath.Join(root, "build", name)
			if _, exists := os.Stat(dir); os.IsNotExist(exists) {
				if err := base.Run("git", "clone", "--depth", "1", "https://aur.archlinux.org/"+name+".git", dir); err != nil {
					manifest.ManualReview = append(manifest.ManualReview, name+": "+err.Error())
					continue
				}
			}
			if err := base.RunDir(dir, "makepkg", "--syncdeps", "--cleanbuild", "--clean"); err != nil {
				manifest.ManualReview = append(manifest.ManualReview, name+": "+err.Error())
				continue
			}
			output, err := base.Output("bash", "-c", `cd "$1" && makepkg --packagelist`, "weazlback", dir)
			if err != nil {
				return empty(err)
			}
			for _, path := range strings.Split(strings.TrimSpace(output), "\n") {
				if path != "" {
					roots = append(roots, filepath.Dir(path))
					pkg, err := packagecapsule.ReadArtifactIdentity(run, path, env.Architecture)
					if err != nil {
						return empty(err)
					}
					manifest.Packages = append(manifest.Packages, pkg)
				}
			}
		}
	}
	if len(manifest.Packages) == 0 {
		return empty(fmt.Errorf("no target repository applications resolved; inspect availability first"))
	}
	return packagecapsule.PrepareArtifacts(packagecapsule.Options{Context: ctx, StagingRoot: staging, ArtifactRoots: roots, Run: run, Progress: progress}, manifest)
}
