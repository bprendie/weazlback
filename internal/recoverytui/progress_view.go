package recoverytui

import (
	"fmt"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/freshrestore"
)

func (m Model) progressView() string {
	elapsed := time.Since(m.started).Round(time.Second)
	if m.height > 0 && m.height < 30 {
		return strings.Join([]string{
			compactLaneView("FILES", m.filesystemProgress), compactLaneView("PKGS", m.packageProgress),
			compactLaneView("APPS", m.appProgress), compactLaneView("BROWSER", m.browserProgress),
			"Elapsed " + elapsed.String() + "  •  journal active",
		}, "\n")
	}
	filesystem := laneView("FILESYSTEM / "+strings.ToUpper(m.filesystemProgress.Lane), m.filesystemProgress, false)
	packages := laneView("PACKAGE CAPSULE / "+strings.ToUpper(m.packageProgress.Lane), m.packageProgress, true)
	applications := laneView("APPLICATIONS / "+strings.ToUpper(m.appProgress.Lane), m.appProgress, true)
	browser := laneView("BROWSER COMPATIBILITY", m.browserProgress, true)
	return filesystem + "\n\n" + packages + "\n\n" + applications + "\n\n" + browser + "\n\nElapsed " + elapsed.String() + "  •  resumable journal active"
}

func compactLaneView(title string, progress freshrestore.RestoreProgress) string {
	resolved, percent := progress.Completed+progress.Failed, 0
	if progress.Total > 0 {
		percent = min(100, resolved*100/progress.Total)
	}
	filled := percent * 12 / 100
	detail := progress.Current
	if progress.Total > 0 {
		detail = fmt.Sprintf("%d/%d", resolved, progress.Total)
	}
	if progress.BytesPerSecond > 0 {
		detail += " " + restoreRate(progress.BytesPerSecond)
	}
	return fmt.Sprintf("%-7s [%s%s] %3d%% %s", title, strings.Repeat("█", filled), strings.Repeat("░", 12-filled), percent, detail)
}

func laneView(title string, p freshrestore.RestoreProgress, failures bool) string {
	if p.Total <= 0 {
		label := p.Current
		if label == "" {
			label = "waiting for measured totals"
		}
		return title + "\n" + label + "\n[▓▒░▒▓▒░▒▓▒░▒▓▒░▒▓▒░▒▓▒░▒▓▒░]"
	}
	resolved := p.Completed
	if failures {
		resolved += p.Failed
	}
	percent := min(100, resolved*100/p.Total)
	filled := percent * 28 / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 28-filled)
	detail := fmt.Sprintf("%d / %d", resolved, p.Total)
	if p.BytesTotal > 0 {
		detail += fmt.Sprintf("  •  %.1f / %.1f GiB", float64(p.BytesDone)/(1<<30), float64(p.BytesTotal)/(1<<30))
	}
	if p.BytesPerSecond > 0 {
		detail += "  •  output " + restoreRate(p.BytesPerSecond)
	}
	if p.WireBytesPerSecond > 0 {
		detail += "  •  source " + restoreRate(p.WireBytesPerSecond)
	}
	if p.FilesPerSecond > 0 {
		detail += fmt.Sprintf("  •  %.1f files/s", p.FilesPerSecond)
	}
	if failures && p.Failed > 0 {
		detail += fmt.Sprintf("  •  %d exceptions", p.Failed)
	}
	return fmt.Sprintf("%s\n%s\n[%s] %d%%\n%s", title, p.Current, bar, percent, detail)
}

func restoreRate(value float64) string {
	if value >= 1<<30 {
		return fmt.Sprintf("%.1f GiB/s", value/(1<<30))
	}
	if value >= 1<<20 {
		return fmt.Sprintf("%.1f MiB/s", value/(1<<20))
	}
	if value >= 1<<10 {
		return fmt.Sprintf("%.1f KiB/s", value/(1<<10))
	}
	return fmt.Sprintf("%.0f B/s", value)
}

func reviewLines(plan freshrestore.Plan) []string {
	var lines []string
	add := func(label string, values []string) {
		for _, value := range values {
			lines = append(lines, fmt.Sprintf("%-9s %s", label, value))
		}
	}
	add("OFFICIAL", plan.Official)
	add("AUR", plan.AUR)
	for _, item := range plan.PackageDelta.Local {
		lines = append(lines, fmt.Sprintf("%-9s %s %s", "CAPSULE", item.Name, item.Version))
	}
	add("FLATPAK", plan.Flatpak)
	add("SYSTEM", plan.SystemServices)
	add("USER", plan.UserServices)
	add("LOCALAPP", plan.LocalApps)
	if plan.Applications != nil {
		add("REVIEW", plan.Applications.ManualReview)
	}
	if plan.PackageCapsule != nil {
		add("REVIEW", plan.PackageCapsule.ManualReview)
	}
	if len(lines) == 0 {
		lines = append(lines, "No application changes required")
	}
	return lines
}
