package recoverytui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	title := lipgloss.NewStyle().Foreground(pink).Bold(true).Render("WEAZLBACK / FRESH SYSTEM RECOVERY")
	width := max(36, m.width-4)
	if width > 92 {
		width = 92
	}
	help := lipgloss.NewStyle().Foreground(dim).Render("enter continue  •  ctrl+c quit")
	version := lipgloss.NewStyle().Foreground(dim).Render("recovery binary " + m.mediaVersion + " — shown before vault unlock")
	return panel.Width(width).Render(title + "\n" + version + "\n\n" + m.body() + "\n\n" + help)
}

func (m Model) body() string {
	switch m.stage {
	case "kit-choice":
		var lines []string
		for index, kit := range m.kits {
			cursor := "  "
			if index == m.kitIndex {
				cursor = "> "
			}
			lines = append(lines, cursor+kit)
		}
		return "RECOVERY KITS DISCOVERED\n\n" + strings.Join(lines, "\n") + "\n\n↑/↓ select • enter continue"
	case "kit", "pass", "override", "confirm", "hostname-custom":
		return m.input.View() + errorText(m.err)
	case "hostname-choice":
		return "HOSTNAME\n\n[o / enter] original hostname (best application compatibility)\n[c] current fresh-system hostname\n[n] enter a new hostname"
	case "engine-choice":
		return "RECOVERY ENGINE\n\n[s / enter] Standard — official Restic recovery authority\n[t] Turbo — authenticated embedded pack reader, bounded RAM, maximum local I/O\n\nTurbo aggressively consumes CPU, memory, source I/O, and target I/O. Do not use the machine during recovery. Any failed safety gate falls back to Standard."
	case "turbo-network":
		return "TURBO SSH POLICY\n\n[r / enter] Respect the configured transfer ceiling\n[f] Full link — remove the configured ceiling for this recovery\n\nFull link can saturate the network. This choice is recorded in the recovery journal."
	case "scope-choice":
		adoption := "off — preserve this machine's identity"
		if m.adoptIdentity {
			adoption = "ON — this replacement continues the source identity"
		}
		return "RECOVERY SCOPE\n\n[s] System Set — Exact complete Package + Core + Home + Heavy generation\n[c] Core — System personality\n[h / enter] Core + Home — Personality and personal data\n[e] Everything — Nearest compatible Restore Points\n\n[a] Replacement identity: " + adoption
	case "target-identity":
		return "TARGET MACHINE IDENTITY\n\n[c / enter] Keep this installation's identity\n[n] Generate a new independent machine identity\n[a] Replacement hardware — explicitly adopt the selected source identity\n\nHostname is selected separately on the next screen."
	case "action-choice":
		catalogState := "not built — selected-point browsing works immediately"
		if m.catalogPath != "" {
			catalogState = "encrypted catalog ready at " + m.catalogPath
		}
		return "RECOVERY WORKSPACE\n\n[s] System Set — Exact complete Package + Core + Home + Heavy generation\n[c] Core — System personality\n[h / enter] Core + Home — Personality and personal data\n[e] Everything — Nearest compatible Restore Points\n\nTOOLS\n[a] Applications only\n[f] Select one file or directory\n[i] Build optional encrypted history catalog\n\nSource identity  " + m.machineID + "\nTarget identity  " + targetIdentityText(m) + "\nCatalog          " + catalogState
	case "destination-loading":
		return "◉ Unlocking recovery kit and reading destinations…"
	case "identity-loading":
		return "◉ Reading encrypted machine histories…"
	case "selective-points":
		return "◉ Reading source Restore Points…"
	case "system-set-loading":
		return "◉ Reading complete atomic System Sets…"
	case "point-choice":
		return m.pointChoiceView()
	case "selective-loading":
		return "◉ Reading selected Restore Point filesystem…"
	case "catalog-loading":
		return "◉ Building an optional vault-encrypted history catalog…\n\nThis may take time. Selected-point browsing does not require it."
	case "selective":
		return m.selectiveView()
	case "selective-confirm":
		return "SELECTIVE RESTORE\n\n" + m.selectedPath + "\n→ original path mapped into this user's home\n\nThe live object is preserved for rollback before replacement.\n\n" + m.input.View() + errorText(m.err)
	case "selective-running":
		return selectiveProgressView(m.selectiveProgress)
	case "selective-done":
		return fmt.Sprintf("SELECTIVE RESTORE RESULT\n\nPlaced       %d\nRollback     %d\nStaging      %s\nJournal      %s", len(m.selectiveResult.Placed), len(m.selectiveResult.Rollback), m.selectiveResult.StagedAt, m.selectiveResult.JournalPath) + errorText(m.err)
	case "destination-choice":
		return m.destinationView()
	case "destination-warning":
		chosen := m.destinationSummaries[m.destination]
		newest := m.destinationSummaries[m.destinations[0].ID]
		return fmt.Sprintf("OLDER RECOVERY SOURCE SELECTED\n\nChosen       %s\nNewer source %s  %s\n\n[enter] Continue with older source\n[esc] Return", recoveryDestinationTime(chosen).Local().Format("2006-01-02 15:04"), m.destinations[0].Name, recoveryDestinationTime(newest).Local().Format("2006-01-02 15:04"))
	case "identity-choice":
		return m.identityView()
	case "loading":
		return "◉ Unlocking kit and verifying repository…"
	case "authorizing":
		return "Authorizing the visible restore transaction…"
	case "access":
		return "REPOSITORY ACCESS BLOCKED\n\n" + m.err + "\n\n[a] adopt exact local repository with sudo\n[o] choose a different local mount path\n[r] retry"
	case "plan":
		p := m.restore.Plan
		start := min(m.reviewIndex, max(0, len(m.review)-1))
		end := min(len(m.review), start+9)
		engine := "\nEngine         Standard"
		if m.engine == freshrestore.EngineTurbo {
			q := m.restore.Journal.Qualification
			engine = fmt.Sprintf("\nEngine         %s (eligible: %t)\nMemory budget  %.1f GiB\nSource         %s %s\nTarget         %s %s", strings.ToUpper(m.restore.Journal.Engine), q.Eligible, float64(q.Budget.MemoryBudget)/(1<<30), q.SourceTransport, q.SourceDevice, q.TargetFilesystem, q.TargetDevice)
			for _, finding := range append(q.HardFailures, q.SoftFindings...) {
				engine += "\nFinding        " + finding
			}
			if len(q.HardFailures) == 0 && len(q.SoftFindings) > 0 && !q.BreakGlassApplied {
				engine += "\n[b] Accept soft findings with break glass"
			}
		}
		return freshrestore.PlanText(p) + engine + fmt.Sprintf("\nManual review  %d items\n\nEXACT RESTORE QUEUE [%d/%d]\n%s\n\n↑/↓ review all • enter continue", len(m.review), start+1, len(m.review), strings.Join(m.review[start:end], "\n"))
	case "compatibility-warning":
		return "CROSS-PLATFORM RESTORE\n\n" + freshrestore.PlatformMismatchWarning + "\n\n[enter] Continue\n[esc] Cancel"
	case "running":
		return m.progressView()
	case "done":
		result := "Restore complete"
		if !m.report.Complete {
			result = "Restore incomplete — review exceptions"
		}
		capsule := fmt.Sprintf("%d installed / %d online fallback", len(m.report.CapsuleInstalled), len(m.report.CapsuleFallback))
		if m.report.CapsuleFallbackReason != "" {
			capsule += "\nFallback       " + m.report.CapsuleFallbackReason
		}
		return fmt.Sprintf("%s\nEngine         %s\nUsable         %s\nDurable        %s\nPlaced paths   %d\nCapsule local  %s\nBrowser locks  %d removed / %d live\nExceptions     %d\nJournal        %s", result, strings.ToUpper(m.report.Engine), m.report.Timing.TimeToUsable.Round(time.Second), m.report.Timing.TimeToDurable.Round(time.Second), len(m.report.RestoredPaths), capsule, m.report.BrowserRepair.Removed, m.report.BrowserRepair.Live, len(m.report.PackageErrors)+len(m.report.BrowserIssues), m.report.JournalPath) + errorText(m.err)
	default:
		return "Starting recovery…"
	}
}

func (m Model) pointChoiceView() string {
	var lines []string
	limit := max(5, m.height-12)
	start := max(0, min(m.pointIndex, max(0, len(m.points)-limit)))
	for index := start; index < len(m.points) && index < start+limit; index++ {
		point := m.points[index]
		cursor := "  "
		if index == m.pointIndex {
			cursor = "> "
		}
		marker := ""
		if index == 0 {
			marker = "  NEWEST"
		}
		if generation.Has(point.Tags, generation.TagComplete) {
			marker = "  COMPLETE SYSTEM SNAPSHOT"
		}
		lines = append(lines, fmt.Sprintf("%s%s  %-8s%s", cursor, point.Time.Local().Format("2006-01-02 15:04"), profile(point.Tags), marker))
	}
	if m.pointIntent == "system-set" {
		return "CHOOSE SYSTEM SET\n\n" + strings.Join(lines, "\n") + "\n\nEvery lane comes from this exact complete generation.\n↑/↓ select • enter recover set"
	}
	return "CHOOSE RESTORE POINT\n\n" + strings.Join(lines, "\n") + "\n\nHome and Heavy resolve to their nearest healthy points, disclosed in the final plan.\n↑/↓ select • enter continue"
}

func targetIdentityText(m Model) string {
	if m.adoptIdentity {
		return "ADOPT " + m.machineID
	}
	return m.targetIdentityMode + " " + m.targetMachineID
}

func (m Model) selectiveView() string {
	point := m.points[m.pointIndex]
	body := fmt.Sprintf("SELECTIVE RECOVERY / %s / %s\n\n%s\n\n", strings.ToUpper(profile(point.Tags)), point.Time.Local().Format("2006-01-02 15:04"), m.input.View())
	limit := max(5, m.height-14)
	start := max(0, min(m.fileIndex, max(0, len(m.visibleFiles)-limit)))
	for index := start; index < len(m.visibleFiles) && index < start+limit; index++ {
		file := m.visibleFiles[index]
		cursor, kind := "  ", "FILE"
		if index == m.fileIndex {
			cursor = "> "
		}
		if file.Type == "dir" {
			kind = "DIR "
		}
		body += fmt.Sprintf("%s%-4s %9s  %s\n", cursor, kind, compactBytes(file.Size), filepath.Clean(file.Path))
	}
	return body + fmt.Sprintf("\n%d matches • ↑/↓ select • [/] Restore Point • enter restore • esc workspace", len(m.visibleFiles))
}

func selectiveProgressView(p restoretxn.Progress) string {
	percent := 0
	if p.BytesTotal > 0 {
		percent = int(p.BytesDone * 100 / p.BytesTotal)
	} else if p.FilesTotal > 0 {
		percent = int(p.FilesDone * 100 / p.FilesTotal)
	}
	filled := min(100, percent) * 28 / 100
	return fmt.Sprintf("SELECTIVE RESTORE / %s\n\n[%s%s] %d%%\n%d / %d files • %s/s\n\nEncrypted resumable journal active",
		strings.ToUpper(p.Phase), strings.Repeat("█", filled), strings.Repeat("░", 28-filled), min(100, percent), p.FilesDone, p.FilesTotal, restoreRate(p.BytesPerSecond))
}

func compactBytes(value uint64) string {
	if value >= 1<<30 {
		return fmt.Sprintf("%.1fG", float64(value)/(1<<30))
	}
	if value >= 1<<20 {
		return fmt.Sprintf("%.1fM", float64(value)/(1<<20))
	}
	if value >= 1<<10 {
		return fmt.Sprintf("%.1fK", float64(value)/(1<<10))
	}
	return fmt.Sprint(value)
}

func profile(tags []string) string {
	for _, tag := range tags {
		if value, ok := strings.CutPrefix(tag, "profile:"); ok {
			return value
		}
	}
	return "point"
}

func (m Model) identityView() string {
	var lines []string
	for i, identity := range m.identities {
		cursor, legacy := "  ", ""
		if i == m.identityIndex {
			cursor = "> "
		}
		if identity.Legacy {
			legacy = "  LEGACY"
		}
		latest := "unknown"
		if !identity.Latest.IsZero() {
			latest = identity.Latest.Local().Format("2006-01-02 15:04")
		}
		lines = append(lines, fmt.Sprintf("%s%-22s %-18s %s  %4d points%s", cursor, truncate(identity.Name, 22), truncate(identity.Hostname, 18), latest, identity.Points, legacy))
	}
	return "CHOOSE SOURCE MACHINE\n\n" + strings.Join(lines, "\n") + "\n\n↑/↓ select  •  enter continue"
}

func (m Model) destinationView() string {
	var lines []string
	for i, destination := range m.destinations {
		cursor := "  "
		if i == m.destinationIndex {
			cursor = "> "
		}
		label := destination.Name
		if label == "" {
			label = destination.ID
		}
		summary := m.destinationSummaries[destination.ID]
		date, marker := "no healthy pair", ""
		if !summary.LatestComplete.IsZero() {
			date, marker = summary.LatestComplete.Local().Format("2006-01-02 15:04"), "  LATEST COMPLETE"
		}
		if summary.LatestComplete.IsZero() && !summary.LatestCore.IsZero() {
			date = "Core " + summary.LatestCore.Local().Format("2006-01-02 15:04")
		}
		if summary.Error != "" {
			date = "unavailable"
		}
		lines = append(lines, fmt.Sprintf("%s%-20s %-5s  %s%s", cursor, truncate(label, 20), strings.ToUpper(destination.Kind), date, marker))
	}
	return "CHOOSE BACKUP DESTINATION\n\n" + strings.Join(lines, "\n") + errorText(m.err) + "\n\nNewest recoverable destination is selected.\n↑/↓ select  •  enter continue"
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width < 2 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func errorText(value string) string {
	if value == "" {
		return ""
	}
	return "\n\n" + lipgloss.NewStyle().Foreground(pink).Render(value)
}
