package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
)

func fileRestorePoint(s restic.Snapshot) bool {
	switch restic.Profile(s.Tags) {
	case "core", "home", "heavy":
		return true
	}
	return false
}

func preferredRestorePoint(points []restic.Snapshot) restic.Snapshot {
	for _, p := range points {
		if restic.Profile(p.Tags) == "home" && restic.SnapshotHealth(p.Tags) == "healthy" {
			return p
		}
	}
	for _, p := range points {
		if fileRestorePoint(p) && restic.SnapshotHealth(p.Tags) == "healthy" {
			return p
		}
	}
	for _, p := range points {
		if fileRestorePoint(p) {
			return p
		}
	}
	return restic.Snapshot{}
}

func (m *Model) openRestorePoints() {
	m.pointChoices = nil
	m.pointIndex = 0
	for i, p := range m.snapshots {
		if fileRestorePoint(p) && len(restic.FilterIdentity([]restic.Snapshot{p}, m.selectedRestoreMachineID())) > 0 {
			if i == m.restoreSnapshot {
				m.pointIndex = len(m.pointChoices)
			}
			m.pointChoices = append(m.pointChoices, i)
		}
	}
	m.restoreStage = "points"
}

func (m Model) restorePointsView() string {
	body := "RESTORE POINTS — newest first\n\n"
	start, end := restoreWindow(m.pointIndex, len(m.pointChoices), m.restorePageSize())
	for i := start; i < end; i++ {
		p := m.snapshots[m.pointChoices[i]]
		line := fmt.Sprintf("%s  %-5s  %s", p.Time.Local().Format("Mon 2006-01-02 15:04:05"), restic.Profile(p.Tags), restic.SnapshotHealth(p.Tags))
		body += m.restoreRow(line, i == m.pointIndex) + "\n"
	}
	return body + "\n↑/↓ select • / find date • Enter open exact point • Esc back"
}

func (m Model) restoreMachineName(id string) string {
	for _, identity := range m.restoreIdentities {
		if identity.ID == id {
			return identity.Name
		}
	}
	return "Unknown source"
}

func (m Model) sourceRestorePath(value string) string {
	if value != "~" && !strings.HasPrefix(value, "~/") {
		return filepath.Clean(value)
	}
	homes := map[string]bool{}
	if m.restoreSnapshot < len(m.snapshots) {
		for _, path := range m.snapshots[m.restoreSnapshot].Paths {
			parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
			if len(parts) >= 2 && parts[0] == "home" {
				homes["/home/"+parts[1]] = true
			}
			if path == "/root" || strings.HasPrefix(path, "/root/") {
				homes["/root"] = true
			}
		}
	}
	// Older fixtures and some repositories omit snapshot paths; nodes are authoritative.
	if len(homes) == 0 {
		for path := range m.nodes {
			parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
			if len(parts) >= 2 && parts[0] == "home" {
				homes["/home/"+parts[1]] = true
			}
		}
	}
	if len(homes) == 1 {
		for home := range homes {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(value, "~"), "/"))
		}
	}
	return "" // Ambiguous source homes require an explicit absolute path.
}
