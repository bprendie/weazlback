package tui

import (
	"strings"

	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateRestoreWorkspaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if model, cmd, handled := m.restoreSessionKey(key); handled && (!m.restoreTextFocused() || key == "ctrl+c") {
		return model, cmd
	}
	if model, cmd, handled := m.restoreNavigationKey(msg); handled {
		return model, cmd
	}
	if m.restoreStage == "browse" && m.restoreSearching {
		return m.updateRestoreSearchKey(msg)
	}
	if m.restoreStage == "bundle-components" {
		return m.updateBundleComponentsKey(key)
	}
	if m.restoreStage == "bundle-mode" {
		if key == "o" || key == "enter" {
			m.restoreBundleMode, m.restoreConflict = "overlay", restoretxn.OverlayPreserving
			return m.prepareBundleTransaction()
		}
		if key == "x" {
			m.restoreBundleMode, m.restoreConflict, m.restoreSafetyBackup = "exact", restoretxn.OverlayPreserving, false
			return m.prepareBundleTransaction()
		}
		if key == "esc" {
			m.restoreStage = "bundle-components"
		}
		return m, nil
	}
	if m.restoreStage == "bundle-compatibility-warning" {
		if key == "enter" {
			if len(m.restoreBasket) == 0 {
				m.restoreStage, m.status = "dashboard", "Core withheld; no compatible payload remains"
				return m, nil
			}
			return m.startRestoreTransactionPreflight("")
		}
		if key == "esc" {
			m.restoreStage = "bundle-components"
		}
		return m, nil
	}
	if m.restoreStage == "bundle-safety" {
		if key == "y" {
			return m.runBundleSafetyBackup()
		}
		if key == "n" {
			m.restoreStage = "bundle-understand"
			m.restoreInput = textinput.New()
			m.restoreInput.Prompt = "I understand — type YES > "
			m.restoreInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}
	if m.restoreStage == "bundle-understand" {
		if key == "enter" {
			if m.restoreInput.Value() != "YES" {
				m.err = "confirmation must be exactly YES"
				return m, nil
			}
			m.restoreStage = "bundle-final"
			return m, nil
		}
		var cmd tea.Cmd
		m.restoreInput, cmd = m.restoreInput.Update(msg)
		return m, cmd
	}
	if m.restoreStage == "bundle-final" {
		if key == "y" || key == "enter" {
			m.restoreStage = "transaction-running"
			return m.runRestoreTransaction()
		}
		if key == "n" || key == "esc" {
			m.restoreStage = "transaction-preview"
		}
		return m, nil
	}
	if m.restoreStage == "applications-preview" {
		if key == "enter" || key == "y" {
			m.restoreStage, m.status = "applications-authorizing", "requesting vault-bounded sudo authorization"
			return m, authorizeRestoreApplications()
		}
		if key == "esc" || key == "n" {
			m.restoreStage = "dashboard"
		}
		return m, nil
	}
	if m.restoreStage == "applications-result" {
		if key == "enter" || key == "esc" {
			m.restoreStage = "dashboard"
		}
		return m, nil
	}
	if m.restoreStage == "transaction-target" {
		switch key {
		case "o", "enter":
			m.restoreTargetMode = "original"
			return m.startRestoreTransactionPreflight("")
		case "s":
			m.restoreTargetMode = "staging"
			return m.startRestoreTransactionPreflight("")
		case "a":
			m.restoreTargetMode, m.restoreStage = "alternate", "transaction-alternate"
			m.restoreInput = textinput.New()
			m.restoreInput.Prompt = "destination directory > "
			m.restoreInput.Focus()
			return m, textinput.Blink
		case "esc":
			m.restoreStage = "browse"
		}
		return m, nil
	}
	if m.restoreStage == "transaction-alternate" {
		if key == "enter" && strings.TrimSpace(m.restoreInput.Value()) != "" {
			return m.startRestoreTransactionPreflight(strings.TrimSpace(m.restoreInput.Value()))
		}
		if key == "esc" {
			m.restoreStage = "transaction-target"
			return m, nil
		}
		var cmd tea.Cmd
		m.restoreInput, cmd = m.restoreInput.Update(msg)
		return m, cmd
	}
	if m.restoreStage == "transaction-preview" {
		if key == "up" || key == "down" || key == "k" || key == "j" || key == "pgup" || key == "pgdown" || key == "home" || key == "end" {
			m.restoreIndex = moveList(m.restoreIndex, len(m.restorePreviewRows()), max(1, m.restorePageSize()-2), key)
			return m, nil
		}
		if m.restoreBundleMode == "exact" && (key == "up" || key == "k") {
			m.restoreIndex = max(0, m.restoreIndex-1)
			return m, nil
		}
		if m.restoreBundleMode == "exact" && (key == "down" || key == "j") {
			m.restoreIndex = min(max(0, len(m.restoreBundleDeletes)-1), m.restoreIndex+1)
			return m, nil
		}
		if key == "c" {
			if m.restoreBundleMode == "exact" {
				m.status = "Exact Rewind always replaces conflicts after preserving rollback copies"
				return m, nil
			}
			if m.restoreConflict == restoretxn.ReplacePreserving {
				m.restoreConflict = restoretxn.SkipExisting
			} else {
				m.restoreConflict = restoretxn.ReplacePreserving
			}
			for i := range m.restorePlans {
				m.restorePlans[i].Conflict = m.restoreConflict
			}
			return m, nil
		}
		if key == "enter" {
			if len(m.restoreBundleParts) > 0 {
				if m.restoreBundleMode == "exact" {
					m.restoreStage = "bundle-safety"
				} else {
					m.restoreStage = "bundle-final"
				}
				return m, nil
			}
			m.restoreStage = "transaction-confirm"
			m.restoreInput = textinput.New()
			m.restoreInput.Prompt = "type RESTORE > "
			m.restoreInput.Focus()
			return m, textinput.Blink
		}
		if key == "esc" {
			m.restoreStage = "browse"
		}
		return m, nil
	}
	if m.restoreStage == "transaction-confirm" {
		if key == "enter" {
			if m.restoreInput.Value() != "RESTORE" {
				m.err = "confirmation must be exactly RESTORE"
				return m, nil
			}
			m.restoreStage = "transaction-running"
			return m.runRestoreTransaction()
		}
		if key == "esc" {
			m.restoreStage = "transaction-preview"
			return m, nil
		}
		var cmd tea.Cmd
		m.restoreInput, cmd = m.restoreInput.Update(msg)
		return m, cmd
	}
	if m.restoreStage == "transaction-result" {
		if key == "r" && m.err != "" {
			m.restoreStage = "transaction-running"
			return m.runRestoreTransaction()
		}
		if key == "u" && len(m.restoreResult.Placed) > 0 {
			return m.rollbackRestoreTransaction()
		}
		if key == "enter" || key == "esc" {
			m.restoreStage = "browse"
		}
		return m, nil
	}
	if m.restoreStage == "dashboard" {
		if key == "b" || key == "B" {
			if m.busy {
				m.status = "Backup Mode unavailable while restore work is active"
				return m, nil
			}
			m.workspace, m.status = "backup", "Backup Mode"
			if m.catalogCancel != nil {
				m.catalogCancel()
			}
			m.catalogRequest++
			if m.searchCancel != nil {
				m.searchCancel()
			}
			m.searchRequest++
			m.mode, m.index, m.railFocused = m.backupMode, m.backupIndex, m.backupRailFocused
			return m, nil
		}
		if key == "g" || key == "a" {
			m.restoreIntent = map[string]string{"g": "bundle", "a": "applications"}[strings.ToLower(key)]
			m.restoreStage = "load"
			return m.startOrRunRestore()
		}
		if key == "j" {
			m.restoreStage, m.status = "transaction-result", "latest restore transaction"
			return m, nil
		}
		if key == "f" || key == "enter" || key == "/" || key == "d" {
			m.restoreStage = "load"
			m.restoreDeletedOnly = key == "d"
			if key != "d" {
				m.restoreDeletedOnly = false
			}
			model, cmd := m.startOrRunRestore()
			m = model.(Model)
			if key == "/" {
				m.restoreSearching = true
			}
			return m, cmd
		}
		if key == "x" {
			m.restoreStage, m.status = "full", "fresh-system recovery"
			return m, nil
		}
		return m, nil
	}
	if m.restoreStage == "content-confirm" {
		if key == "y" || key == "enter" {
			m.restoreContentOpen = true
			return m.startContentSearch()
		}
		if key == "n" || key == "esc" {
			m.restoreStage, m.restoreContentQuery = "browse", ""
		}
		return m, nil
	}
	if m.restoreStage == "content-results" {
		switch key {
		case "up", "k":
			m.restoreIndex = max(0, m.restoreIndex-1)
		case "down", "j":
			m.restoreIndex = min(max(0, len(m.restoreVisible)-1), m.restoreIndex+1)
		case "enter", "right":
			if len(m.restoreVisible) > 0 {
				m.openRestoreVersions(m.restoreVisible[m.restoreIndex].Path)
			}
		case "esc", "left":
			m.restoreStage, m.restoreContentQuery = "browse", ""
			m.filterRestore()
		}
		return m, nil
	}
	if m.restoreStage == "full" {
		return m.updateRestore(msg)
	}
	if m.restoreStage != "browse" {
		return m, nil
	}
	return m.updateRestoreBrowserKey(msg)
}
