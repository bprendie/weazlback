package packagecapsule

import "fmt"

// SelectedManifest checks the full ledger but limits file verification to the
// reviewed transaction. Unrelated or foreign archives need not be extracted.
func SelectedManifest(manifest Manifest, names []string) (Manifest, error) {
	if err := ValidateLedger(manifest); err != nil {
		return Manifest{}, err
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	selected := manifest
	selected.Packages = nil
	selected.Summary = Summary{}
	for _, pkg := range manifest.Packages {
		if !wanted[pkg.Name] {
			continue
		}
		delete(wanted, pkg.Name)
		selected.Packages = append(selected.Packages, pkg)
		selected.Summary.Installed++
		if pkg.Artifact != "" {
			selected.Summary.Captured++
		}
	}
	if len(wanted) > 0 {
		return Manifest{}, fmt.Errorf("selected package missing from capsule ledger")
	}
	return selected, nil
}
