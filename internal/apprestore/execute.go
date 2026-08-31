package apprestore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Run(context.Context, string, ...string) error
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	var output bytes.Buffer
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, &output, &output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func Execute(ctx context.Context, plan Plan, runner Runner, progress func(Progress)) Result {
	if runner == nil {
		runner = CommandRunner{}
	}
	result := Result{}
	for _, pkg := range plan.Unavailable {
		result.Unavailable = append(result.Unavailable, pkg.Name)
	}
	for _, pkg := range plan.Conflicts {
		result.Conflicts = append(result.Conflicts, pkg.Name)
	}
	packages := append(append([]Package(nil), plan.Install...), plan.Substitutions...)
	total := len(packages) + len(plan.SystemServices) + len(plan.UserServices)
	done, failed := 0, 0
	for _, source := range []Source{Official, AUR, Flatpak} {
		var selected []Package
		for _, pkg := range packages {
			if pkg.Source == source {
				selected = append(selected, pkg)
			}
		}
		if len(selected) == 0 {
			continue
		}
		emit(progress, Progress{Lane: string(source), Current: selected[0].Name, Completed: done, Failed: failed, Total: total})
		batchCtx, cancelBatch := context.WithTimeout(ctx, packageTimeoutFor(source))
		batchErr := installBatch(batchCtx, runner, source, selected)
		cancelBatch()
		if batchErr == nil {
			for _, pkg := range selected {
				done++
				recordInstalled(&result, pkg)
				emit(progress, Progress{Lane: string(source), Current: pkg.Name, Completed: done, Failed: failed, Total: total})
			}
			continue
		}
		for _, pkg := range selected {
			emit(progress, Progress{Lane: string(source), Current: pkg.Name, Completed: done, Failed: failed, Total: total})
			itemCtx, cancel := context.WithTimeout(ctx, packageTimeoutFor(source))
			err := installOne(itemCtx, runner, pkg)
			cancel()
			if err != nil {
				failed++
				result.Failures = append(result.Failures, fmt.Sprintf("%s / %s: %v", source, pkg.Name, err))
			} else {
				done++
				recordInstalled(&result, pkg)
			}
			emit(progress, Progress{Lane: string(source), Current: pkg.Name, Completed: done, Failed: failed, Total: total})
		}
	}
	for _, service := range plan.SystemServices {
		err := runner.Run(ctx, "sudo", "-n", "systemctl", "enable", service)
		done, failed = serviceResult(service, err, done, failed, total, &result, progress, "system services")
	}
	for _, service := range plan.UserServices {
		err := runner.Run(ctx, "systemctl", "--user", "enable", service)
		done, failed = serviceResult(service, err, done, failed, total, &result, progress, "user services")
	}
	for _, pkg := range plan.InstalledLater {
		if pkg.Source == Flatpak {
			result.RemovalCommands = append(result.RemovalCommands, "flatpak uninstall --user -- "+shellQuote(pkg.Name))
		} else {
			result.RemovalCommands = append(result.RemovalCommands, "sudo pacman -Rns -- "+shellQuote(pkg.Name))
		}
	}
	return result
}

func installBatch(ctx context.Context, runner Runner, source Source, packages []Package) error {
	args := make([]string, 0, len(packages))
	for _, pkg := range packages {
		args = append(args, pkg.Name)
	}
	switch source {
	case Official:
		return runner.Run(ctx, "sudo", append([]string{"-n", "pacman", "-S", "--needed", "--noconfirm", "--"}, args...)...)
	case AUR:
		helper := "paru"
		if _, err := exec.LookPath(helper); err != nil {
			helper = "yay"
		}
		return runner.Run(ctx, helper, append([]string{"-S", "--needed", "--noconfirm", "--batchinstall", "--"}, args...)...)
	case Flatpak:
		return runner.Run(ctx, "flatpak", append([]string{"install", "--user", "--noninteractive", "flathub"}, args...)...)
	}
	return fmt.Errorf("unsupported package source %q", source)
}

func recordInstalled(result *Result, pkg Package) {
	result.Installed = append(result.Installed, pkg.Name)
	if pkg.AvailableVersion != "" && pkg.WantedVersion != "" && pkg.AvailableVersion != pkg.WantedVersion {
		result.Substituted = append(result.Substituted, fmt.Sprintf("%s %s → %s", pkg.Name, pkg.WantedVersion, pkg.AvailableVersion))
	}
}

func installOne(ctx context.Context, runner Runner, pkg Package) error {
	switch pkg.Source {
	case Official:
		return runner.Run(ctx, "sudo", "-n", "pacman", "-S", "--needed", "--noconfirm", "--", pkg.Name)
	case AUR:
		helper := "paru"
		if _, err := exec.LookPath(helper); err != nil {
			helper = "yay"
		}
		return runner.Run(ctx, helper, "-S", "--needed", "--noconfirm", "--batchinstall", "--", pkg.Name)
	case Flatpak:
		return runner.Run(ctx, "flatpak", "install", "--user", "--noninteractive", "flathub", pkg.Name)
	}
	return fmt.Errorf("unsupported package source %q", pkg.Source)
}

func packageTimeout(source Source) time.Duration {
	if source == AUR {
		return 20 * time.Minute
	}
	return 5 * time.Minute
}

var packageTimeoutFor = packageTimeout

func serviceResult(name string, err error, done, failed, total int, result *Result, progress func(Progress), lane string) (int, int) {
	if err != nil {
		failed++
		result.MissingServices = append(result.MissingServices, name+": "+err.Error())
	} else {
		done++
	}
	emit(progress, Progress{Lane: lane, Current: name, Completed: done, Failed: failed, Total: total})
	return done, failed
}

func emit(callback func(Progress), value Progress) {
	if callback != nil {
		callback(value)
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
