package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) restoreTransactionPreview() string {
	var files, bytes uint64
	for _, report := range m.restorePreflights {
		files += uint64(report.Files)
		bytes += report.BytesRequired
	}
	body := fmt.Sprintf("RESTORE PREFLIGHT\n%d selected paths • %d files • %s\nDestination: %s • Conflict: %s\n\n", len(m.restoreBasket), files, bytesText(bytes), m.restoreTargetMode, m.restoreConflict)
	rows := m.restorePreviewRows()
	start, end := restoreWindow(m.restoreIndex, len(rows), max(1, m.restorePageSize()-2))
	for i := start; i < end; i++ {
		body += m.restoreRow(rows[i], i == m.restoreIndex) + "\n"
	}
	return body + "\n" + m.restoreRow(fmt.Sprintf("%d affected paths • ↑/↓ scroll • c conflicts • Enter confirm • Esc back", len(rows)), false)
}

func (m Model) restorePreviewRows() []string {
	var rows []string
	for _, plan := range m.restorePlans {
		for _, item := range plan.Items {
			date := ""
			for _, point := range m.snapshots {
				if point.ID == plan.Snapshot {
					date = point.Time.Local().Format("2006-01-02 15:04")
					break
				}
			}
			rows = append(rows, fmt.Sprintf("PUT %s  %s → %s", date, item.SourcePath, item.TargetPath))
		}
	}
	if m.restoreBundleMode == "exact" {
		for _, path := range m.restoreBundleDeletes {
			rows = append(rows, "REMOVE "+path)
		}
	}
	return rows
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
