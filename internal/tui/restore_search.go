package tui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

type restoreSearchMsg struct {
	request uint64
	results []catalog.PathRecord
	files   []restic.FileEntry
	count   int
	err     error
}

func (m Model) searchParameters() (query, scope string) {
	query = strings.TrimSpace(m.restoreInput.Value())
	scope = m.searchScope
	if strings.HasPrefix(query, "p ") {
		path := m.sourceRestorePath(strings.TrimSpace(strings.TrimPrefix(query, "p ")))
		if path == "" {
			return "", "\x00"
		}
		scope = path
		query = ""
		for scope != "/" {
			if e, ok := m.nodes[scope]; ok && e.Type == "dir" {
				break
			}
			scope = filepath.Dir(scope)
		}
		if path != scope {
			query = strings.TrimPrefix(strings.TrimPrefix(path, scope), "/")
		}
	}
	return
}

func (m Model) queueRestoreSearch() (tea.Model, tea.Cmd) {
	if m.searchCancel != nil {
		m.searchCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.searchCancel = cancel
	m.searchRequest++
	request := m.searchRequest
	m.searchBusy = true
	m.status = "Searching filenames…"
	if m.searchLimit == 0 {
		m.searchLimit = 200
	}
	query, scope := m.searchParameters()
	machine := m.selectedRestoreMachineID()
	return m, func() tea.Msg {
		defer cancel()
		var results []catalog.PathRecord
		var err error
		count := 0
		pathMode := strings.HasPrefix(strings.TrimSpace(m.restoreInput.Value()), "p ")
		if m.restoreDeletedOnly && len(m.snapshots) > 0 {
			filtered := catalog.New()
			for _, record := range deletedCatalogResults(m.restoreCatalog, m.restoreIdentities, m.restoreIdentity, m.snapshots[m.restoreSnapshot].Time) {
				r := record
				filtered.Paths[r.Path] = &r
			}
			results, count, err = filtered.SearchPaths(ctx, query, machine, scope, m.searchLimit)
		} else if !pathMode || m.restorePathHistory {
			results, count, err = m.restoreCatalog.SearchPaths(ctx, query, machine, scope, m.searchLimit)
		}
		var files []restic.FileEntry
		if len(results) == 0 && err == nil && !m.restoreDeletedOnly {
			for i, e := range m.restoreEntries {
				if i%256 == 0 && ctx.Err() != nil {
					return restoreSearchMsg{request: request, err: ctx.Err()}
				}
				if scope != "" && e.Path != scope && !strings.HasPrefix(e.Path, strings.TrimSuffix(scope, "/")+"/") {
					continue
				}
				matchPath := e.Path
				if scope != "" {
					matchPath = strings.TrimPrefix(strings.TrimPrefix(e.Path, scope), "/")
				}
				if catalog.MatchPath(matchPath, query) {
					count++
					if len(files) < m.searchLimit {
						files = append(files, e)
					}
				}
			}
		}
		return restoreSearchMsg{request: request, results: results, files: files, count: count, err: err}
	}
}

func (m Model) updateRestoreSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+e":
		m.openRestoreBasket()
		return m, nil
	case "esc":
		if m.searchCancel != nil {
			m.searchCancel()
		}
		m.searchRequest++
		m.searchBusy = false
		m.restoreSearching = false
		m.restoreDeletedOnly = false
		m.restoreInput.Blur()
		m.filterRestoreTree()
		m.restoreResults = nil
		m.restoreIndex = min(m.browserIndex, max(0, len(m.restoreVisible)-1))
		return m, nil
	case "ctrl+a":
		m.restoreAllMachines = !m.restoreAllMachines
	case "ctrl+h":
		m.restorePathHistory = !m.restorePathHistory
	case "ctrl+g":
		m.searchScope = ""
		m.restoreInput.SetValue("")
	case "ctrl+l":
		m.searchLimit += 200
	case "ctrl+r":
		return m.refreshRestoreHistory()
	case "ctrl+t":
		m.openRestorePoints()
		return m, nil
	case "enter":
		if m.searchBusy {
			m.status = "Searching…"
			return m, nil
		}
		return m.applyRestoreSearch()
	case "up", "down", "pgup", "pgdown", "ctrl+n", "ctrl+p":
		if key == "ctrl+n" {
			key = "down"
		}
		if key == "ctrl+p" {
			key = "up"
		}
		m.restoreIndex = moveList(m.restoreIndex, max(len(m.restoreVisible), len(m.restoreResults)), m.restorePageSize(), key)
		return m, nil
	default:
		var cmd tea.Cmd
		m.restoreInput, cmd = m.restoreInput.Update(msg)
		next, search := m.queueRestoreSearch()
		return next, tea.Batch(cmd, search)
	}
	m.restoreIndex = 0
	return m.queueRestoreSearch()
}
