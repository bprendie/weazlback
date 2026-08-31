package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) applyRestoreSearch() (tea.Model, tea.Cmd) {
	query := strings.TrimSpace(m.restoreInput.Value())
	if strings.HasPrefix(query, "c ") {
		m.restoreContentQuery = strings.TrimSpace(strings.TrimPrefix(query, "c "))
		m.restoreSearching = false
		m.restoreInput.Blur()
		if !m.restoreContentOpen {
			m.restoreStage = "content-confirm"
			return m, nil
		}
		return m.startContentSearch()
	}
	if strings.HasPrefix(query, "p ") {
		path := expandRestorePath(strings.TrimSpace(strings.TrimPrefix(query, "p ")), m.cfg)
		if entry, ok := findRestoreEntry(m.restoreEntries, path); ok {
			m.restoreSearching, m.restorePathMode = false, true
			m.restoreInput.Blur()
			if entry.Type == "dir" {
				m.restoreTreePath = filepath.Clean(path)
				m.filterRestore()
				m.status = "path mode: " + m.restoreTreePath
				return m, nil
			}
			m.restoreVersionPath, m.restoreStage = entry.Path, "versions"
			return m, nil
		}
	}
	if len(m.restoreResults) > 0 {
		m.restoreVersionPath, m.restoreStage, m.restoreSearching = m.restoreResults[m.restoreIndex].Path, "versions", false
		m.restoreInput.Blur()
		return m, nil
	}
	return m, func() tea.Msg {
		hints, err := catalog.LivePathHints(query, 8)
		return restoreLiveHintsMsg{hints: hints, err: err}
	}
}

func expandRestorePath(value string, cfg config.Config) string {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home := ""
		for _, profile := range cfg.Profiles {
			if profile.Name == "home" && len(profile.Includes) > 0 {
				home = profile.Includes[0]
			}
		}
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return filepath.Clean(value)
}

func findRestoreEntry(entries []restic.FileEntry, path string) (restic.FileEntry, bool) {
	for _, entry := range entries {
		if filepath.Clean(entry.Path) == filepath.Clean(path) {
			return entry, true
		}
	}
	return restic.FileEntry{}, false
}

func (m Model) loadRestorePoint(index int) (tea.Model, tea.Cmd) {
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.status, m.restoreSnapshot = true, "loading selected restore point", index
	snapshots := append([]restic.Snapshot(nil), m.snapshots...)
	return m, func() tea.Msg {
		files, err := restic.NewService(io.Discard).Files(context.Background(), repo, snapshots[index].ID)
		return restoreBrowseMsg{snapshots: snapshots, files: files, index: index, err: err}
	}
}

func (m *Model) filterRestore() {
	query := strings.TrimSpace(m.restoreInput.Value())
	m.restoreVisible = m.restoreVisible[:0]
	m.restoreResults = nil
	m.restoreLiveHints = nil
	if m.restoreSearching {
		m.filterRestoreSearch(query)
	} else {
		m.filterRestoreTree()
	}
	count := len(m.restoreVisible)
	if len(m.restoreResults) > 0 {
		count = len(m.restoreResults)
	}
	if m.restoreIndex >= count {
		m.restoreIndex = max(0, count-1)
	}
}

func (m *Model) filterRestoreSearch(query string) {
	if strings.HasPrefix(query, "p ") {
		pathQuery := strings.ToLower(expandRestorePath(strings.TrimSpace(strings.TrimPrefix(query, "p ")), m.cfg))
		if m.restorePathHistory {
			m.restoreResults = m.restoreCatalog.Search(pathQuery, m.selectedRestoreMachineID(), 200)
			return
		}
		for _, entry := range m.restoreEntries {
			if strings.Contains(strings.ToLower(entry.Path), pathQuery) {
				m.restoreVisible = append(m.restoreVisible, entry)
			}
		}
		return
	}
	m.restoreResults = m.restoreCatalog.Search(query, m.selectedRestoreMachineID(), 200)
	if len(m.restoreResults) == 0 {
		for _, entry := range m.restoreEntries {
			if strings.Contains(strings.ToLower(entry.Path), strings.ToLower(query)) {
				m.restoreVisible = append(m.restoreVisible, entry)
			}
		}
	}
}

func (m *Model) filterRestoreTree() {
	parent := filepath.Clean(m.restoreTreePath)
	seen := map[string]bool{}
	for _, entry := range m.restoreEntries {
		path := filepath.Clean(entry.Path)
		if path == parent {
			continue
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		first := strings.Split(rel, string(filepath.Separator))[0]
		child := filepath.Join(parent, first)
		if seen[child] {
			continue
		}
		seen[child] = true
		if exact, ok := findRestoreEntry(m.restoreEntries, child); ok {
			m.restoreVisible = append(m.restoreVisible, exact)
		} else {
			m.restoreVisible = append(m.restoreVisible, restic.FileEntry{Name: first, Path: child, Type: "dir"})
		}
	}
	sort.Slice(m.restoreVisible, func(i, j int) bool {
		if m.restoreVisible[i].Type != m.restoreVisible[j].Type {
			return m.restoreVisible[i].Type == "dir"
		}
		return strings.ToLower(m.restoreVisible[i].Name) < strings.ToLower(m.restoreVisible[j].Name)
	})
}

func (m Model) selectedRestoreMachineID() string {
	if m.restoreAllMachines || len(m.restoreIdentities) == 0 || m.restoreIdentity >= len(m.restoreIdentities) {
		return ""
	}
	return m.restoreIdentities[m.restoreIdentity].ID
}

func restoreRoot(files []restic.FileEntry) string {
	if len(files) == 0 {
		return "/"
	}
	root := filepath.Clean(files[0].Path)
	for _, file := range files[1:] {
		path := filepath.Clean(file.Path)
		for root != "/" && path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
			root = filepath.Dir(root)
		}
	}
	if entry, ok := findRestoreEntry(files, root); ok && entry.Type != "dir" {
		return filepath.Dir(root)
	}
	return root
}

func deletedCatalogResults(c catalog.Catalog, identities []restic.Identity, selected int, cutoff time.Time) []catalog.PathRecord {
	machineID := ""
	if selected >= 0 && selected < len(identities) {
		machineID = identities[selected].ID
	}
	var result []catalog.PathRecord
	for _, record := range c.Paths {
		filtered := catalog.PathRecord{Path: record.Path, Name: record.Name}
		for _, version := range record.Versions {
			if machineID == "" || version.MachineID == machineID {
				filtered.Versions = append(filtered.Versions, version)
			}
		}
		if deletedAfter(filtered, cutoff) {
			result = append(result, filtered)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func deletedAfter(record catalog.PathRecord, cutoff time.Time) bool {
	hadAtCutoff, deletedLater := false, false
	for _, version := range record.Versions {
		if !version.Time.After(cutoff) && !strings.Contains(version.Change, "-") {
			hadAtCutoff = true
		}
		if version.Time.After(cutoff) && strings.Contains(version.Change, "-") {
			deletedLater = true
		}
	}
	return hadAtCutoff && deletedLater
}
