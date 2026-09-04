package tui

import (
	"fmt"
	"strings"
	"time"
)

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
