package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) tuneScreen() string {
	body := m.styles.header.Render("TUNE REPOSITORY") + "\n\n"
	switch m.tuneStage {
	case "":
		body += "Measure repository concurrency and end-to-end SSH upload bandwidth.\n\nPress enter to begin."
	case "connections":
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		body += fmt.Sprintf("%s  testing %d connections…", frames[m.tuneFrame%len(frames)], m.tuneActiveConnection)
	case "choose-connections":
		body += "CONNECTION REPORT\n\n"
		for _, trial := range m.tuneTrials {
			cursor := "  "
			if trial.Connections == m.tuneConnection {
				cursor = "> "
			}
			result := trial.Elapsed.Round(time.Millisecond).String()
			if trial.Error != "" {
				result = "failed"
			}
			body += fmt.Sprintf("%s%2d connections  %s\n", cursor, trial.Connections, result)
		}
		body += "\n" + m.styles.help.Render("↑/↓ choose • enter continue")
	case "bandwidth":
		percent := float64(m.tuneProbeWritten) / float64(restic.UploadProbeBytes)
		if percent > 1 {
			percent = 1
		}
		filled := int(percent * 24)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", 24-filled)
		rate := 0.0
		if m.tuneProbeElapsed > 0 {
			rate = float64(m.tuneProbeWritten) / (1 << 20) / m.tuneProbeElapsed.Seconds()
		}
		body += lipgloss.NewStyle().Foreground(accent).Render("[" + bar + "]")
		body += fmt.Sprintf(" %3.0f%%\n\n%.1f / 100 MiB\n%.1f MiB/s\n%s elapsed",
			percent*100, float64(m.tuneProbeWritten)/(1<<20), rate, m.tuneProbeElapsed.Round(100*time.Millisecond))
	case "choose-bandwidth":
		recommended := restic.RecommendedUploadMiB(m.tuneProbe.MiBPerS)
		body += fmt.Sprintf("BANDWIDTH REPORT\n\nSustained    %.1f MiB/s\n79%% guard    %d MiB/s\n\n%s\n\n%s",
			m.tuneProbe.MiBPerS, recommended, m.tuneInput.View(), m.styles.help.Render("enter save • 0 unlimited • esc back"))
	case "done":
		destination := m.cfg.Active()
		guard := "unlimited"
		if destination != nil && destination.UploadLimitKiB > 0 {
			guard = fmt.Sprintf("%d MiB/s", destination.UploadLimitKiB/1024)
		}
		body += fmt.Sprintf("TUNING SAVED\n\nConnections   %d\nUpload guard  %s\n\n%s", m.tuneConnection, guard, m.styles.help.Render("enter close"))
	}
	if m.err != "" {
		body += "\n\n" + m.styles.status.Render(m.err)
	}
	return body
}
