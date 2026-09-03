package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/platform"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
)

func loadBundleManifest(ctx context.Context, service restic.Service, repo restic.Repository, components []restoretxn.Component) (inventory.ApplicationManifest, error) {
	var core *restoretxn.Component
	for i := range components {
		if components[i].Bundle == restoretxn.SystemConfig {
			core = &components[i]
			break
		}
	}
	if core == nil {
		return inventory.ApplicationManifest{}, errors.New("Core component is required to resolve restore compatibility")
	}
	files, err := service.Files(ctx, repo, core.Snapshot.ID)
	if err != nil {
		return inventory.ApplicationManifest{}, err
	}
	manifestPath := ""
	for _, file := range files {
		if filepath.Base(file.Path) == backupmeta.ManifestName {
			manifestPath = file.Path
			break
		}
	}
	if manifestPath == "" {
		return inventory.NormalizeApplications(inventory.ApplicationManifest{SchemaVersion: 1, Omarchy: "legacy"}, sourceHome(files)), nil
	}
	stage, err := os.MkdirTemp("", "weazlback-compatibility-")
	if err != nil {
		return inventory.ApplicationManifest{}, err
	}
	defer os.RemoveAll(stage)
	if err := service.Restore(ctx, repo, core.Snapshot.ID, stage, []string{filepath.Dir(manifestPath)}); err != nil {
		return inventory.ApplicationManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(stage, strings.TrimPrefix(filepath.Clean(manifestPath), string(filepath.Separator))))
	if err != nil {
		return inventory.ApplicationManifest{}, err
	}
	var manifest inventory.ApplicationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if err := inventory.ValidateApplications(manifest); err != nil {
		return manifest, err
	}
	return inventory.NormalizeApplications(manifest, sourceHome(files)), nil
}

func personalBundlePathsClaims(files []restic.FileEntry, claims []platform.Claim) []string {
	home := sourceHome(files)
	var boundaries []string
	for _, claim := range claims {
		if claim.Path != "" {
			boundaries = append(boundaries, filepath.Clean(claim.Path))
		}
	}
	return personalBundlePathsExcluding(files, home, boundaries)
}

func sourceHome(files []restic.FileEntry) string {
	for _, entry := range files {
		if !strings.HasPrefix(entry.Path, "/home/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(entry.Path, "/"), "/")
		if len(parts) >= 2 {
			return "/" + filepath.Join(parts[0], parts[1])
		}
	}
	return ""
}

func bundleScope(choices map[restoretxn.Bundle]bool) string {
	if choices[restoretxn.HeavyData] {
		return "everything"
	}
	if choices[restoretxn.PersonalFiles] {
		return "core-home"
	}
	return "core"
}

func bundleChoiceLabel(choices map[restoretxn.Bundle]bool, bundle restoretxn.Bundle) string {
	if choices[bundle] {
		return "[✓]"
	}
	return "[ ]"
}

func combinedBundleChoiceLabel(choices map[restoretxn.Bundle]bool, heavy bool) string {
	selected := choices[restoretxn.SystemConfig] && choices[restoretxn.PersonalFiles]
	if heavy {
		selected = selected && choices[restoretxn.HeavyData]
	} else {
		selected = selected && !choices[restoretxn.HeavyData]
	}
	if selected {
		return "[✓]"
	}
	return "[ ]"
}
