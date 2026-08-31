package inventory

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func packages(ctx context.Context) PackageInventory {
	explicit := lines(run(ctx, "pacman", "-Qqe"))
	foreignAll := set(lines(run(ctx, "pacman", "-Qqm")))
	installed := installedPackages(run(ctx, "pacman", "-Q"))
	result := PackageInventory{}
	for _, name := range explicit {
		if foreignAll[name] {
			result.ForeignExplicit = append(result.ForeignExplicit, name)
		} else {
			result.OfficialExplicit = append(result.OfficialExplicit, name)
		}
	}
	for _, pkg := range installed {
		if foreignAll[pkg.Name] {
			result.ForeignInstalled = append(result.ForeignInstalled, pkg)
		} else {
			result.OfficialInstalled = append(result.OfficialInstalled, pkg)
		}
	}
	result.FlatpakApps = lines(run(ctx, "flatpak", "list", "--app", "--columns=application"))
	return result
}

func installedPackages(value string) []InstalledPackage {
	var result []InstalledPackage
	for _, line := range lines(value) {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			result = append(result, InstalledPackage{Name: fields[0], Version: fields[1]})
		}
	}
	return result
}

func services(ctx context.Context) ServiceInventory {
	return ServiceInventory{
		SystemEnabled: unitNames(run(ctx, "systemctl", "list-unit-files", "--state=enabled", "--no-legend")),
		UserEnabled:   unitNames(run(ctx, "systemctl", "--user", "list-unit-files", "--state=enabled", "--no-legend")),
	}
}

func unitNames(value string) []string {
	var result []string
	for _, line := range lines(value) {
		if fields := strings.Fields(line); len(fields) > 0 {
			result = append(result, fields[0])
		}
	}
	return result
}

func relativeFiles(root string) []string {
	var result []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			result = append(result, rel)
		}
		return nil
	})
	sort.Strings(result)
	return result
}

func lines(value string) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			result = append(result, line)
		}
	}
	sort.Strings(result)
	return result
}

func set(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func classify(entries []PathEntry) {
	for i := range entries {
		base := strings.ToLower(filepath.Base(entries[i].Path))
		switch {
		case base == ".cache", base == "node_modules", base == "go", base == ".npm":
			entries[i].Classification = "exclude"
			entries[i].Reason = "reproducible cache or dependency data"
		case base == "containers", base == "isos", base == "videos", base == "music":
			entries[i].Classification = "heavy"
			entries[i].Reason = "large or churn-heavy data"
		case base == ".config", strings.HasPrefix(base, ".weazl"), base == "pkginstall":
			entries[i].Classification = "core"
			entries[i].Reason = "configuration or application state"
		default:
			entries[i].Classification = "home"
			entries[i].Reason = "normal user data; review before v1"
		}
	}
}

func classifyConfig(entries []PathEntry) {
	for i := range entries {
		base := strings.ToLower(filepath.Base(entries[i].Path))
		if base == "go" {
			entries[i].Classification = "exclude"
			entries[i].Reason = "reproducible language cache"
			continue
		}
		entries[i].Classification = "core"
		entries[i].Reason = "application or desktop configuration/state"
	}
}
