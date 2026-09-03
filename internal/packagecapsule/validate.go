package packagecapsule

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Load(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, ManifestName))
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := Parse(data)
	if err != nil {
		return Manifest{}, err
	}
	if err := Validate(root, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Parse(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.PackageFamily == "" {
		manifest.PackageFamily = "pacman"
	}
	if err := ValidateLedger(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateLedger(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion || manifest.CapturedAt.IsZero() || manifest.Hostname == "" {
		return errors.New("package capsule identity is incomplete or unsupported")
	}
	if len(manifest.Packages) == 0 || manifest.Summary.Installed != len(manifest.Packages) {
		return errors.New("package capsule inventory is incomplete")
	}
	for _, pkg := range manifest.Packages {
		if pkg.Name == "" || pkg.Installed == "" || (pkg.Source != "official" && pkg.Source != "foreign") {
			return fmt.Errorf("invalid package ledger entry %q", pkg.Name)
		}
	}
	return nil
}

func Validate(root string, manifest Manifest) error {
	if err := ValidateLedger(manifest); err != nil {
		return err
	}
	captured := 0
	for _, pkg := range manifest.Packages {
		if pkg.Artifact == "" {
			continue
		}
		if !pkg.Compatible || pkg.SHA256 == "" {
			return fmt.Errorf("unvalidated artifact for %s", pkg.Name)
		}
		path, err := capsulePath(root, pkg.Artifact)
		if err != nil {
			return fmt.Errorf("%s: %w", pkg.Name, err)
		}
		actual, err := digest(path)
		if err != nil || actual != pkg.SHA256 {
			return fmt.Errorf("artifact checksum mismatch for %s", pkg.Name)
		}
		if pkg.Signature != "" {
			if !pkg.SignatureValid {
				return fmt.Errorf("signature was not verified for %s", pkg.Name)
			}
			signature, err := capsulePath(root, pkg.Signature)
			if err != nil {
				return fmt.Errorf("%s signature: %w", pkg.Name, err)
			}
			actual, err := digest(signature)
			if err != nil || actual != pkg.SignatureSHA256 {
				return fmt.Errorf("signature checksum mismatch for %s", pkg.Name)
			}
		}
		captured++
	}
	if captured != manifest.Summary.Captured {
		return errors.New("package capsule summary does not match artifact ledger")
	}
	return nil
}

func VerifyArtifacts(root string, manifest Manifest, run Runner) (map[string]string, error) {
	if run == nil {
		run = ExecRunner{Quiet: true}
	}
	if err := Validate(root, manifest); err != nil {
		return nil, err
	}
	result := make(map[string]string, manifest.Summary.Captured)
	for _, pkg := range manifest.Packages {
		if pkg.Artifact == "" {
			continue
		}
		path, _ := capsulePath(root, pkg.Artifact)
		meta, buildInfo, err := inspectArtifact(run, path)
		if err != nil {
			return nil, fmt.Errorf("inspect restored artifact for %s: %w", pkg.Name, err)
		}
		if meta.Name != pkg.Name || meta.Version != pkg.ArtifactVersion || meta.Architecture != pkg.Architecture {
			return nil, fmt.Errorf("restored artifact identity mismatch for %s", pkg.Name)
		}
		if ok, reason := compatible(meta, buildInfo); !ok {
			return nil, fmt.Errorf("restored artifact incompatible for %s: %s", pkg.Name, reason)
		}
		if pkg.Signature != "" {
			signature, _ := capsulePath(root, pkg.Signature)
			if err := run.Run("pacman-key", "--verify", signature, path); err != nil {
				return nil, fmt.Errorf("restored artifact signature invalid for %s: %w", pkg.Name, err)
			}
		}
		result[pkg.Name] = path
	}
	return result, nil
}

func capsulePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("absolute artifact path refused")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes capsule")
	}
	return filepath.Join(root, clean), nil
}
