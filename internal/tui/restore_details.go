package tui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (m *Model) openRestoreDetails() {
	path := m.restoreVersionPath
	metadata := m.restoreMetadataPreview()
	if m.restoreStage == "versions" {
		versions := m.fileVersions()
		if m.versionIndex < len(versions) {
			v := versions[m.versionIndex]
			metadata = fmt.Sprintf("%s • %s • %s\n%s • %s • mode %04o • uid:gid %d:%d", m.restoreMachineName(v.MachineID), v.Profile, v.Time.Local().Format("2006-01-02 15:04:05"), v.Type, bytesText(v.Size), v.Mode&07777, v.UID, v.GID)
		}
	}
	if m.restoreStage == "browse" {
		if len(m.restoreVisible) == 0 {
			return
		}
		path = m.restoreVisible[m.restoreIndex].Path
	}
	m.detailReturn = m.restoreStage
	m.restoreStage = "details"
	m.detailOffset = 0
	m.detailText = fmt.Sprintf("Repository path: %q\n\n%s", path, metadata)
}

func (m Model) restoreDetailLines() []string {
	return strings.Split(ansi.Hardwrap(m.detailText, max(1, m.width-16), true), "\n")
}

func (m Model) restoreDetailsView() string {
	lines := m.restoreDetailLines()
	start := min(m.detailOffset, max(0, len(lines)-1))
	end := min(len(lines), start+m.restorePageSize())
	return "FILE DETAILS\n\n" + strings.Join(lines[start:end], "\n") + "\n\n↑/↓ scroll • Esc back"
}
