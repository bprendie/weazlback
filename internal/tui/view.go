package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) header(width int) string {
	modeTitle := "BACKUP MODE"
	if m.workspace == "restore" {
		modeTitle = "RESTORE MODE"
	}
	compact := m.styles.header.Render("weazlback") + m.styles.help.Render(" / ") + m.styles.status.Render(modeTitle)
	if width < lipgloss.Width(logo) || m.height < 36 {
		return compact
	}
	return renderLogo(width) + "\n" + compact
}

func renderLogo(width int) string {
	lines := strings.Split(logo, "\n")
	colors := []lipgloss.Color{accent, secondary, success, warning}
	for i := range lines {
		lines[i] = lipgloss.NewStyle().Foreground(colors[i%len(colors)]).Render(lines[i])
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) content(width, height int) string {
	if m.helpVisible {
		return m.styles.focused.Width(width).Height(height).Render(m.helpScreen())
	}
	if m.workspace == "restore" {
		return m.styles.focused.Width(width).Height(height).Render(m.restoreScreen())
	}
	if width < 68 {
		if m.railFocused {
			return m.styles.focused.Width(width).Height(height).Render(m.sidebar(width))
		}
		return m.styles.focused.Width(width).Height(height).Render(m.screen())
	}
	sidebarWidth := min(28, max(18, width/4))
	mainWidth := max(24, width-sidebarWidth-2)
	railStyle, mainStyle := m.styles.panel, m.styles.focused
	if m.railFocused {
		railStyle, mainStyle = m.styles.focused, m.styles.panel
	}
	rail := railStyle.Width(sidebarWidth).Height(height).Render(m.sidebar(sidebarWidth))
	main := mainStyle.Width(mainWidth).Height(height).Render(m.screen())
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, main)
}

func (m Model) sidebar(width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(warning).Bold(true).Render("BACKUP"))
	b.WriteString("\n\n")
	for i, entry := range navigation {
		label := entry.label
		if width < 22 {
			label = compactNavigationLabel(entry.mode)
		}
		prefix := "  "
		style := m.styles.item
		if i == m.index {
			prefix, style = "> ", m.styles.selected
		}
		b.WriteString(prefix + m.styles.help.Render(entry.key) + " " + style.Render(label) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(warning).Bold(true).Render("REPOSITORY"))
	repository := "not initialized"
	if active := m.cfg.Active(); active != nil {
		repository = active.Name
	}
	b.WriteString("\n" + m.styles.help.Render(repository))
	if m.height < 30 {
		return b.String()
	}
	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(warning).Bold(true).Render("LAST RUN"))
	b.WriteString("\n" + m.styles.help.Render(m.status))
	return b.String()
}

func compactNavigationLabel(current mode) string {
	labels := map[mode]string{
		modeHome: "Home", modeBackup: "Backup", modeSnapshots: "Points",
		modeRestore: "Restore", modeProfiles: "Apps", modeDestinations: "Targets",
		modeRecovery: "Recovery", modeCheck: "Repo check", modeSchedule: "Schedule",
		modeTune: "Tune",
		modeNuke: "Nuke",
	}
	return labels[current]
}

func (m Model) screen() string {
	if m.helpVisible {
		return m.helpScreen()
	}
	if m.vaultStage != "" {
		warning := "Any non-empty passphrase is accepted. There is no reset, escrow, or recovery."
		body := m.styles.header.Render("PRIVATE VAULT") + "\n\n" + m.vaultInput.View() + "\n\n" + m.styles.help.Render(warning)
		if m.err != "" {
			body += "\n\n" + m.styles.status.Render(m.err)
		}
		return body
	}
	if m.mode == modeHome {
		destinations := "not initialized"
		hint := "Choose Destinations to initialize storage, then Backup now."
		if active := m.cfg.Active(); active != nil {
			destinations = active.Name
			hint = "Sovereign vault initialized"
		}
		return strings.Join([]string{
			m.styles.header.Render("Phase 4 — encrypted recovery"), "",
			"Vault        unlocked", "Destination  " + destinations,
			"Engine       Restic 0.19.1", "Session      tmux-backed from widget", "",
			m.styles.status.Render(hint),
		}, "\n")
	}
	if m.mode == modeBackup {
		return m.backupScreen()
	}
	if m.mode == modeSnapshots {
		return m.snapshotsScreen()
	}
	if m.mode == modeRestore {
		return m.restoreScreen()
	}
	if m.mode == modeProfiles {
		return m.applicationsScreen()
	}
	if m.mode == modeCheck {
		body := m.styles.header.Render("CHECK REPOSITORY") + "\n\n"
		if m.busy {
			body += m.styles.status.Render("◉ verifying encrypted indexes…")
		} else {
			body += "Press enter to verify repository indexes."
		}
		if m.err != "" {
			body += "\n\n" + m.styles.status.Render(m.err)
			body += "\n\n" + m.styles.help.Render("No repair runs automatically. Verify mount ownership and free space, retry Check, then review repository repair guidance before changing indexes.")
		}
		return body
	}
	if m.mode == modeTune {
		return m.tuneScreen()
	}
	if m.mode == modeDestinations {
		return m.destinationScreen()
	}
	if m.mode == modeRecovery {
		return m.recoveryScreen()
	}
	if m.mode == modeNuke {
		return m.nukeScreen()
	}
	return m.styles.header.Render(m.title()) + "\n\n" + navigationDescription(m.mode)
}

func (m Model) destinationScreen() string {
	body := m.styles.header.Render("DESTINATIONS") + "\n\n"
	if len(m.cfg.Destinations) > 0 && m.destinationStage == "" {
		if m.destinationSelection < 0 || m.destinationSelection >= len(m.cfg.Destinations) {
			m.destinationSelection = 0
		}
		for i, destination := range m.cfg.Destinations {
			cursor, active := "  ", " "
			if i == m.destinationSelection {
				cursor = "> "
			}
			if destination.ID == m.cfg.Active().ID {
				active = "●"
			}
			body += fmt.Sprintf("%s%d %s %-20s %s\n", cursor, i+1, active, destination.Name, strings.ToUpper(destination.Kind))
		}
		destination := m.cfg.Destinations[m.destinationSelection]
		connections := "auto (starts at 4)"
		if destination.Connections > 0 {
			connections = fmt.Sprint(destination.Connections)
		}
		transport := "encrypted local repository"
		if destination.Kind == "ssh" {
			transport = "encrypted SFTP / host key pinned"
		}
		body += "\nRepository  " + destination.Repository + "\nRepo ID     " + shortID(destination.RepositoryID) +
			"\nMachine     " + m.cfg.Machine.Name + "  " + shortID(m.cfg.Machine.ID) + "\nTransport   " + transport + "\nConnections " + connections
		if destination.UploadLimitKiB > 0 {
			body += fmt.Sprintf("\nUpload guard %d MiB/s", destination.UploadLimitKiB/1024)
		} else if destination.Kind == "ssh" {
			body += "\nUpload guard unlimited"
		}
		return body + "\n\n" + m.styles.help.Render("↑/↓ select • enter active • 1-9 quick select • D next • n add")
	}
	if m.destinationStage == "" {
		return body + "Press enter to configure the default SSH destination."
	}
	if m.destinationStage == "choose" {
		return body + "CREATE NEW REPOSITORY\n\n[s] SSH target (default)\n[l] Local filesystem repository\n\nCONNECT EXISTING\n\n[c] Existing local repository\n\nConnect verifies the repository password and ID. It never runs restic init.\nExisting SSH repositories are connected by importing their recovery kit."
	}
	if m.busy {
		return body + m.styles.status.Render("◉ "+m.status+"…")
	}
	if m.destinationStage == "confirm-host" {
		body += "Server presented:\n\n" + m.styles.status.Render(m.sshFingerprint)
		body += "\n\nVerify this fingerprint belongs to your server.\nPress enter to trust and bootstrap, or esc to abort."
	} else if m.destinationStage == "local" || m.destinationStage == "connect-local" {
		body += m.destinationInputs[0].View() + "\n\nRepository contents are encrypted before they reach this path."
	} else {
		for i := range m.destinationInputs {
			body += m.destinationInputs[i].View() + "\n"
		}
		body += "\nPassword is bootstrap-only and is never saved."
	}
	if m.err != "" {
		body += "\n\n" + m.styles.status.Render(m.err)
	}
	return body
}

func (m Model) backupScreen() string {
	body := m.styles.header.Render("BACKUP NOW") + "\n\n"
	if !m.busy {
		body += "Profile      " + strings.ToUpper(m.selectedProfile) + "  [1 core / 2 home / 3 heavy]\nDestination  "
		if len(m.cfg.Destinations) == 0 {
			body += "not initialized"
		} else {
			body += m.cfg.Active().Name
		}
		body += "\n\nPress enter to start the encrypted incremental backup."
		if m.selectedProfile == "heavy" {
			body += "\n\n" + m.styles.help.Render("Heavy is deduplicated independently. Live writable VM/container data is refused; stop it before capture.")
			if len(m.heavyReport.Images) > 0 {
				var logical int64
				for _, image := range m.heavyReport.Images {
					logical += image.Logical
				}
				state := "SAFE / IDLE"
				if !m.heavyReport.Safe {
					state = fmt.Sprintf("BLOCKED / %d LIVE WRITERS", len(m.heavyReport.Writers))
				}
				body += fmt.Sprintf("\n\nImages       %d\nLogical      %s\nPreflight    %s", len(m.heavyReport.Images), bytesText(uint64(logical)), state)
			}
		}
	} else {
		if m.progress.MessageType != "status" || (m.progress.TotalFiles == 0 && m.progress.TotalBytes == 0) {
			body += m.styles.status.Render("◉ scanning source and calculating totals…")
			body += "\n\n" + m.styles.help.Render("The measured percentage appears when Restic finishes discovery.")
			if m.err != "" {
				body += "\n\n" + m.styles.status.Render(m.err)
			}
			return body
		}
		percent := m.progress.PercentDone
		filled := int(percent * 32)
		if filled < 0 {
			filled = 0
		}
		if filled > 32 {
			filled = 32
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", 32-filled)
		body += lipgloss.NewStyle().Foreground(accent).Render("[" + bar + "]")
		body += fmt.Sprintf("  %3.0f%%\n\n", percent*100)
		body += fmt.Sprintf("Files       %d / %d\n", m.progress.FilesDone, m.progress.TotalFiles)
		body += fmt.Sprintf("Data        %s / %s\n", bytesText(m.progress.BytesDone), bytesText(m.progress.TotalBytes))
		body += fmt.Sprintf("Elapsed     %s\nETA         %s", timeText(m.progress.SecondsElapsed), timeText(m.progress.SecondsRemaining))
	}
	if m.err != "" {
		body += "\n\n" + m.styles.status.Render(m.err)
	}
	return body
}

func (m Model) snapshotsScreen() string {
	body := m.styles.header.Render("RESTORE POINTS") + "\n\n"
	if m.busy {
		return body + m.styles.status.Render("◉ loading encrypted snapshot index…")
	}
	if len(m.snapshots) == 0 {
		return body + "Press enter to load recoverable point-in-time versions."
	}
	for i, snapshot := range m.snapshots {
		if i == 8 {
			body += "…\n"
			break
		}
		body += fmt.Sprintf("%s  %s  %s\n", snapshot.ShortID, snapshot.Time.Local().Format("2006-01-02 15:04"), strings.Join(snapshot.Tags, ","))
	}
	return body
}

func timeText(seconds uint64) string {
	if seconds == 0 {
		return "--"
	}
	return fmt.Sprintf("%s", time.Duration(seconds)*time.Second)
}
