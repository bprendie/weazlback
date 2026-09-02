package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
)

func packagesCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "schedule" {
		return packageScheduleCommand(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "refresh" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("packages refresh", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	download := flags.Bool("download-missing", true, "download missing official package artifacts")
	buildAUR := flags.Bool("build-missing-aur", false, "review and build missing AUR artifacts")
	dryRun := flags.Bool("dry-run", false, "capture and validate without creating a Restore Point")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, destination, vaultFile, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer vaultFile.Lock()
	if *download || *buildAUR {
		if err := authorizePackageCapture(stdout, stderr); err != nil {
			return err
		}
	}
	repo, err := repositoryFrom(vaultFile, *destination)
	if err != nil {
		return err
	}
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return err
	}
	staging, err := packageStagingRoot()
	if err != nil {
		return err
	}
	manifest, root, cleanup, err := packagecapsule.Capture(packagecapsule.Options{
		Context: ctx, MachineID: cfg.Machine.ID, StagingRoot: staging, Download: *download,
		BuildMissingAUR: *buildAUR, Run: packagecapsule.ExecRunner{Context: ctx},
		Progress: func(value packagecapsule.Progress) {
			if value.Package != "" {
				fmt.Fprintf(stderr, "\r%-9s %d/%d  %-32s", value.Phase, value.Completed, value.Total, value.Package)
			}
		},
	})
	fmt.Fprintln(stderr)
	if err != nil {
		return err
	}
	defer cleanup()
	if !*dryRun {
		if err := restic.NewService(stderr).BackupMachineWithProgress(ctx, repo, "packages", cfg.Machine.ID, []string{root}, nil, false, false, nil); err != nil {
			return err
		}
		if err := catalog.Refresh(ctx, vaultFile, destination.ID, repo, cfg.Machine.ID, "packages"); err != nil {
			fmt.Fprintf(stderr, "warning: package history catalog update deferred: %v\n", err)
		}
		now := manifest.CapturedAt
		cfg.PackagePolicy.LastCaptured = &now
		if path, err := config.Path(); err == nil {
			if err := config.Save(path, cfg); err != nil {
				return err
			}
		}
	}
	return writeJSON(stdout, manifest.Summary)
}

func packageScheduleCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("packages schedule", flag.ContinueOnError)
	flags.SetOutput(stderr)
	days := flags.Int("days", 30, "refresh reminder interval; 0 disables")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *days < 0 || *days > 3650 {
		return errors.New("--days must be between 0 and 3650")
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg.PackagePolicy.Scheduled = *days > 0
	if *days > 0 {
		cfg.PackagePolicy.IntervalDays = *days
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	if *days == 0 {
		_, err = fmt.Fprintln(stdout, "Package Capsule refresh reminders disabled")
	} else {
		_, err = fmt.Fprintf(stdout, "Package Capsule refresh reminder every %d days\n", *days)
	}
	return err
}

func packageStagingRoot() (string, error) {
	path, err := config.Path()
	if err != nil {
		return "", err
	}
	root := filepath.Join(filepath.Dir(path), "staging")
	if root == "." || root == "" || root == string(filepath.Separator) {
		return "", errors.New("unsafe package staging root")
	}
	return root, nil
}

func authorizePackageCapture(stdout, stderr io.Writer) error {
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, stdout, stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("package capture sudo authorization failed: %w", err)
	}
	return nil
}
