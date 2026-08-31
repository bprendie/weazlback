package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

const contentSearchMaxFile = 2 << 20

type contentSearchMsg struct {
	files []restic.FileEntry
	err   error
}

func (m Model) startContentSearch() (tea.Model, tea.Cmd) {
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if len(m.snapshots) == 0 || m.restoreSnapshot >= len(m.snapshots) {
		m.err = "no Restore Point selected"
		return m, nil
	}
	query := strings.ToLower(strings.TrimSpace(m.restoreContentQuery))
	if query == "" {
		m.err = "content query is empty"
		return m, nil
	}
	snapshot := m.snapshots[m.restoreSnapshot].ID
	var candidates []restic.FileEntry
	for _, entry := range m.restoreEntries {
		if entry.Type == "file" && entry.Size <= contentSearchMaxFile && pathInside(m.restoreTreePath, entry.Path) {
			candidates = append(candidates, entry)
		}
	}
	m.busy, m.restoreStage, m.status, m.err = true, "content-running", fmt.Sprintf("searching %d bounded files without retaining contents", len(candidates)), ""
	return m, func() tea.Msg {
		service := restic.NewService(io.Discard)
		jobs := make(chan restic.FileEntry)
		matches := make(chan restic.FileEntry, len(candidates))
		var wg sync.WaitGroup
		for range min(4, max(1, len(candidates))) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for entry := range jobs {
					data, dumpErr := service.Dump(context.Background(), repo, snapshot, entry.Path)
					if dumpErr == nil && !bytes.Contains(data, []byte{0}) && bytes.Contains(bytes.ToLower(data), []byte(query)) {
						matches <- entry
					}
				}
			}()
		}
		for _, entry := range candidates {
			jobs <- entry
		}
		close(jobs)
		wg.Wait()
		close(matches)
		var found []restic.FileEntry
		for entry := range matches {
			found = append(found, entry)
		}
		return contentSearchMsg{files: found}
	}
}

func pathInside(root, path string) bool {
	root = strings.TrimSuffix(root, "/")
	return path == root || strings.HasPrefix(path, root+"/")
}
