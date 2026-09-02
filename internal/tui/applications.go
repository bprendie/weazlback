package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
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
	body := m.styles.header.Render("PROFILES / PACKAGE CAPSULE") + "\n\n"
	if m.packageStage == "confirm-aur" {
		return body + "AUR BUILD EXECUTION\n\nThis runs reviewed AUR PKGBUILDs now so disaster recovery will not compile them later.\nPKGBUILDs execute code as your user and may request dependency installation.\n\nenter build + capture  •  esc cancel"
	}
	if m.busy {
		if m.operation == "package capsule" {
			return body + packageProgressView(m.packageProgress)
		}
		return body + m.styles.status.Render("◉ capturing applications, services, and Omarchy plugins…")
	}
	schedule := "off (manual only)"
	if m.cfg.PackagePolicy.Scheduled {
		schedule = fmt.Sprintf("every %d days", m.cfg.PackagePolicy.IntervalDays)
	}
	body += "Package artifacts are an independent encrypted Restore Point.\nCore and Home never traverse package caches.\n\n"
	if m.packageManifest != nil {
		capsule := m.packageManifest
		body += fmt.Sprintf("Last capsule        %s\nInstalled packages  %d\nArtifacts captured  %d\nOfficial / foreign  %d / %d\nArtifact data       %s\nExceptions          %d\n\n",
			capsule.CapturedAt.Local().Format("2006-01-02 15:04:05"), capsule.Summary.Installed, capsule.Summary.Captured,
			capsule.Summary.Official, capsule.Summary.Foreign, bytesText(uint64(capsule.Summary.Bytes)), len(capsule.Exceptions))
	}
	body += fmt.Sprintf("Refresh schedule    %s\n\n[P] refresh cached + official artifacts\n[A] review and build missing AUR artifacts\n[S] toggle independent schedule\n[enter] inspect application manifest", schedule)
	if m.applications == nil {
		return body
	}
	a := m.applications
	body += fmt.Sprintf("\n\nAPPLICATION MANIFEST\nOfficial packages   %d\nAUR/foreign         %d\nFlatpak apps        %d\nSystem services     %d\nUser services       %d\nOmarchy plugins     %d\nDiscovery hints     %d\nManual review       %d\n\nCaptured  %s",
		len(a.Packages.OfficialExplicit), len(a.Packages.ForeignExplicit), len(a.Packages.FlatpakApps),
		len(a.Services.SystemEnabled), len(a.Services.UserEnabled), len(a.OmarchyPlugins), len(a.PkgInstallPlan), len(a.ManualReview),
		a.CapturedAt.Local().Format("2006-01-02 15:04:05"))
	return body
}

func packageProgressStatus(progress packagecapsule.Progress) string {
	if progress.Package != "" {
		return fmt.Sprintf("Package Capsule %s — %d/%d %s", progress.Phase, progress.Completed, progress.Total, progress.Package)
	}
	return "Package Capsule " + progress.Phase
}

func packageProgressView(progress packagecapsule.Progress) string {
	percent := 0
	if progress.Total > 0 {
		percent = progress.Completed * 100 / progress.Total
	}
	return fmt.Sprintf("◉ PACKAGE CAPSULE / %s\n\n[%s%s] %3d%%\n\nPackage  %s\nItems    %d / %d\nData     %s",
		strings.ToUpper(progress.Phase), strings.Repeat("█", percent*24/100), strings.Repeat("░", 24-percent*24/100), percent,
		progress.Package, progress.Completed, progress.Total, bytesText(uint64(progress.Bytes)))
}
