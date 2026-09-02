package packagecapsule

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const ManifestName = "weazlback-package-capsule-v1.json"

func Capture(options Options) (Manifest, string, func(), error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Run == nil {
		options.Run = ExecRunner{Context: options.Context}
	}
	root, cleanup, err := newStaging(options.StagingRoot)
	if err != nil {
		return Manifest{}, "", func() {}, err
	}
	manifest, err := inventoryManifest(options)
	if err != nil {
		cleanup()
		return Manifest{}, "", func() {}, err
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		cleanup()
		return Manifest{}, "", func() {}, err
	}
	if options.Download {
		downloadOfficial(options, root, &manifest)
	}
	if err := options.Context.Err(); err != nil {
		cleanup()
		return Manifest{}, "", func() {}, err
	}
	if options.BuildMissingAUR {
		buildForeign(options, root, &manifest)
	}
	if err := options.Context.Err(); err != nil {
		cleanup()
		return Manifest{}, "", func() {}, err
	}
	harvest(options, root, &manifest)
	path := filepath.Join(root, ManifestName)
	if err := writeManifest(path, manifest); err != nil {
		cleanup()
		return Manifest{}, "", func() {}, err
	}
	if err := Validate(root, manifest); err != nil {
		cleanup()
		return Manifest{}, "", func() {}, fmt.Errorf("validate package capsule: %w", err)
	}
	return manifest, root, cleanup, nil
}

func inventoryManifest(options Options) (Manifest, error) {
	installedOutput, err := options.Run.Output("pacman", "-Q")
	if err != nil {
		return Manifest{}, fmt.Errorf("inventory installed packages: %w", err)
	}
	explicitOutput, err := options.Run.Output("pacman", "-Qqe")
	if err != nil {
		return Manifest{}, fmt.Errorf("inventory explicit packages: %w", err)
	}
	foreignOutput, err := options.Run.Output("pacman", "-Qqm")
	if err != nil {
		return Manifest{}, fmt.Errorf("inventory foreign packages: %w", err)
	}
	installed := parseInstalled(installedOutput)
	if len(installed) == 0 {
		return Manifest{}, errors.New("installed package inventory is empty")
	}
	hostname, _ := os.Hostname()
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	explicit, foreign := stringSet(nonemptyLines(explicitOutput)), stringSet(nonemptyLines(foreignOutput))
	packages := make([]Package, 0, len(installed))
	officialCount, foreignCount := 0, 0
	for _, item := range installed {
		reason := "dependency"
		if explicit[item.Name] {
			reason = "explicit"
		}
		source := "official"
		if foreign[item.Name] {
			source, foreignCount = "foreign", foreignCount+1
		} else {
			officialCount++
		}
		packages = append(packages, Package{Name: item.Name, Installed: item.Version, Source: source, Reason: reason})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	flatpaks, _ := options.Run.Output("flatpak", "list", "--app", "--columns=application")
	systemUnits, _ := options.Run.Output("systemctl", "list-unit-files", "--state=enabled", "--no-legend")
	userUnits, _ := options.Run.Output("systemctl", "--user", "list-unit-files", "--state=enabled", "--no-legend")
	return Manifest{SchemaVersion: SchemaVersion, CapturedAt: time.Now().UTC(), Hostname: hostname,
		Architecture: arch, MachineID: options.MachineID, Packages: packages,
		Flatpaks: nonemptyLines(flatpaks), SystemUnits: unitNames(systemUnits), UserUnits: unitNames(userUnits),
		Summary: Summary{Installed: len(packages), Official: officialCount, Foreign: foreignCount}}, nil
}

type installedPackage struct{ Name, Version string }

func parseInstalled(value string) []installedPackage {
	var result []installedPackage
	for _, line := range nonemptyLines(value) {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			result = append(result, installedPackage{Name: fields[0], Version: fields[1]})
		}
	}
	return result
}

func nonemptyLines(value string) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			result = append(result, line)
		}
	}
	return result
}

func unitNames(value string) []string {
	var result []string
	for _, line := range nonemptyLines(value) {
		if fields := strings.Fields(line); len(fields) > 0 {
			result = append(result, fields[0])
		}
	}
	return result
}

func newStaging(parent string) (string, func(), error) {
	if parent == "" {
		return "", func() {}, errors.New("package capsule staging root is required")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", func() {}, err
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return "", func() {}, err
	}
	root, err := os.MkdirTemp(parent, "packages-")
	if err != nil {
		return "", func() {}, err
	}
	return root, func() { _ = os.RemoveAll(root) }, nil
}

func writeManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
