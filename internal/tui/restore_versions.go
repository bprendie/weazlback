package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openRestoreVersions(path string) {
	m.restoreVersionPath, m.restoreStage, m.versionIndex = path, "versions", 0
	m.returnSearch = m.restoreSearching
	m.restoreSearching = false
	m.restoreInput.Blur()
}

func (m Model) fileVersions() []catalog.Version {
	var versions []catalog.Version
	known := map[string]bool{}
	available := map[string]bool{}
	for _, s := range m.snapshots {
		available[s.ID] = true
	}
	if record := m.restoreCatalog.Paths[filepath.Clean(m.restoreVersionPath)]; record != nil {
		for _, v := range record.Versions {
			if !available[v.SnapshotID] || (m.selectedRestoreMachineID() != "" && v.MachineID != m.selectedRestoreMachineID()) {
				continue
			}
			versions = append(versions, v)
			known[v.SnapshotID] = true
		}
	}
	if len(m.snapshots) > m.restoreSnapshot {
		s := m.snapshots[m.restoreSnapshot]
		if _, ok := findRestoreEntry(m.restoreEntries, m.restoreVersionPath); ok && !known[s.ID] {
			versions = append(versions, catalog.Version{SnapshotID: s.ID, MachineID: restic.IdentityID(s), Profile: restic.Profile(s.Tags), Time: s.Time, Change: "+", Health: restic.SnapshotHealth(s.Tags)})
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].Time.After(versions[j].Time) })
	return versions
}

func (m *Model) chooseRestoreVersion() {
	versions := m.fileVersions()
	if len(versions) == 0 {
		m.err = "No available version in this repository"
		return
	}
	m.versionIndex = min(m.versionIndex, len(versions)-1)
	v := versions[m.versionIndex]
	if strings.Contains(v.Change, "-") {
		for i := m.versionIndex + 1; i < len(versions); i++ {
			if versions[i].MachineID == v.MachineID && versions[i].Profile == v.Profile && !strings.Contains(versions[i].Change, "-") {
				m.versionIndex = i
				m.status = "Deleted at this point; preceding available version selected. Press Space to add it."
				return
			}
		}
		m.err = "No preceding available version for this deletion"
		return
	}
	if current, ok := m.restoreBasket[m.restoreVersionPath]; ok && current.Snapshot == v.SnapshotID {
		delete(m.restoreBasket, m.restoreVersionPath)
		return
	}
	m.addRestoreBasket(restoreBasketItem{Path: m.restoreVersionPath, Snapshot: v.SnapshotID, MachineID: v.MachineID, Profile: v.Profile, Time: v.Time})
}

func (m *Model) addRestoreBasket(item restoreBasketItem) {
	if active := m.cfg.Active(); active != nil {
		item.Destination = active.ID
	}
	if m.restoreBasket == nil {
		m.restoreBasket = map[string]restoreBasketItem{}
	}
	for path, existing := range m.restoreBasket {
		overlap := path == item.Path || strings.HasPrefix(path, item.Path+"/") || strings.HasPrefix(item.Path, path+"/")
		if overlap && existing.Snapshot != item.Snapshot {
			m.pendingBasket = &item
			m.restoreStage = "basket-conflict"
			return
		}
	}
	m.restoreBasket[item.Path] = item
	m.restoreBasket = normalizedBasket(m.restoreBasket)
	m.status = restoreBasketStatus(len(m.restoreBasket))
}

func (m *Model) openRestoreBasket() {
	if len(m.restoreBasket) == 0 {
		m.status = "Select a file or version with Space first"
		return
	}
	m.restoreBundleParts = nil
	m.restoreAuthorized = false
	m.restoreBundleMode = ""
	m.restoreBundleDeletes = nil
	m.restoreStage, m.basketIndex = "basket", 0
}

func (m Model) basketItems() []restoreBasketItem {
	items := make([]restoreBasketItem, 0, len(m.restoreBasket))
	for _, item := range m.restoreBasket {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items
}

func (m Model) restoreNavigationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	if key == "v" && (m.restoreStage == "versions" || m.restoreStage == "browse" && !m.restoreSearching) {
		m.openRestoreDetails()
		return m, nil, true
	}
	switch m.restoreStage {
	case "details":
		if key == "esc" || key == "left" {
			m.restoreStage = m.detailReturn
			return m, nil, true
		}
		m.detailOffset = moveList(m.detailOffset, len(m.restoreDetailLines()), m.restorePageSize(), key)
		return m, nil, true
	case "point-date":
		next, cmd := m.restoreDateKey(msg)
		return next, cmd, true
	case "load", "load-error":
		if key == "esc" {
			if m.loadCancel != nil {
				m.loadCancel()
			}
			m.loadRequest++
			m.busy = false
			m.restoreStage = "dashboard"
			return m, nil, true
		}
		if key == "r" && !m.busy {
			if m.loadSpecific {
				next, cmd := m.loadRestorePoint(m.loadIndex)
				return next, cmd, true
			}
			next, cmd := m.startOrRunRestore()
			return next, cmd, true
		}
		return m, nil, true
	case "points":
		if key == "/" {
			m.restoreStage = "point-date"
			m.restoreInput = newRestoreInput()
			m.restoreInput.Prompt = "Date (Monday or YYYY-MM-DD HH:MM) > "
			m.restoreInput.Focus()
			return m, nil, true
		}
		m.pointIndex = moveList(m.pointIndex, len(m.pointChoices), m.restorePageSize(), key)
		if key == "esc" {
			m.restoreStage = "browse"
		}
		if key == "enter" && len(m.pointChoices) > 0 {
			next, cmd := m.loadRestorePoint(m.pointChoices[m.pointIndex])
			return next, cmd, true
		}
		return m, nil, true
	case "versions":
		m.versionIndex = moveList(m.versionIndex, len(m.fileVersions()), m.restorePageSize(), key)
		switch key {
		case " ", "enter":
			m.chooseRestoreVersion()
		case "e", "r", "ctrl+e":
			m.openRestoreBasket()
		case "esc", "left":
			m.restoreStage = "browse"
			m.restoreSearching = m.returnSearch
			if m.restoreSearching {
				m.restoreInput.Focus()
			}
		}
		return m, nil, true
	case "basket-conflict":
		if key == "y" && m.pendingBasket != nil {
			item := *m.pendingBasket
			for path := range m.restoreBasket {
				if path == item.Path || strings.HasPrefix(path, item.Path+"/") || strings.HasPrefix(item.Path, path+"/") {
					delete(m.restoreBasket, path)
				}
			}
			m.restoreBasket[item.Path] = item
			m.pendingBasket = nil
			m.restoreStage = "basket"
		}
		if key == "n" || key == "esc" {
			m.pendingBasket = nil
			m.restoreStage = "browse"
		}
		return m, nil, true
	case "basket":
		items := m.basketItems()
		m.basketIndex = moveList(m.basketIndex, len(items), m.restorePageSize(), key)
		if (key == " " || key == "delete") && len(items) > 0 {
			delete(m.restoreBasket, items[m.basketIndex].Path)
			m.basketIndex = max(0, min(m.basketIndex, len(items)-2))
		}
		if key == "esc" {
			m.restoreStage = "browse"
			m.restoreSearching = false
			m.filterRestore()
		}
		if (key == "enter" || key == "e" || key == "r") && len(m.restoreBasket) > 0 {
			m.restoreStage = "transaction-target"
			m.restoreTargetMode = "original"
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) restoreBasketView() string {
	if m.restoreStage == "basket-conflict" {
		return "SELECTION OVERLAP\n\nThe new version overlaps paths already selected from a different point.\nReplace those overlapping selections? [y/N]"
	}
	items := m.basketItems()
	body := fmt.Sprintf("RESTORE SELECTED — %d paths\nEnter: Choose destination\n", len(items))
	start, end := restoreWindow(m.basketIndex, len(items), m.restorePageSize())
	for i := start; i < end; i++ {
		v := items[i]
		line := fmt.Sprintf("%s  %s  %s", v.Path, v.Time.Local().Format("2006-01-02 15:04"), m.restoreMachineName(v.MachineID))
		body += m.restoreRow(line, i == m.basketIndex) + "\n"
	}
	return body + "\nSpace remove • Enter destination • Esc browse"
}
