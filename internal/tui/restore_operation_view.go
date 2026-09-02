package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) restoreTransactionPreview() string {
	var files, bytes uint64
	cross, ownership := false, false
	for _, report := range m.restorePreflights {
		files += uint64(report.Files)
		bytes += report.BytesRequired
		cross = cross || report.CrossFilesystem
		ownership = ownership || report.OwnershipMappingNeeded
	}
	body := fmt.Sprintf("SELECTIVE RESTORE PREFLIGHT\n\nPaths          %d\nFiles          %d\nData           %s\nDestination    %s\nConflict       %s\nCross-device   %t\nOwnership map  %t\n\n",
		len(m.restoreBasket), files, bytesText(bytes), m.restoreTargetMode, m.restoreConflict, cross, ownership)
	for _, plan := range m.restorePlans {
		for _, item := range plan.Items {
			body += fmt.Sprintf("%-8s %s  →  %s\n", shortID(plan.Snapshot), item.SourcePath, item.TargetPath)
		}
	}
	if m.restoreBundleMode == "exact" {
		body += fmt.Sprintf("\nEXACT REWIND DELETIONS  %d\n", len(m.restoreBundleDeletes))
		limit := max(4, m.height-19-len(m.restorePlans))
		start := max(0, min(m.restoreIndex, max(0, len(m.restoreBundleDeletes)-limit)))
		for index := start; index < len(m.restoreBundleDeletes) && index < start+limit; index++ {
			prefix := "  "
			if index == m.restoreIndex {
				prefix = "> "
			}
			body += prefix + m.restoreBundleDeletes[index] + "\n"
		}
		body += fmt.Sprintf("Showing %d–%d of %d; every deletion is available with ↑/↓.\n", min(len(m.restoreBundleDeletes), start+1), min(len(m.restoreBundleDeletes), start+limit), len(m.restoreBundleDeletes))
	}
	return body + "\n" + m.styles.help.Render("↑/↓ affected paths • c conflict policy • enter confirm • esc cancel")
}

func (m Model) restoreApplicationPreview() string {
	p := m.restoreAppPlan
	body := fmt.Sprintf("APPLICATION TIME RECONCILIATION\n\nSource identity %s\nRestore Point   %s\nTimestamp       %s\n\nInstall         %d\nSubstitutions   %d\nUnavailable     %d\nConflicts       %d\nUnchanged       %d\nInstalled later %d\nServices        %d system / %d user\n\n",
		p.MachineID, shortID(p.Snapshot), p.Timestamp.Local().Format("2006-01-02 15:04"), len(p.Install), len(p.Substitutions), len(p.Unavailable), len(p.Conflicts),
		len(p.Unchanged), len(p.InstalledLater), len(p.SystemServices), len(p.UserServices))
	for _, pkg := range p.Substitutions {
		body += fmt.Sprintf("SUBSTITUTE  %s  %s → %s\n", pkg.Name, pkg.WantedVersion, pkg.AvailableVersion)
	}
	for _, pkg := range p.Unavailable {
		body += "UNAVAILABLE " + pkg.Name + "\n"
	}
	for _, pkg := range p.Conflicts {
		body += "CONFLICT    " + pkg.Name + "\n"
	}
	return body + "\nNo application will be removed. Installed-later items produce commands for review only.\n\n" + m.styles.help.Render("enter approve • esc cancel")
}

func (m Model) restoreTransactionProgressView() string {
	p := m.restoreTransaction
	percent := 0
	if p.BytesTotal > 0 {
		percent = int(p.BytesDone * 100 / p.BytesTotal)
	} else if p.FilesTotal > 0 {
		percent = int(p.FilesDone * 100 / p.FilesTotal)
	}
	percent = min(percent, 100)
	filled := percent * 28 / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 28-filled)
	return fmt.Sprintf("RESTORE TRANSACTION / %s\n\n[%s] %d%%\n%d / %d files  •  %s / %s  •  %s/s\nElapsed %s  •  ETA %s\n\n%s",
		strings.ToUpper(p.Phase), bar, percent, p.FilesDone, p.FilesTotal, bytesText(p.BytesDone), bytesText(p.BytesTotal), bytesText(uint64(p.BytesPerSecond)),
		p.Elapsed.Round(time.Second), p.EstimatedRemain.Round(time.Second), m.styles.help.Render("Ctrl+C cancels safely; encrypted journal remains resumable"))
}

func (m Model) restoreTransactionResultView() string {
	body := fmt.Sprintf("RESTORE TRANSACTION\n\nPlaced       %d\nRollback     %d\nSkipped      %d\nStaging      %s\nJournal      %s",
		len(m.restoreResult.Placed), len(m.restoreResult.Rollback), len(m.restoreResult.Skipped), m.restoreResult.StagedAt, m.restoreResult.JournalPath)
	if m.restoreResult.BrowserRepair.Removed+m.restoreResult.BrowserRepair.Live+m.restoreResult.BrowserRepair.Failed > 0 {
		body += fmt.Sprintf("\nBrowser      %d removed / %d live / %d failed", m.restoreResult.BrowserRepair.Removed, m.restoreResult.BrowserRepair.Live, m.restoreResult.BrowserRepair.Failed)
	}
	if m.restoreBundleJournal != "" {
		body += "\nComposition  " + m.restoreBundleJournal
	}
	if m.err != "" {
		body += "\n\n" + m.styles.status.Render(m.err) + "\n\n" + m.styles.help.Render("r resume • u rollback placed paths • esc return")
	} else if len(m.restoreResult.Rollback) > 0 {
		body += "\n\n" + m.styles.help.Render("u rollback • enter return")
	} else {
		body += "\n\n" + m.styles.help.Render("enter return")
	}
	return body
}

func (m Model) restoreApplicationProgressView() string {
	p := m.restoreAppProgress
	resolved := p.Completed + p.Failed
	percent := 0
	if p.Total > 0 {
		percent = resolved * 100 / p.Total
	}
	filled := percent * 28 / 100
	return fmt.Sprintf("APPLICATIONS / %s\n\n%s\n[%s%s] %d%%\n%d / %d resolved  •  %d failures\n\n%s",
		strings.ToUpper(p.Lane), p.Current, strings.Repeat("█", filled), strings.Repeat("░", 28-filled), percent, resolved, p.Total, p.Failed,
		m.styles.help.Render("Filesystem transactions remain independent; Ctrl+C cancels this lane"))
}

func (m Model) restoreApplicationResultView() string {
	r := m.restoreAppResult
	body := fmt.Sprintf("APPLICATION RECONCILIATION RESULT\n\nInstalled       %d\nSubstituted     %d\nUnavailable     %d\nConflicts       %d\nFailures        %d\nMissing units   %d\nRemoval review  %d\nJournal         %s\n",
		len(r.Installed), len(r.Substituted), len(r.Unavailable), len(r.Conflicts), len(r.Failures), len(r.MissingServices), len(r.RemovalCommands), m.restoreAppJournal)
	for _, value := range r.Substituted {
		body += "SUBSTITUTED  " + value + "\n"
	}
	for _, value := range r.Failures {
		body += "FAILED       " + value + "\n"
	}
	for _, value := range r.RemovalCommands {
		body += "REVIEW ONLY  " + value + "\n"
	}
	return body + "\n" + m.styles.help.Render("enter dashboard")
}
