package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ApplicationSchemaVersion = 1

type ApplicationManifest struct {
	SchemaVersion  int                `json:"schema_version"`
	CapturedAt     time.Time          `json:"captured_at"`
	Hostname       string             `json:"hostname"`
	Omarchy        string             `json:"omarchy_version,omitempty"`
	Packages       PackageInventory   `json:"packages"`
	PackagePlan    PackageRestorePlan `json:"package_restore_plan"`
	Services       ServiceInventory   `json:"services"`
	FlatpakRemotes []string           `json:"flatpak_remotes,omitempty"`
	OmarchyPlugins []string           `json:"omarchy_plugins,omitempty"`
	ShellPlugins   []string           `json:"shell_plugins,omitempty"`
	PkgInstall     []string           `json:"pkginstall_files,omitempty"`
	PkgInstallPlan []InstallIntent    `json:"pkginstall_plan,omitempty"`
	ManualReview   []string           `json:"manual_review,omitempty"`
	WeazlApps      []string           `json:"weazl_apps,omitempty"`
	AURArtifacts   []PackageArtifact  `json:"aur_artifacts,omitempty"`
}

type PackageArtifact struct {
	Package string `json:"package"`
	Version string `json:"version"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
}

type InstallIntent struct {
	Source   string   `json:"source"`
	Line     int      `json:"line"`
	Manager  string   `json:"manager"`
	Packages []string `json:"packages"`
}

type PackageRestorePlan struct {
	Official []string `json:"official"`
	AUR      []string `json:"aur"`
	Flatpak  []string `json:"flatpak,omitempty"`
}

func CaptureApplications(ctx context.Context) (ApplicationManifest, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ApplicationManifest{}, err
	}
	hostname, _ := os.Hostname()
	pkgs := packages(ctx)
	m := ApplicationManifest{SchemaVersion: ApplicationSchemaVersion, CapturedAt: time.Now().UTC(),
		Hostname: hostname, Omarchy: firstLine(run(ctx, "omarchy", "version")), Packages: pkgs,
		PackagePlan: PackageRestorePlan{Official: clone(pkgs.OfficialExplicit), AUR: clone(pkgs.ForeignExplicit), Flatpak: clone(pkgs.FlatpakApps)},
		Services:    services(ctx), FlatpakRemotes: lines(run(ctx, "flatpak", "remotes", "--columns=name,url")),
		OmarchyPlugins: directoryNames(filepath.Join(home, ".config", "omarchy", "plugins")),
		ShellPlugins:   discoverShellPlugins(home), PkgInstall: relativeFiles(filepath.Join(home, "pkginstall"))}
	m.WeazlApps = discoverWeazlApps(home)
	m.PkgInstallPlan = parsePkgInstall(filepath.Join(home, "pkginstall"), m.PkgInstall)
	for _, file := range m.PkgInstall {
		m.ManualReview = append(m.ManualReview, filepath.Join("pkginstall", file))
	}
	return m, nil
}

func discoverWeazlApps(home string) []string {
	matches, _ := filepath.Glob(filepath.Join(home, ".*weazl*"))
	var apps []string
	for _, root := range matches {
		if info, err := os.Stat(filepath.Join(root, "bin")); err == nil && info.IsDir() {
			apps = append(apps, strings.TrimPrefix(filepath.Base(root), "."))
		}
	}
	sort.Strings(apps)
	return apps
}

func WriteApplications(path string, manifest ApplicationManifest) error {
	if err := ValidateApplications(manifest); err != nil {
		return err
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func ValidateApplications(manifest ApplicationManifest) error {
	if manifest.SchemaVersion != ApplicationSchemaVersion {
		return errors.New("unsupported application manifest schema")
	}
	if manifest.CapturedAt.IsZero() || manifest.Hostname == "" {
		return errors.New("application manifest identity is incomplete")
	}
	if len(manifest.Packages.OfficialInstalled)+len(manifest.Packages.ForeignInstalled) == 0 {
		return errors.New("application package inventory is empty")
	}
	return nil
}

func directoryNames(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func discoverShellPlugins(home string) []string {
	var result []string
	for _, root := range []string{filepath.Join(home, ".oh-my-zsh", "custom", "plugins"), filepath.Join(home, ".config", "fish", "functions")} {
		for _, name := range directoryNames(root) {
			result = append(result, strings.TrimPrefix(root, home+string(filepath.Separator))+"/"+name)
		}
	}
	sort.Strings(result)
	return result
}

func clone(values []string) []string { return append([]string(nil), values...) }
