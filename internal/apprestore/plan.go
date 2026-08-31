package apprestore

import (
	"sort"

	"github.com/bprendie/weazlback/internal/inventory"
)

func Build(machineID, snapshot string, manifest, current inventory.ApplicationManifest, resolver Resolver) Plan {
	plan := Plan{MachineID: machineID, Snapshot: snapshot, Timestamp: manifest.CapturedAt, Manifest: manifest}
	desired := desiredPackages(manifest)
	installed := installedPackages(current)
	explicit := explicitPackages(current)
	for key, wanted := range desired {
		if actual, ok := installed[key]; ok {
			wanted.CurrentVersion = actual.CurrentVersion
			plan.Unchanged = append(plan.Unchanged, wanted)
			delete(installed, key)
			continue
		}
		if conflict, ok := installed[packageKey(wanted.Name, opposite(wanted.Source))]; ok {
			wanted.CurrentVersion = conflict.CurrentVersion
			plan.Conflicts = append(plan.Conflicts, wanted)
			delete(installed, packageKey(wanted.Name, opposite(wanted.Source)))
			continue
		}
		available, ok := resolver.Available(wanted.Name, wanted.Source)
		wanted.AvailableVersion = available
		if !ok {
			plan.Unavailable = append(plan.Unavailable, wanted)
		} else if wanted.WantedVersion != "" && available != "" && available != wanted.WantedVersion {
			plan.Substitutions = append(plan.Substitutions, wanted)
		} else {
			plan.Install = append(plan.Install, wanted)
		}
	}
	for key, pkg := range installed {
		if explicit[key] {
			plan.InstalledLater = append(plan.InstalledLater, pkg)
		}
	}
	plan.SystemServices = missingStrings(manifest.Services.SystemEnabled, current.Services.SystemEnabled)
	plan.UserServices = missingStrings(manifest.Services.UserEnabled, current.Services.UserEnabled)
	sortPlan(&plan)
	return plan
}

func explicitPackages(manifest inventory.ApplicationManifest) map[string]bool {
	result := map[string]bool{}
	for _, name := range manifest.Packages.OfficialExplicit {
		result[packageKey(name, Official)] = true
	}
	for _, name := range manifest.Packages.ForeignExplicit {
		result[packageKey(name, AUR)] = true
	}
	for _, name := range manifest.Packages.FlatpakApps {
		result[packageKey(name, Flatpak)] = true
	}
	return result
}

func desiredPackages(manifest inventory.ApplicationManifest) map[string]Package {
	versions := map[string]string{}
	for _, pkg := range append(append([]inventory.InstalledPackage(nil), manifest.Packages.OfficialInstalled...), manifest.Packages.ForeignInstalled...) {
		versions[pkg.Name] = pkg.Version
	}
	result := map[string]Package{}
	for _, name := range manifest.PackagePlan.Official {
		pkg := Package{Name: name, WantedVersion: versions[name], Source: Official}
		result[packageKey(name, Official)] = pkg
	}
	for _, name := range manifest.PackagePlan.AUR {
		pkg := Package{Name: name, WantedVersion: versions[name], Source: AUR}
		result[packageKey(name, AUR)] = pkg
	}
	for _, name := range manifest.PackagePlan.Flatpak {
		pkg := Package{Name: name, Source: Flatpak}
		result[packageKey(name, Flatpak)] = pkg
	}
	return result
}

func installedPackages(manifest inventory.ApplicationManifest) map[string]Package {
	result := map[string]Package{}
	for _, pkg := range manifest.Packages.OfficialInstalled {
		result[packageKey(pkg.Name, Official)] = Package{Name: pkg.Name, CurrentVersion: pkg.Version, Source: Official}
	}
	for _, pkg := range manifest.Packages.ForeignInstalled {
		result[packageKey(pkg.Name, AUR)] = Package{Name: pkg.Name, CurrentVersion: pkg.Version, Source: AUR}
	}
	for _, name := range manifest.Packages.FlatpakApps {
		result[packageKey(name, Flatpak)] = Package{Name: name, Source: Flatpak}
	}
	return result
}

func packageKey(name string, source Source) string { return string(source) + "\x00" + name }

func opposite(source Source) Source {
	if source == Official {
		return AUR
	}
	if source == AUR {
		return Official
	}
	return source
}

func missingStrings(wanted, actual []string) []string {
	seen := map[string]bool{}
	for _, value := range actual {
		seen[value] = true
	}
	var result []string
	for _, value := range wanted {
		if !seen[value] {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func sortPlan(plan *Plan) {
	less := func(values []Package) {
		sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	}
	less(plan.Install)
	less(plan.Substitutions)
	less(plan.Unavailable)
	less(plan.Conflicts)
	less(plan.Unchanged)
	less(plan.InstalledLater)
}
