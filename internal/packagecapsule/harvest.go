package packagecapsule

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type candidate struct {
	path, buildInfo string
	meta            artifactMetadata
}

func harvest(options Options, root string, manifest *Manifest) {
	roots := cacheRoots(root)
	destination := filepath.Join(root, "artifacts")
	totals, completed := map[string]int{}, map[string]int{}
	for _, pkg := range manifest.Packages {
		totals[pkg.Source]++
	}
	for index := range manifest.Packages {
		if options.Context.Err() != nil {
			return
		}
		pkg := &manifest.Packages[index]
		emit(options, Progress{Phase: "harvest", Package: pkg.Name, Source: pkg.Source, Completed: completed[pkg.Source], Total: totals[pkg.Source], Bytes: manifest.Summary.Bytes})
		completed[pkg.Source]++
		selected, err := selectCandidate(options.Run, roots, *pkg)
		if err != nil {
			manifest.Summary.Missing++
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "artifact-missing", Detail: acquisitionHint(*pkg)})
			continue
		}
		ok, reason := compatible(selected.meta, selected.buildInfo)
		if !ok {
			manifest.Summary.Rejected++
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "artifact-incompatible", Detail: reason})
			continue
		}
		if signature := selected.path + ".sig"; regular(signature) {
			if err := options.Run.Run("pacman-key", "--verify", signature, selected.path); err != nil {
				manifest.Summary.Rejected++
				manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "signature-invalid", Detail: compactError(err)})
				continue
			}
			pkg.SignatureValid = true
		}
		target := filepath.Join(destination, filepath.Base(selected.path))
		hash, bytes, err := copyHashed(selected.path, target)
		if err != nil {
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "artifact-copy-failed", Detail: compactError(err)})
			continue
		}
		pkg.Artifact, pkg.SHA256 = filepath.Join("artifacts", filepath.Base(target)), hash
		pkg.ArtifactVersion, pkg.Architecture = selected.meta.Version, selected.meta.Architecture
		pkg.Depends, pkg.Provides, pkg.Conflicts = selected.meta.Depends, selected.meta.Provides, selected.meta.Conflicts
		pkg.BuildDate, pkg.Packager, pkg.BuildInfo, pkg.Compatible = selected.meta.BuildDate, selected.meta.Packager, selected.buildInfo, true
		pkg.PackageInfo = selected.meta.Raw
		manifest.Summary.Captured++
		manifest.Summary.Bytes += bytes
		captureSignature(selected.path, destination, pkg)
	}
	for source, total := range totals {
		emit(options, Progress{Phase: "complete", Source: source, Completed: total, Total: total, Bytes: manifest.Summary.Bytes})
	}
	if len(manifest.Flatpaks) > 0 {
		manifest.ManualReview = append(manifest.ManualReview, "Flatpak applications are manifest-only and require their configured remotes during recovery")
	}
	if len(manifest.Exceptions) > 0 {
		manifest.ManualReview = append(manifest.ManualReview, "Packages without compatible artifacts will use the controlled online fallback lane")
	}
}

func selectCandidate(run Runner, roots []string, pkg Package) (candidate, error) {
	if exact := findArtifact(roots, pkg.Name, pkg.Installed); exact != "" {
		meta, build, err := inspectArtifact(run, exact)
		if err == nil && meta.Name == pkg.Name {
			return candidate{path: exact, meta: meta, buildInfo: build}, nil
		}
	}
	var best candidate
	for _, root := range roots {
		for _, pattern := range []string{filepath.Join(root, pkg.Name+"-*.pkg.tar.*"), filepath.Join(root, "*", pkg.Name+"-*.pkg.tar.*"), filepath.Join(root, "*", "*", pkg.Name+"-*.pkg.tar.*")} {
			matches, _ := filepath.Glob(pattern)
			for _, path := range matches {
				if strings.HasSuffix(path, ".sig") || !regular(path) {
					continue
				}
				meta, build, err := inspectArtifact(run, path)
				if err != nil || meta.Name != pkg.Name {
					continue
				}
				if best.path == "" || versionGreater(run, meta.Version, best.meta.Version) {
					best = candidate{path: path, meta: meta, buildInfo: build}
				}
			}
		}
	}
	if best.path == "" {
		return candidate{}, fmt.Errorf("no valid artifact")
	}
	return best, nil
}

func versionGreater(run Runner, left, right string) bool {
	value, err := run.Output("vercmp", left, right)
	if err != nil {
		return left > right
	}
	comparison, err := strconv.Atoi(strings.TrimSpace(value))
	return err == nil && comparison > 0
}

func captureSignature(source, destination string, pkg *Package) {
	signature := source + ".sig"
	if !regular(signature) {
		return
	}
	target := filepath.Join(destination, filepath.Base(signature))
	hash, _, err := copyHashed(signature, target)
	if err != nil {
		return
	}
	pkg.Signature, pkg.SignatureSHA256 = filepath.Join("artifacts", filepath.Base(target)), hash
}
