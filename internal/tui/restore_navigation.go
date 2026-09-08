package tui

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

// Repository nodes are immutable after loading. Expansion belongs to the UI.
func prepareRestoreTree(entries []restic.FileEntry) *restoreNavigation {
	m := Model{restoreEntries: entries}
	m.buildRestoreTree()
	return &m.restoreNavigation
}

type restoreNavigation struct {
	nodes             map[string]restic.FileEntry
	children          map[string][]string
	expanded          map[string]bool
	treeReady         bool
	versionIndex      int
	returnSearch      bool
	browserIndex      int
	searchScope       string
	searchCount       int
	searchLimit       int
	searchRequest     uint64
	searchCancel      context.CancelFunc
	searchBusy        bool
	loadRequest       uint64
	loadSpecific      bool
	loadIndex         int
	loadCancel        context.CancelFunc
	catalogRequest    uint64
	catalogCancel     context.CancelFunc
	pointIndex        int
	pointChoices      []int
	basketIndex       int
	pendingBasket     *restoreBasketItem
	restoreAuthorized bool
	detailText        string
	detailReturn      string
	detailOffset      int
}

func (m Model) enterRestoreWorkspace() (tea.Model, tea.Cmd) {
	if m.busy {
		m.status = "Restore Mode unavailable while backup work is active"
		return m, nil
	}
	m.backupMode, m.backupIndex, m.backupRailFocused = m.mode, m.index, m.railFocused
	m.workspace, m.mode, m.index, m.restoreStage, m.railFocused = "restore", modeRestore, navigationIndex(modeRestore), "dashboard", false
	m.status = "Restore Mode"
	return m, nil
}

func (m Model) restoreTextFocused() bool {
	if m.workspace != "restore" {
		return false
	}
	switch m.restoreStage {
	case "transaction-alternate", "transaction-confirm", "bundle-understand", "point-date":
		return true
	case "browse":
		return m.restoreSearching
	}
	return false
}

func (m *Model) buildRestoreTree() {
	m.nodes = make(map[string]restic.FileEntry, len(m.restoreEntries))
	m.children = make(map[string][]string)
	m.expanded = map[string]bool{}
	for _, entry := range m.restoreEntries {
		entry.Path = filepath.Clean(entry.Path)
		entry.Name = filepath.Base(entry.Path)
		m.nodes[entry.Path] = entry
	}
	// Synthesize missing ancestors once; each node has precisely one parent.
	for _, entry := range m.restoreEntries {
		for p := filepath.Dir(entry.Path); ; p = filepath.Dir(p) {
			if _, ok := m.nodes[p]; ok {
				break
			}
			m.nodes[p] = restic.FileEntry{Path: p, Name: filepath.Base(p), Type: "dir"}
			if p == "/" {
				break
			}
		}
	}
	for p := range m.nodes {
		if p != "/" {
			parent := filepath.Dir(p)
			m.children[parent] = append(m.children[parent], p)
		}
	}
	for parent, paths := range m.children {
		sort.Slice(paths, func(i, j int) bool {
			a, b := m.nodes[paths[i]], m.nodes[paths[j]]
			if (a.Type == "dir") != (b.Type == "dir") {
				return a.Type == "dir"
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		m.children[parent] = paths
	}
	m.treeReady = true
}

func (m *Model) selectTreePath(path string) {
	for i, e := range m.restoreVisible {
		if e.Path == path {
			m.restoreIndex = i
			return
		}
	}
	m.restoreIndex = 0
}

func (m *Model) moveRestoreTree(right bool) {
	if len(m.restoreVisible) == 0 {
		return
	}
	entry := m.restoreVisible[m.restoreIndex]
	if right {
		if entry.Type != "dir" {
			m.openRestoreVersions(entry.Path)
			return
		}
		if m.expanded[entry.Path] {
			if m.restoreIndex+1 < len(m.restoreVisible) {
				m.restoreIndex++
			}
			return
		}
		m.expanded[entry.Path] = true
		m.filterRestoreTree()
		m.selectTreePath(entry.Path)
		return
	}
	if m.expanded[entry.Path] {
		delete(m.expanded, entry.Path)
		m.filterRestoreTree()
		m.selectTreePath(entry.Path)
		return
	}
	parent := filepath.Dir(entry.Path)
	if parent == m.restoreTreePath {
		if parent == "/" {
			return
		}
		m.restoreTreePath = filepath.Dir(parent)
		m.filterRestoreTree()
		m.selectTreePath(parent)
	} else {
		m.selectTreePath(parent)
	}
}

func (m Model) restorePageSize() int {
	// Header, outer padding/borders, context, path, details, and footer.
	header := lineHeight(m.header(max(1, m.width-8)))
	extra := 0
	if m.restoreSearching {
		extra = 1
	}
	if len(m.restoreBasket) > 0 {
		extra++
	}
	return max(1, m.height-header-17-extra)
}

func restoreWindow(index, count, limit int) (int, int) {
	start := max(0, index-limit+1)
	return start, min(count, start+limit)
}

func moveList(index, count, page int, key string) int {
	switch key {
	case "up", "k":
		index--
	case "down", "j":
		index++
	case "pgup":
		index -= page
	case "pgdown":
		index += page
	case "home":
		index = 0
	case "end":
		index = count - 1
	}
	return max(0, min(max(0, count-1), index))
}
