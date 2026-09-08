package tui

import (
	"fmt"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"path/filepath"
)

func (m Model) restoreBrowseFinished(msg restoreBrowseMsg) (tea.Model, tea.Cmd) {
	if msg.request != 0 && msg.request != m.loadRequest {
		return m, nil
	}
	m.busy = false
	if msg.err != nil {
		m.restoreAuthorized = false
		m.err, m.status = msg.err.Error(), "restore-point index failed"
		m.restoreStage = "load-error"
		return m, nil
	}
	m.snapshots, m.restoreEntries, m.restoreSnapshot = msg.snapshots, msg.files, msg.index
	if msg.tree != nil {
		m.nodes, m.children, m.expanded, m.treeReady = msg.tree.nodes, msg.tree.children, msg.tree.expanded, true
	} else {
		m.buildRestoreTree()
	}
	if len(msg.identities) > 0 {
		m.restoreIdentities = msg.identities
		m.restoreIdentity = identityIndex(msg.identities, restic.IdentityID(msg.snapshots[msg.index]))
	}
	m.restoreInput, m.restoreStage, m.restoreIndex = newRestoreInput(), "browse", 0
	if m.restoreTreePath == "" {
		m.restoreTreePath = restoreRoot(msg.files)
	}
	for m.restoreTreePath != "/" {
		if _, ok := m.nodes[m.restoreTreePath]; ok {
			break
		}
		m.restoreTreePath = filepath.Dir(m.restoreTreePath)
	}
	m.filterRestore()
	if m.restoreSearching {
		m.restoreInput.Focus()
	}
	if m.restoreDeletedOnly {
		m.restoreResults = deletedCatalogResults(m.restoreCatalog, m.restoreIdentities, m.restoreIdentity, msg.snapshots[msg.index].Time)
		m.restoreVisible = nil
		m.restoreSearching = true
		m.restoreInput.Prompt = "deleted since then > "
	}
	point := msg.snapshots[msg.index]
	m.status, m.err = fmt.Sprintf("Browsing %d paths • %s • %s", len(msg.files), restic.Profile(point.Tags), point.Time.Local().Format("2006-01-02 15:04")), ""
	if len(msg.identities) > 0 && restic.Profile(point.Tags) != "home" {
		m.status = "No healthy Home point available; " + m.status
	}
	intent := m.restoreIntent
	m.restoreIntent = ""
	if intent == "bundle" {
		m.restoreBundleChoices = map[restoretxn.Bundle]bool{restoretxn.SystemConfig: true, restoretxn.PersonalFiles: true}
		m.restoreBundleTime, m.restoreBundleMode, m.restoreStage = msg.snapshots[msg.index].Time, "overlay", "bundle-components"
		m.status = "choose point-in-time file bundles"
	}
	if len(msg.identities) == 0 {
		return m, textinput.Blink
	}
	m.restoreCatalogState = "loading encrypted history catalog in background"
	commands := []tea.Cmd{textinput.Blink, m.loadRestoreCatalogCmd(msg.snapshots)}
	if intent == "applications" {
		m.restoreStage, m.status = "applications-loading", "loading timestamp-scoped application manifest"
		commands = append(commands, m.loadApplicationPlanCmd())
	}
	return m, tea.Batch(commands...)
}

func identityIndex(identities []restic.Identity, id string) int {
	for i := range identities {
		if identities[i].ID == id {
			return i
		}
	}
	return 0
}
