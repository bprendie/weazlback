package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	tea "github.com/charmbracelet/bubbletea"
)

type bundleSafetyMsg struct{ err error }
type bundlePreparedMsg struct {
	components []restoretxn.Component
	basket     map[string]restoreBasketItem
	err        error
}

func (m Model) prepareBundleTransaction() (tea.Model, tea.Cmd) {
	profiles := map[restoretxn.Bundle]string{}
	for bundle, selected := range m.restoreBundleChoices {
		if !selected {
			continue
		}
		switch bundle {
		case restoretxn.SystemConfig:
			profiles[bundle] = "core"
		case restoretxn.PersonalFiles:
			profiles[bundle] = "home"
		case restoretxn.HeavyData:
			profiles[bundle] = "heavy"
		}
	}
	if len(profiles) == 0 {
		m.err = "select at least one file bundle"
		return m, nil
	}
	components, err := restoretxn.ComposeNearest(m.snapshots, m.selectedRestoreMachineID(), m.restoreBundleTime, profiles)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.status = true, "resolving independent bundle boundaries"
	cfg := m.cfg
	return m, func() tea.Msg {
		service := restic.NewService(io.Discard)
		basket := map[string]restoreBasketItem{}
		for _, component := range components {
			paths := bundlePaths(cfg, component, nil)
			if component.Bundle == restoretxn.PersonalFiles {
				files, loadErr := service.Files(context.Background(), repo, component.Snapshot.ID)
				if loadErr != nil {
					return bundlePreparedMsg{err: loadErr}
				}
				paths = bundlePaths(cfg, component, files)
			}
			for _, path := range paths {
				basket[path] = restoreBasketItem{Path: path, Snapshot: component.Snapshot.ID, MachineID: component.MachineID,
					Profile: component.Profile, Time: component.Snapshot.Time}
			}
		}
		return bundlePreparedMsg{components: components, basket: basket}
	}
}

func bundlePaths(cfg config.Config, component restoretxn.Component, files []restic.FileEntry) []string {
	profile := config.Profile{}
	for _, candidate := range cfg.Profiles {
		if candidate.Name == component.Profile {
			profile = candidate
			break
		}
	}
	paths := append([]string(nil), profile.Includes...)
	if component.Bundle == restoretxn.SystemConfig {
		return paths
	}
	if component.Bundle == restoretxn.PersonalFiles && len(files) > 0 {
		return personalBundlePaths(files, cfg)
	}
	if len(component.Snapshot.Paths) > 0 {
		var repositoryPaths []string
		for _, path := range component.Snapshot.Paths {
			if !strings.Contains(path, "weazlback/staging/applications-") {
				repositoryPaths = append(repositoryPaths, filepath.Clean(path))
			}
		}
		if len(repositoryPaths) > 0 {
			paths = repositoryPaths
		}
	}
	return paths
}

func personalBundlePaths(files []restic.FileEntry, cfg config.Config) []string {
	home := ""
	if len(files) > 0 {
		for _, path := range files {
			if strings.HasPrefix(path.Path, "/home/") {
				parts := strings.Split(strings.TrimPrefix(path.Path, "/"), "/")
				if len(parts) >= 2 {
					home = "/" + filepath.Join(parts[0], parts[1])
					break
				}
			}
		}
	}
	if home == "" {
		return nil
	}
	var core []string
	for _, profile := range cfg.Profiles {
		if profile.Name == "core" {
			for _, path := range profile.Includes {
				if path == home || strings.HasPrefix(path, home+"/") {
					core = append(core, filepath.Clean(path))
				}
			}
		}
	}
	children := map[string]map[string]bool{}
	for _, entry := range files {
		path := filepath.Clean(entry.Path)
		if path == home || !strings.HasPrefix(path, home+"/") {
			continue
		}
		parent := filepath.Dir(path)
		if children[parent] == nil {
			children[parent] = map[string]bool{}
		}
		children[parent][path] = true
	}
	var selected []string
	var walk func(string)
	walk = func(path string) {
		inside, contains := false, false
		for _, boundary := range core {
			inside = inside || path == boundary || strings.HasPrefix(path, boundary+"/")
			contains = contains || strings.HasPrefix(boundary, path+"/")
		}
		if inside {
			return
		}
		if !contains {
			selected = append(selected, path)
			return
		}
		for child := range children[path] {
			walk(child)
		}
	}
	for child := range children[home] {
		walk(child)
	}
	return selected
}

func (m Model) previousBundleTime(direction int) Model {
	id := m.selectedRestoreMachineID()
	var points []restic.Snapshot
	for _, point := range m.snapshots {
		if restic.IdentityID(point) == id && restic.SnapshotHealth(point.Tags) == "healthy" {
			points = append(points, point)
		}
	}
	if len(points) == 0 {
		return m
	}
	current := 0
	best := points[0].Time.Sub(m.restoreBundleTime)
	if best < 0 {
		best = -best
	}
	for i := range points {
		distance := points[i].Time.Sub(m.restoreBundleTime)
		if distance < 0 {
			distance = -distance
		}
		if distance < best {
			best, current = distance, i
		}
	}
	current = max(0, min(len(points)-1, current+direction))
	m.restoreBundleTime = points[current].Time
	m.status = "requested bundle time: " + m.restoreBundleTime.Local().Format("2006-01-02 15:04")
	return m
}

func bundleChoiceLabel(choices map[restoretxn.Bundle]bool, bundle restoretxn.Bundle) string {
	if choices[bundle] {
		return "[✓]"
	}
	return "[ ]"
}

func (m Model) updateBundleComponentsKey(key string) (tea.Model, tea.Cmd) {
	toggle := func(bundle restoretxn.Bundle) { m.restoreBundleChoices[bundle] = !m.restoreBundleChoices[bundle] }
	switch key {
	case "1":
		toggle(restoretxn.SystemConfig)
	case "2":
		toggle(restoretxn.PersonalFiles)
	case "3":
		toggle(restoretxn.HeavyData)
	case "4":
		m.restoreBundleChoices = map[restoretxn.Bundle]bool{restoretxn.SystemConfig: true, restoretxn.PersonalFiles: true, restoretxn.HeavyData: true}
	case "[":
		m = m.previousBundleTime(1)
	case "]":
		m = m.previousBundleTime(-1)
	case "enter":
		m.restoreStage = "bundle-mode"
	case "esc":
		m.restoreStage = "dashboard"
	}
	return m, nil
}

func (m Model) bundleSummary() string {
	return fmt.Sprintf("Requested time  %s\nSource identity %s", m.restoreBundleTime.Local().Format("2006-01-02 15:04"), m.selectedRestoreMachineID())
}

func (m Model) runBundleSafetyBackup() (tea.Model, tea.Cmd) {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	choices, cfg, vaultFile := m.restoreBundleChoices, m.cfg, m.vault
	m.busy, m.status, m.err = true, "creating quick safety backup before Exact Rewind", ""
	return m, func() tea.Msg {
		ctx := context.Background()
		service := restic.NewService(io.Discard)
		for _, profile := range cfg.Profiles {
			bundle := map[string]restoretxn.Bundle{"core": restoretxn.SystemConfig, "home": restoretxn.PersonalFiles, "heavy": restoretxn.HeavyData}[profile.Name]
			if !choices[bundle] {
				continue
			}
			if profile.Name == "heavy" {
				report := heavy.Inspect(profile.Includes)
				if len(report.Writers) > 0 {
					return bundleSafetyMsg{fmt.Errorf("quick Heavy safety backup blocked: %d writable files are open", len(report.Writers))}
				}
			}
			manifest, cleanup, prepareErr := backupmeta.PrepareApplications(ctx, profile.Name)
			if prepareErr != nil {
				return bundleSafetyMsg{prepareErr}
			}
			includes := append([]string(nil), profile.Includes...)
			if manifest != "" {
				includes = append(includes, manifest)
			}
			excludes := append([]string(nil), profile.Excludes...)
			if profile.Name == "core" || profile.Name == "home" {
				home, _ := os.UserHomeDir()
				excludes = append(excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
			}
			err := service.BackupMachineWithProgress(ctx, repo, profile.Name, cfg.Machine.ID, includes, excludes, false, false, nil)
			cleanup()
			if err != nil {
				return bundleSafetyMsg{err}
			}
			_ = catalog.Refresh(ctx, vaultFile, destination.ID, repo, cfg.Machine.ID, profile.Name)
		}
		return bundleSafetyMsg{}
	}
}
