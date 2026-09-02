package freshrestore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/inventory"
)

func MissingApplications(ctx context.Context, manifest inventory.ApplicationManifest) (official, aur, flatpaks []string) {
	installed := commandSet(ctx, "pacman", "-Qq")
	flatInstalled := commandSet(ctx, "flatpak", "list", "--app", "--columns=application")
	official = missingSatisfied(ctx, manifest.PackagePlan.Official, installed)
	aur = missing(manifest.PackagePlan.AUR, installed)
	flatpaks = missing(manifest.PackagePlan.Flatpak, flatInstalled)
	return
}

func missingSatisfied(ctx context.Context, wanted []string, installed map[string]bool) []string {
	if len(wanted) == 0 {
		return nil
	}
	if _, err := exec.LookPath("pacman"); err != nil {
		return missing(wanted, installed)
	}
	out, err := exec.CommandContext(ctx, "pacman", append([]string{"-T"}, wanted...)...).Output()
	if err == nil {
		return nil
	}
	var result []string
	for _, line := range strings.Fields(string(out)) {
		result = append(result, strings.TrimSpace(line))
	}
	if len(result) == 0 {
		return missing(wanted, installed)
	}
	sort.Strings(result)
	return result
}

func MissingServices(ctx context.Context, manifest inventory.ApplicationManifest) (system, user []string) {
	systemEnabled := commandFirstFieldSet(ctx, "systemctl", "list-unit-files", "--state=enabled", "--no-legend")
	userEnabled := commandFirstFieldSet(ctx, "systemctl", "--user", "list-unit-files", "--state=enabled", "--no-legend")
	return missing(manifest.Services.SystemEnabled, systemEnabled), missing(manifest.Services.UserEnabled, userEnabled)
}

func ReconcileApplications(ctx context.Context, plan Plan) []string {
	return ReconcileApplicationsProgress(ctx, plan, nil)
}

func ReconcileApplicationsProgress(ctx context.Context, plan Plan, progress func(RestoreProgress)) []string {
	return reconcileApplicationLanes(ctx, plan, progress, true, true, 0)
}

func reconcileApplicationLanes(ctx context.Context, plan Plan, progress func(RestoreProgress), system, user bool, initial int) []string {
	var failures []string
	invoke := func(name string, args ...string) error {
		if progress == nil {
			return visible(ctx, name, args...)
		}
		return quiet(ctx, name, args...)
	}
	total := len(plan.Official) + len(plan.AUR) + len(plan.Flatpak) + len(plan.SystemServices) + len(plan.UserServices)
	done, failed := initial, 0
	run := func(lane string, items []string, command func([]string) error) {
		if len(items) == 0 {
			return
		}
		emitProgress(progress, RestoreProgress{Phase: "applications", Lane: lane, Current: items[0], Completed: done, Failed: failed, Total: total})
		err := command(items)
		if err == nil {
			for _, item := range items {
				done++
				emitProgress(progress, RestoreProgress{Phase: "applications", Lane: lane, Current: item, Completed: done, Failed: failed, Total: total})
			}
			return
		}
		if len(items) == 1 {
			failed++
			failures = append(failures, lane+" / "+items[0]+": "+err.Error())
			emitProgress(progress, RestoreProgress{Phase: "applications", Lane: lane, Current: items[0], Completed: done, Failed: failed, Total: total})
			return
		}
		for _, item := range items {
			itemErr := command([]string{item})
			if itemErr == nil {
				done++
			} else {
				failed++
				failures = append(failures, lane+" / "+item+": "+itemErr.Error())
			}
			emitProgress(progress, RestoreProgress{Phase: "applications", Lane: lane, Current: item, Completed: done, Failed: failed, Total: total})
		}
	}
	if system && len(plan.Official) > 0 {
		run("official packages", plan.Official, func(items []string) error {
			args := append([]string{"-n", "pacman", "-S", "--needed", "--noconfirm", "--"}, items...)
			if progress != nil {
				return runPacmanProgress(ctx, "sudo", args, "applications", "official packages", items, progress)
			}
			return invoke("sudo", args...)
		})
	}
	if system && len(plan.AUR) > 0 {
		var artifactPackages, fallback []string
		for _, name := range plan.AUR {
			if path := plan.ArtifactFiles[name]; path != "" && validArtifact(plan.Applications, name, path) {
				artifactPackages = append(artifactPackages, name)
			} else {
				fallback = append(fallback, name)
			}
		}
		if len(artifactPackages) > 0 {
			run("cached AUR artifacts", artifactPackages, func(items []string) error {
				args := []string{"-n", "pacman", "-U", "--needed", "--noconfirm", "--"}
				for _, name := range items {
					args = append(args, plan.ArtifactFiles[name])
				}
				if progress != nil {
					return runPacmanProgress(ctx, "sudo", args, "applications", "cached AUR artifacts", items, progress)
				}
				return invoke("sudo", args...)
			})
		}
		plan.AUR = fallback
	}
	if system && len(plan.AUR) > 0 {
		helper := "paru"
		batch := []string{"-S", "--needed", "--noconfirm", "--batchinstall", "--"}
		if _, err := exec.LookPath(helper); err != nil {
			helper = "yay"
			batch = []string{"-S", "--needed", "--noconfirm", "--"}
		}
		run("AUR packages", plan.AUR, func(items []string) error {
			return invoke(helper, append(batch, items...)...)
		})
	}
	if user && len(plan.Flatpak) > 0 {
		run("Flatpaks", plan.Flatpak, func(items []string) error {
			return invoke("flatpak", append([]string{"install", "--user", "--noninteractive"}, items...)...)
		})
	}
	if system && len(plan.SystemServices) > 0 {
		run("system services", plan.SystemServices, func(items []string) error {
			return invoke("sudo", append([]string{"-n", "systemctl", "enable"}, items...)...)
		})
	}
	if user && len(plan.UserServices) > 0 {
		run("user services", plan.UserServices, func(items []string) error {
			return invoke("systemctl", append([]string{"--user", "enable"}, items...)...)
		})
	}
	return failures
}

func validArtifact(manifest *inventory.ApplicationManifest, name, path string) bool {
	if manifest == nil {
		return false
	}
	for _, artifact := range manifest.AURArtifacts {
		if artifact.Package != name {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			return false
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		_ = file.Close()
		return err == nil && fmt.Sprintf("%x", hash.Sum(nil)) == artifact.SHA256
	}
	return false
}

func emitProgress(callback func(RestoreProgress), value RestoreProgress) {
	if callback != nil {
		callback(value)
	}
}

func AuthorizeSudo() error {
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func keepSudoAlive(ctx context.Context) func() {
	keepCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-keepCtx.Done():
				return
			case <-ticker.C:
				_ = exec.CommandContext(keepCtx, "sudo", "-n", "true").Run()
			}
		}
	}()
	return cancel
}

func visible(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func quiet(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var output bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func commandSet(ctx context.Context, name string, args ...string) map[string]bool {
	result := map[string]bool{}
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return result
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		result[strings.TrimSpace(scanner.Text())] = true
	}
	return result
}

func commandFirstFieldSet(ctx context.Context, name string, args ...string) map[string]bool {
	result := map[string]bool{}
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return result
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if fields := strings.Fields(scanner.Text()); len(fields) > 0 {
			result[fields[0]] = true
		}
	}
	return result
}

func missing(wanted []string, installed map[string]bool) []string {
	var result []string
	for _, name := range wanted {
		if !installed[name] {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func PlanText(plan Plan) string {
	points := fmt.Sprintf("Core           %s  %s", plan.Snapshot.ShortID, plan.Snapshot.Time.Local().Format("2006-01-02 15:04"))
	if plan.HomeSnapshot != nil {
		points += fmt.Sprintf("\nHome           %s  %s", plan.HomeSnapshot.ShortID, plan.HomeSnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	if plan.HeavySnapshot != nil {
		points += fmt.Sprintf("\nHeavy          %s  %s", plan.HeavySnapshot.ShortID, plan.HeavySnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	if plan.PackageSnapshot != nil {
		points += fmt.Sprintf("\nPackages       %s  %s", plan.PackageSnapshot.ShortID, plan.PackageSnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	identity := "preserve target identity"
	if plan.AdoptSourceIdentity {
		identity = "ADOPT source identity " + plan.SourceMachineID
	}
	return fmt.Sprintf("Recovery scope %s\nIncludes       %s\n%s\nHostname       %s\nMachine        %s\nHome mapping   %s -> %s\nOwnership      %d:%d -> %d:%d\nPackage delta  %d local / %d official online / %d foreign online / %d kept\nFlatpaks       %d\nServices       %d system / %d user",
		plan.Scope, recoveryScopeContents(plan.Scope), points, plan.Hostname, identity, plan.OriginalHome, plan.TargetHome, plan.SourceUID, plan.SourceGID, plan.TargetUID, plan.TargetGID,
		len(plan.PackageDelta.Local), len(plan.Official), len(plan.AUR), len(plan.PackageDelta.Kept), len(plan.Flatpak), len(plan.SystemServices), len(plan.UserServices))
}

func recoveryScopeContents(scope string) string {
	switch scope {
	case "everything":
		return "Applications (parallel) + Core + Home + Heavy"
	case "home":
		return "Applications (parallel) + Core + Home"
	case "applications":
		return "Applications only"
	default:
		return "Applications (parallel) + Core"
	}
}
