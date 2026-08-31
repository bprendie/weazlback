package backupmeta

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
)

const ManifestName = "weazlback-applications-v1.json"

func PrepareApplications(ctx context.Context, profile string) (string, func(), error) {
	if profile != "core" {
		return "", func() {}, nil
	}
	root, err := privateRoot()
	if err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", func() {}, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", func() {}, err
	}
	staging, err := os.MkdirTemp(root, "applications-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	manifest, err := inventory.CaptureApplications(ctx)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	manifest.AURArtifacts = captureArtifacts(manifest, staging)
	path := filepath.Join(staging, ManifestName)
	if err := inventory.WriteApplications(path, manifest); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return staging, cleanup, nil
}

func captureArtifacts(manifest inventory.ApplicationManifest, staging string) []inventory.PackageArtifact {
	versions := map[string]string{}
	for _, pkg := range manifest.Packages.ForeignInstalled {
		versions[pkg.Name] = pkg.Version
	}
	destination := filepath.Join(staging, "aur-artifacts")
	var artifacts []inventory.PackageArtifact
	for _, name := range manifest.PackagePlan.AUR {
		source := findArtifact(name, versions[name])
		if source == "" {
			continue
		}
		_ = os.MkdirAll(destination, 0o700)
		target := filepath.Join(destination, filepath.Base(source))
		digest, err := copyDigest(source, target)
		if err == nil {
			artifacts = append(artifacts, inventory.PackageArtifact{Package: name, Version: versions[name], File: filepath.Base(target), SHA256: digest})
		}
	}
	return artifacts
}

func findArtifact(name, version string) string {
	home, _ := os.UserHomeDir()
	roots := []string{"/var/cache/pacman/pkg", filepath.Join(home, ".cache", "paru", "clone"), filepath.Join(home, ".cache", "yay")}
	prefix := name + "-" + version + "-"
	for _, root := range roots {
		matches, _ := filepath.Glob(filepath.Join(root, prefix+"*.pkg.tar.*"))
		nested, _ := filepath.Glob(filepath.Join(root, "*", prefix+"*.pkg.tar.*"))
		matches = append(matches, nested...)
		for _, match := range matches {
			if !strings.HasSuffix(match, ".sig") {
				return match
			}
		}
	}
	return ""
}

func copyDigest(source, target string) (string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func privateRoot() (string, error) {
	path, err := config.Path()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(path), "staging")
	if dir == "" || dir == "." {
		return "", errors.New("invalid private staging path")
	}
	return dir, nil
}
