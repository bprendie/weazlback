package tui

import (
	"context"
	"fmt"

	"github.com/bprendie/weazlback/internal/inventory"
	tea "github.com/charmbracelet/bubbletea"
)

type applicationsMsg struct {
	manifest inventory.ApplicationManifest
	err      error
}

func (m Model) startApplications() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	m.busy, m.status, m.err = true, "capturing application inventory", ""
	return m, func() tea.Msg {
		manifest, err := inventory.CaptureApplications(context.Background())
		return applicationsMsg{manifest: manifest, err: err}
	}
}

func (m Model) applicationsScreen() string {
	body := m.styles.header.Render("PROFILES / APPLICATION MANIFEST") + "\n\n"
	if m.busy {
		return body + m.styles.status.Render("◉ capturing applications, services, and Omarchy plugins…")
	}
	if m.applications == nil {
		return body + "Every Core restore point includes a validated application inventory and deterministic restore plan.\n\nPress enter to inspect the current machine."
	}
	a := m.applications
	body += fmt.Sprintf("Official packages   %d\nAUR/foreign         %d\nFlatpak apps        %d\nSystem services     %d\nUser services       %d\nOmarchy plugins     %d\nDiscovery hints     %d\nManual review       %d\n\nCaptured  %s",
		len(a.Packages.OfficialExplicit), len(a.Packages.ForeignExplicit), len(a.Packages.FlatpakApps),
		len(a.Services.SystemEnabled), len(a.Services.UserEnabled), len(a.OmarchyPlugins), len(a.PkgInstallPlan), len(a.ManualReview),
		a.CapturedAt.Local().Format("2006-01-02 15:04:05"))
	return body
}
