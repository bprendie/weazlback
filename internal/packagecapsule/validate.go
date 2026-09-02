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
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := Validate(root, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Validate(root string, manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion || manifest.CapturedAt.IsZero() || manifest.Hostname == "" {
		return errors.New("package capsule identity is incomplete or unsupported")
	}
	if len(manifest.Packages) == 0 || manifest.Summary.Installed != len(manifest.Packages) {
		return errors.New("package capsule inventory is incomplete")
	}
	captured := 0
	for _, pkg := range manifest.Packages {
		if pkg.Name == "" || pkg.Installed == "" || (pkg.Source != "official" && pkg.Source != "foreign") {
			return fmt.Errorf("invalid package ledger entry %q", pkg.Name)
		}
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
