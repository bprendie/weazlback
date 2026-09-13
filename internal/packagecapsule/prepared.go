package packagecapsule

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PrepareArtifacts captures an independently resolved target cache including its
// downloaded dependency closure. It never inventories or downloads host packages.
func PrepareArtifacts(options Options, manifest Manifest) (Manifest, string, func(), error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Run == nil {
		options.Run = ExecRunner{Context: options.Context}
	}
	root, cleanup, err := newStaging(options.StagingRoot)
	if err != nil {
		return manifest, "", cleanup, err
	}
	fail := func(err error) (Manifest, string, func(), error) { cleanup(); return manifest, "", func() {}, err }
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0700); err != nil {
		return fail(err)
	}
	wanted := map[string]Package{}
	for _, pkg := range manifest.Packages {
		wanted[pkg.Name] = pkg
	}
	var archives []string
	for _, cache := range options.ArtifactRoots {
		paths, _ := filepath.Glob(filepath.Join(cache, "*.pkg.tar.*"))
		for _, path := range paths {
			if !strings.HasSuffix(path, ".sig") && !strings.HasSuffix(path, ".part") {
				archives = append(archives, path)
			}
		}
	}
	for index, path := range archives {
		if err := options.Context.Err(); err != nil {
			return fail(err)
		}
		emit(options, Progress{Phase: "inspect cache", Source: "official", Package: filepath.Base(path), Completed: index, Total: len(archives)})
		meta, build, err := inspectArtifact(options.Run, path)
		if err != nil {
			continue
		}
		if ok, _ := compatibleFor(meta, build, manifest.Architecture); !ok {
			continue
		}
		if _, ok := wanted[meta.Name]; !ok {
			wanted[meta.Name] = Package{Name: meta.Name, Installed: meta.Version, Source: "official", Reason: "dependency"}
		}
	}
	emit(options, Progress{Phase: "inspect cache", Source: "official", Completed: len(archives), Total: len(archives), Package: "package metadata inspected"})
	manifest.Packages = nil
	for _, pkg := range wanted {
		manifest.Packages = append(manifest.Packages, pkg)
	}
	sort.Slice(manifest.Packages, func(i, j int) bool { return manifest.Packages[i].Name < manifest.Packages[j].Name })
	manifest.SchemaVersion = SchemaVersion
	manifest.CapturedAt = time.Now().UTC()
	manifest.Summary = Summary{Installed: len(manifest.Packages)}
	if manifest.Hostname == "" {
		manifest.Hostname = "prepared-target"
	}
	for _, pkg := range manifest.Packages {
		if pkg.Source == "official" {
			manifest.Summary.Official++
		} else {
			manifest.Summary.Foreign++
		}
	}
	options.TargetArchitecture = manifest.Architecture
	harvest(options, root, &manifest)
	if err := options.Context.Err(); err != nil {
		return fail(err)
	}
	if err := writeManifest(filepath.Join(root, ManifestName), manifest); err != nil {
		return fail(err)
	}
	if err := Validate(root, manifest); err != nil {
		return fail(fmt.Errorf("prepared capsule: %w", err))
	}
	return manifest, root, cleanup, nil
}

// ReadArtifactIdentity validates metadata before admitting a native build output.
func ReadArtifactIdentity(run Runner, path, target string) (Package, error) {
	meta, build, err := inspectArtifact(run, path)
	if err != nil {
		return Package{}, err
	}
	if ok, reason := compatibleFor(meta, build, target); !ok {
		return Package{}, fmt.Errorf("%s", reason)
	}
	return Package{Name: meta.Name, Installed: meta.Version, ArtifactVersion: meta.Version, Architecture: meta.Architecture, Source: "foreign", Reason: "explicit"}, nil
}
