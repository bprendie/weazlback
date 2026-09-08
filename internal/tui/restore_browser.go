package tui

import (
	"context"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/catalog"
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
		path := m.sourceRestorePath(strings.TrimSpace(strings.TrimPrefix(query, "p ")))
		if path == "" {
			m.err = "Source home is ambiguous; enter an absolute repository path"
			return m, nil
		}
		if entry, ok := m.nodes[path]; ok {
			if entry.Type == "dir" {
				m.restoreSearching, m.restorePathMode = false, true
				m.restoreInput.Blur()
				m.restoreTreePath = filepath.Clean(path)
				m.searchScope = m.restoreTreePath
				m.restoreIndex = 0
				m.filterRestore()
				m.status = "path mode: " + m.restoreTreePath
				return m, nil
			}
			m.openRestoreVersions(entry.Path)
			return m, nil
		}
	}
	if len(m.restoreResults) > 0 {
		m.openRestoreVersions(m.restoreResults[m.restoreIndex].Path)
		return m, nil
	}
	if len(m.restoreVisible) > 0 {
		entry := m.restoreVisible[m.restoreIndex]
		if entry.Type == "dir" {
			m.searchScope, m.restoreTreePath, m.restoreSearching, m.restoreIndex = entry.Path, entry.Path, false, 0
			m.restoreInput.Blur()
			m.filterRestore()
		} else {
			m.openRestoreVersions(entry.Path)
		}
		return m, nil
	}
	return m, func() tea.Msg {
		hints, err := catalog.LivePathHints(query, 8)
		return restoreLiveHintsMsg{hints: hints, err: err}
	}
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
	m.loadSpecific, m.loadIndex = true, index
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if repo.Elevated && !m.restoreAuthorized {
		return m, authorizeRestoreLoad(index, true)
	}
	m.busy, m.status = true, "loading selected restore point"
	m.restoreStage = "load"
	if m.loadCancel != nil {
		m.loadCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.loadCancel = cancel
	m.loadRequest++
	request := m.loadRequest
	snapshots := append([]restic.Snapshot(nil), m.snapshots...)
	return m, func() tea.Msg {
		defer cancel()
		files, err := restic.NewService(io.Discard).Files(ctx, repo, snapshots[index].ID)
		return restoreBrowseMsg{tree: prepareRestoreTree(files), request: request, snapshots: snapshots, files: files, index: index, err: err}
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

func (m *Model) filterRestoreSearch(_ string) {
	query, scope := m.searchParameters()
	m.restoreResults, m.searchCount, _ = m.restoreCatalog.SearchPaths(context.Background(), query, m.selectedRestoreMachineID(), scope, 200)
	if len(m.restoreResults) == 0 {
		for _, entry := range m.restoreEntries {
			if scope != "" && entry.Path != scope && !strings.HasPrefix(entry.Path, strings.TrimSuffix(scope, "/")+"/") {
				continue
			}
			if catalog.MatchPath(strings.TrimPrefix(entry.Path, scope), query) {
				m.searchCount++
				if len(m.restoreVisible) < 200 {
					m.restoreVisible = append(m.restoreVisible, entry)
				}
			}
		}
	}
}

func (m *Model) filterRestoreTree() {
	if !m.treeReady {
		m.buildRestoreTree()
	}
	m.restoreVisible = nil
	var walk func(string)
	walk = func(parent string) {
		for _, child := range m.children[parent] {
			m.restoreVisible = append(m.restoreVisible, m.nodes[child])
			if m.expanded[child] {
				walk(child)
			}
		}
	}
	walk(filepath.Clean(m.restoreTreePath))
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
