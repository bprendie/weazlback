package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/lipgloss"
)

var restoreSelection = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true)

func (m Model) restoreScreen() string {
	body := m.styles.header.Render("RESTORE MODE") + "\n\n"
	switch m.restoreStage {
	case "", "dashboard":
		return body + m.restoreDashboard()
	case "full":
		return body + m.styles.header.Render("FRESH SYSTEM RECOVERY") +
			"\n\nAttach prepared recovery media, then press enter to launch the guided interface.\n\n" +
			m.styles.status.Render("weazlback-restore") + "\n\n" + m.styles.help.Render("enter launch • esc dashboard")
	case "versions":
		return body + m.restoreVersionsView()
	case "content-confirm":
		return body + m.styles.header.Render("UNLOCK CONTENT SEARCH") +
			"\n\nThis decrypts bounded text files from the selected Restore Point for this search only.\nNo content index or plaintext result cache is retained.\n\nQuery  " +
			m.restoreContentQuery + "\nPath   " + m.restoreTreePath + "\n\n" + restoreSelection.Render("Continue? [y/N]")
	case "content-running":
		return body + m.styles.status.Render("◉ "+m.status+"…")
	case "transaction-target":
		return body + m.styles.header.Render("SELECTIVE RESTORE DESTINATION") +
			fmt.Sprintf("\n\n%d paths are pinned to their selected machine and Restore Point.\n\n", len(m.restoreBasket)) +
			"[o / enter] Original mapped paths\n[a] Alternate directory\n[s] Private staging only\n\n" + m.styles.help.Render("esc return without changes")
	case "transaction-alternate", "transaction-confirm":
		return body + m.restoreInput.View() + "\n\n" + m.styles.help.Render("enter continue • esc back")
	case "transaction-preview":
		return body + m.restoreTransactionPreview()
	case "transaction-running":
		return body + m.restoreTransactionProgressView()
	case "transaction-result":
		return body + m.restoreTransactionResultView()
	case "bundle-components":
		return body + m.styles.header.Render("POINT-IN-TIME BUNDLES") + "\n\n" + m.bundleSummary() + "\n\n" +
			"1 " + bundleChoiceLabel(m.restoreBundleChoices, restoretxn.SystemConfig) + " System Config — dotfiles, widgets, Weazl settings\n" +
			"2 " + bundleChoiceLabel(m.restoreBundleChoices, restoretxn.PersonalFiles) + " Personal Files — normal Home data\n" +
			"3 " + bundleChoiceLabel(m.restoreBundleChoices, restoretxn.HeavyData) + " VMs / Containers — Heavy lane\n" +
			"4 [ ] Everything above — Applications remain separate\n\n" + m.styles.help.Render("1/2/3 toggle • 4 all • [/] time • enter continue • esc dashboard")
	case "bundle-mode":
		return body + m.styles.header.Render("BUNDLE RESTORE MODE") + "\n\n[o / enter] Safe Overlay\n    Replace conflicts after preserving rollback copies; never delete unrelated files.\n\n[x] Exact Rewind\n    Restore selected boundaries exactly and remove paths proven absent at that point."
	case "bundle-safety":
		return body + m.styles.header.Render("EXACT REWIND / SAFETY BACKUP") + "\n\n" +
			m.styles.status.Render("This is destructive and could result in missing or corrupt files.") +
			fmt.Sprintf("\n\n%d paths are proven deletions inside the selected boundaries.\n\nCreate a quick current-state backup first? [y/N]", len(m.restoreBundleDeletes))
	case "bundle-understand":
		return body + m.styles.status.Render("This is destructive and could result in missing or corrupt files.") + "\n\n" + m.restoreInput.View()
	case "bundle-final":
		return body + m.styles.header.Render("FINAL BUNDLE CONFIRMATION") + "\n\n" + m.restoreBundleApprovalView() + "\n\nProceed with this operation? [y/N]"
	case "applications-loading":
		return body + m.styles.status.Render("◉ loading exact application manifest and current availability…")
	case "applications-preview":
		return body + m.restoreApplicationPreview()
	case "applications-authorizing":
		return body + m.styles.status.Render("Authorizing one bounded sudo session for application installation…")
	case "applications-running":
		return body + m.restoreApplicationProgressView()
	case "applications-result":
		return body + m.restoreApplicationResultView()
	}
	if m.busy {
		return body + m.styles.status.Render("◉ "+m.status+"…")
	}
	return body + m.restoreBrowserView()
}

func (m Model) restoreDashboard() string {
	identity := m.cfg.Machine.Name
	if identity == "" {
		identity = m.cfg.Machine.Hostname
	}
	return "SOURCE  " + m.styles.status.Render(identity) + "\n\n" +
		restoreSelection.Render("f / enter  Browse history") + "\n" +
		"           Timeline and filesystem tree\n\n" +
		"/          Search filenames across time\n" +
		"/p PATH    Open or search inside a repository path\n\n" +
		"/c TEXT    Search contents in the selected point/path\n\n" +
		"d          Deleted since then\n\n" +
		"g          Bundle restore\n" +
		"a          Reconcile applications\n" +
		"j          Active / recent restores\n\n" +
		"x          Fresh-system recovery\n\n" +
		m.styles.help.Render("B backup mode • restore execution arrives in R4")
}

func (m Model) restoreBrowserView() string {
	var body strings.Builder
	body.WriteString(m.restoreContextLine() + "\n\n")
	if m.restoreSearching {
		body.WriteString(m.restoreInput.View() + "\n\n")
		if len(m.restoreResults) > 0 {
			body.WriteString(m.catalogResultsView())
		} else {
			body.WriteString(m.fileRows(m.restoreVisible))
		}
	} else {
		body.WriteString(m.styles.help.Render("PATH  ") + m.restoreTreePath + "\n\n")
		body.WriteString(m.fileRows(m.restoreVisible))
	}
	if len(m.restoreVisible) == 0 && len(m.restoreResults) == 0 {
		body.WriteString("No matching paths.\n")
	}
	if len(m.restoreLiveHints) > 0 {
		body.WriteString("\nLIVE PATH HINTS\n")
		for _, hint := range m.restoreLiveHints {
			body.WriteString("  " + hint + "\n")
		}
	}
	body.WriteString("\n" + m.restoreMetadataPreview())
	if m.restoreCatalogState != "" {
		body.WriteString("\n" + m.styles.help.Render(m.restoreCatalogState))
	}
	body.WriteString("\n" + m.styles.help.Render("/ search • p PATH • H path history • A all machines • ←/→ tree • space basket • e execute • [/] time • i identity • esc"))
	return body.String()
}

func (m Model) restoreBundleApprovalView() string {
	body := fmt.Sprintf("Mode            %s\nRequested time  %s\nSafety backup   %t\n\nCOMPONENT SOURCES\n",
		m.restoreBundleMode, m.restoreBundleTime.Local().Format("2006-01-02 15:04"), m.restoreSafetyBackup)
	for _, component := range m.restoreBundleParts {
		body += fmt.Sprintf("%-16s %-18s %s  %s\n", component.Bundle, component.MachineID, component.Snapshot.ShortID,
			component.Snapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	return body
}

func (m Model) restoreMetadataPreview() string {
	if len(m.restoreResults) > 0 && m.restoreIndex < len(m.restoreResults) {
		record := m.restoreResults[m.restoreIndex]
		if len(record.Versions) > 0 {
			v := record.Versions[0]
			return fmt.Sprintf("METADATA  %s  %s  mode %04o  uid:gid %d:%d  %s", v.Type, bytesText(v.Size), v.Mode&0o7777, v.UID, v.GID, v.Time.Local().Format("2006-01-02 15:04"))
		}
	}
	if len(m.restoreVisible) > 0 && m.restoreIndex < len(m.restoreVisible) {
		e := m.restoreVisible[m.restoreIndex]
		return fmt.Sprintf("METADATA  %s  %s  mode %04o  uid:gid %d:%d  %s", e.Type, bytesText(e.Size), e.Mode&0o7777, e.UID, e.GID, e.ModTime.Local().Format("2006-01-02 15:04"))
	}
	return ""
}

func (m Model) restoreContextLine() string {
	identity := "unknown machine"
	if m.restoreAllMachines {
		identity = "ALL MACHINES"
	} else if len(m.restoreIdentities) > 0 && m.restoreIdentity < len(m.restoreIdentities) {
		identity = m.restoreIdentities[m.restoreIdentity].Name
	}
	point := "no Restore Point"
	if len(m.snapshots) > 0 && m.restoreSnapshot < len(m.snapshots) {
		snapshot := m.snapshots[m.restoreSnapshot]
		point = snapshot.Time.Local().Format("Mon Jan 02 15:04") + "  " + snapshot.ShortID + "  " + restorePointHealth(snapshot.Tags)
	}
	return "SOURCE  " + restoreSelection.Render(identity) + "    POINT  " + point + fmt.Sprintf("    BASKET  %d", len(m.restoreBasket))
}

func restorePointHealth(tags []string) string {
	return strings.ToUpper(restic.SnapshotHealth(tags))
}

func (m Model) fileRows(entries []restic.FileEntry) string {
	var body strings.Builder
	limit := max(8, m.height-15)
	start := 0
	if m.restoreIndex >= limit {
		start = m.restoreIndex - limit + 1
	}
	for i := start; i < len(entries) && i < start+limit; i++ {
		entry := entries[i]
		glyph := "·"
		if entry.Type == "dir" {
			glyph = "▸"
		}
		checked := " "
		if _, selected := m.restoreBasket[entry.Path]; selected {
			checked = "✓"
		}
		line := fmt.Sprintf("%s %s %-4s %9s  %s", checked, glyph, entry.Type, bytesText(entry.Size), filepath.Base(entry.Path))
		if i == m.restoreIndex {
			line = restoreSelection.Render("> " + line)
		} else {
			line = "  " + line
		}
		body.WriteString(line + "\n")
	}
	return body.String()
}

func (m Model) catalogResultsView() string {
	var body strings.Builder
	limit := max(8, m.height-15)
	for i, result := range m.restoreResults {
		if i >= limit {
			break
		}
		line := result.Path + "  " + versionDates(result)
		if deletedLatest(result) {
			line = lipgloss.NewStyle().Foreground(warning).Render(line + "  DELETED")
		}
		if i == m.restoreIndex {
			line = restoreSelection.Render("> " + line)
		} else {
			line = "  " + line
		}
		body.WriteString(line + "\n")
	}
	return body.String()
}

func versionDates(record catalog.PathRecord) string {
	var dates []string
	for i, version := range record.Versions {
		if i == 4 {
			dates = append(dates, fmt.Sprintf("+%d", len(record.Versions)-i))
			break
		}
		dates = append(dates, version.Time.Local().Format("Jan 02 15:04"))
	}
	return strings.Join(dates, " · ")
}

func deletedLatest(record catalog.PathRecord) bool {
	return len(record.Versions) > 0 && strings.Contains(record.Versions[0].Change, "-")
}

func (m Model) restoreVersionsView() string {
	record := m.restoreCatalog.Paths[filepath.Clean(m.restoreVersionPath)]
	body := m.styles.header.Render("FILE VERSIONS") + "\n\n" + m.restoreVersionPath + "\n\n"
	if record == nil || len(record.Versions) == 0 {
		return body + "Only the selected Restore Point is currently catalogued.\n\n" + m.styles.help.Render("← / esc return")
	}
	for _, version := range record.Versions {
		state := version.Change
		if state == "+" {
			state = "present"
		}
		line := fmt.Sprintf("%-17s  %-8s  %s  %s", version.Time.Local().Format("2006-01-02 15:04"), state, version.Profile, shortID(version.SnapshotID))
		if strings.Contains(version.Change, "-") {
			line = lipgloss.NewStyle().Foreground(warning).Render(line)
		}
		body += line + "\n"
	}
	return body + "\n" + m.styles.help.Render("← / esc return • restoration arrives in R4")
}

func shortID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
