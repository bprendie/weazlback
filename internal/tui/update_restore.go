package tui

import (
	"fmt"
	"time"

	"github.com/bprendie/weazlback/internal/restoretxn"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateRestoreMessage(message tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := message.(type) {
	case restoreLoadAuthMsg:
		if msg.err != nil {
			m.err = "System authorization cancelled; r retries, Esc returns"
			m.restoreStage = "load-error"
			return m, nil, true
		}
		m.restoreAuthorized = true
		if msg.point {
			next, cmd := m.loadRestorePoint(msg.index)
			return next, cmd, true
		}
		next, cmd := m.startOrRunRestore()
		return next, cmd, true
	case restorePreflightAuthMsg:
		if msg.err != nil {
			m.err = "System authorization cancelled; retry or go back"
			return m, nil, true
		}
		m.restoreAuthorized = true
		next, cmd := m.startRestoreTransactionPreflight(msg.alternate)
		return next, cmd, true
	case restoreSearchMsg:
		if msg.request != m.searchRequest || !m.restoreSearching || m.restoreStage != "browse" {
			return m, nil, true
		}
		m.searchBusy = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil, true
		}
		m.restoreResults, m.restoreVisible, m.searchCount = msg.results, msg.files, msg.count
		m.status, m.err = fmt.Sprintf("%d matching paths", msg.count), ""
		m.restoreIndex = min(m.restoreIndex, max(0, max(len(msg.results), len(msg.files))-1))
		return m, nil, true
	case restoreApplicationPlanMsg:
		if msg.err != nil {
			m.err, m.status, m.restoreStage = msg.err.Error(), "application plan failed", "dashboard"
		} else {
			m.restoreAppPlan, m.restoreStage, m.status, m.err = msg.plan, "applications-preview", "application reconciliation plan ready", ""
		}
		return m, nil, true
	case restoreApplicationSudoMsg:
		if msg.err != nil {
			m.err, m.status, m.restoreStage = msg.err.Error(), "sudo authorization failed", "applications-preview"
			return m, nil, true
		}
		m.restoreStage = "applications-running"
		model, cmd := m.runApplicationReconciliation()
		return model, cmd, true
	case restoreApplicationProgressMsg:
		m.restoreAppProgress, m.status = msg.progress, "applications: "+msg.progress.Current
		return m, waitApplicationEvent(msg.events), true
	case restoreApplicationDoneMsg:
		m.busy, m.restoreAppResult, m.restoreAppJournal, m.restoreStage = false, msg.result, msg.journal, "applications-result"
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "application journal failed"
		} else {
			m.err, m.status = "", "application reconciliation complete"
		}
		return m, nil, true
	case bundleSafetyMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.status, m.restoreStage = msg.err.Error(), "quick safety backup failed; Exact Rewind remains blocked", "bundle-safety"
		} else {
			m.restoreSafetyBackup, m.restoreStage, m.status, m.err = true, "bundle-final", "quick safety backup complete", ""
		}
		return m, nil, true
	case bundlePreparedMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.restoreStage = msg.err.Error(), "bundle-components"
			return m, nil, true
		}
		m.restoreBundleParts, m.restoreBasket, m.restoreScopeDecision = msg.components, msg.basket, msg.decision
		m.restoreTargetMode, m.restoreConflict = "original", restoretxn.OverlayPreserving
		if msg.decision.PlatformMismatch {
			m.restoreStage, m.status = "bundle-compatibility-warning", "Core withheld for cross-platform restore"
			return m, nil, true
		}
		model, cmd := m.startRestoreTransactionPreflight("")
		return model, cmd, true
	case restoreTransactionPreflightMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.status, m.restoreStage = msg.err.Error(), "restore preflight failed", "browse"
			return m, nil, true
		}
		m.restorePlans, m.restorePreflights, m.restoreBundleDeletes, m.restoreStage = msg.plans, msg.reports, msg.deletes, "transaction-preview"
		m.restoreIndex, m.status, m.err = 0, "restore transaction preflight passed", ""
		return m, nil, true
	case restoreTransactionProgressMsg:
		m.restoreTransaction, m.status = msg.progress, msg.progress.Phase+" in progress"
		return m, waitRestoreTransaction(msg.events), true
	case restoreTransactionDoneMsg:
		m.busy, m.cancel, m.restoreResult = false, nil, msg.result
		if msg.err != nil {
			m.err, m.status, m.restoreStage = msg.err.Error(), "restore transaction interrupted — encrypted journal is resumable", "transaction-result"
		} else {
			m.err, m.status, m.restoreStage = "", "restore transaction complete", "transaction-result"
			m.restoreBasket = map[string]restoreBasketItem{}
		}
		return m, nil, true
	case restoreRollbackDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "rollback incomplete"
		} else {
			m.err, m.status, m.restoreStage = "", "restore transaction rolled back", "browse"
		}
		return m, nil, true
	case restoreLiveHintsMsg:
		m.restoreLiveHints = msg.hints
		if msg.err != nil {
			m.restoreCatalogState = "live plocate hints unavailable; repository search remains authoritative"
		} else if len(msg.hints) > 0 {
			m.restoreCatalogState = "live plocate hints shown below; verified repository history remains authoritative"
		}
		return m, nil, true
	case restoreCatalogMsg:
		if msg.request != 0 && msg.request != m.catalogRequest {
			return m, nil, true
		}
		if msg.err != nil {
			m.restoreCatalogState = "History unavailable • current point searchable • Ctrl+R refresh"
		} else {
			m.restoreCatalog = msg.catalog
			m.restoreCatalogState = fmt.Sprintf("History loaded in %s • Ctrl+R refresh selected scope", msg.elapsed.Round(time.Millisecond))
			if m.restoreDeletedOnly && len(m.snapshots) > 0 && m.restoreSnapshot < len(m.snapshots) {
				m.restoreResults = deletedCatalogResults(m.restoreCatalog, m.restoreIdentities, m.restoreIdentity, m.snapshots[m.restoreSnapshot].Time)
				m.restoreVisible = nil
			} else if m.restoreSearching && m.restoreStage == "browse" {
				next, cmd := m.queueRestoreSearch()
				return next, cmd, true
			} else if m.restoreStage == "browse" {
				m.filterRestore()
			}
		}
		return m, nil, true
	case contentSearchMsg:
		m.busy, m.restoreStage, m.restoreSearching = false, "content-results", false
		m.restoreVisible, m.restoreResults, m.restoreIndex = msg.files, nil, 0
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "content search failed"
		} else {
			m.err, m.status = "", fmt.Sprintf("content search found %d files; no contents retained", len(msg.files))
		}
		return m, nil, true
	}
	return m, nil, false
}
