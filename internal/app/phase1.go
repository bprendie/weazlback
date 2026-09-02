package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/contracts"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/preflight"
	"github.com/bprendie/weazlback/internal/restic"
)

func initCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("name", "default", "destination name")
	repository := flags.String("repository", "", "local path or sftp:user@host:/path")
	sshKey := flags.String("ssh-key", "", "SSH private key to import into the vault")
	knownHosts := flags.String("known-hosts", "", "pinned OpenSSH known_hosts file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *repository == "" {
		return errors.New("--repository is required")
	}
	if !strings.HasPrefix(*repository, "sftp:") {
		if entries, readErr := os.ReadDir(*repository); readErr == nil && len(entries) != 0 {
			return fmt.Errorf("create-new repository requires an empty directory: %s", *repository)
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
	}
	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	v, err := openVault(cfg.ActiveVault, stderr, true)
	if err != nil {
		return err
	}
	defer v.Lock()
	id := slug(*name)
	if id == "" {
		return errors.New("destination name must contain a letter or number")
	}
	if findDestination(cfg, id) != nil {
		return fmt.Errorf("destination %q already exists", id)
	}
	password, err := randomSecret(32)
	if err != nil {
		return err
	}
	destination := config.Destination{ID: id, Name: *name, Kind: "local",
		Repository: *repository, PasswordKey: "destination/" + id + "/repository-password"}
	if strings.HasPrefix(*repository, "sftp:") {
		destination.Kind = "ssh"
	}
	if err := v.Put(destination.PasswordKey, password); err != nil {
		return err
	}
	if *sshKey != "" {
		key, err := os.ReadFile(*sshKey)
		if err != nil {
			return err
		}
		destination.SSHKeyKey = "destination/" + id + "/ssh-private-key"
		if err := v.Put(destination.SSHKeyKey, key); err != nil {
			return err
		}
	}
	if *knownHosts != "" {
		absolute, err := filepath.Abs(*knownHosts)
		if err != nil {
			return err
		}
		destination.SSHKnownHosts = absolute
	}
	repo, err := repositoryFrom(v, destination)
	if err != nil {
		return err
	}
	if err := restic.NewService(stderr).Initialize(ctx, repo); err != nil {
		return err
	}
	repositoryID, err := restic.NewService(stderr).RepositoryID(ctx, repo)
	if err != nil {
		return err
	}
	destination.RepositoryID = repositoryID
	cfg.Destinations = append(cfg.Destinations, destination)
	cfg.ActiveDestination = destination.ID
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "initialized encrypted %s destination %q\n", destination.Kind, id)
	return err
}

func backupCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	profileName := flags.String("profile", "core", "core, home, or heavy (packages use `weazlback packages refresh`)")
	dryRun := flags.Bool("dry-run", false, "show work without saving a snapshot")
	connections := flags.Int("connections", 0, "parallel repository connections (0 uses destination/default)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	profile := findProfile(cfg, *profileName)
	if profile == nil {
		return fmt.Errorf("profile %q not found", *profileName)
	}
	backupProfile := *profile
	manifestPath, cleanupManifest, err := backupmeta.PrepareApplications(ctx, profile.Name)
	if err != nil {
		return fmt.Errorf("application manifest: %w", err)
	}
	defer cleanupManifest()
	if manifestPath != "" {
		backupProfile.Includes = append(append([]string(nil), profile.Includes...), manifestPath)
	}
	backupProfile.Excludes = append([]string(nil), profile.Excludes...)
	if profile.Name == "core" || profile.Name == "home" {
		home, _ := os.UserHomeDir()
		backupProfile.Excludes = append(backupProfile.Excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
	}
	if profile.Name == "heavy" {
		heavyReport := heavy.Inspect(profile.Includes)
		if !heavyReport.Safe {
			var examples []string
			for _, writer := range heavyReport.Writers {
				examples = append(examples, fmt.Sprintf("%s (pid %d, %s)", writer.Path, writer.PID, writer.Process))
				if len(examples) == 5 {
					break
				}
			}
			return fmt.Errorf("Heavy backup refused: live writable data detected; stop the VM/container and retry: %s", strings.Join(examples, ", "))
		}
	}
	incomplete := false
	report := preflight.Scan(profile.Includes, profile.Excludes)
	if report.Unreadable > 0 && !destination.Privileged {
		fmt.Fprintf(stderr, "%d unreadable paths require sudo (examples: %s). Use sudo? [y/N — N creates an incomplete snapshot] ", report.Unreadable, strings.Join(report.Samples, ", "))
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			incomplete = true
			backupProfile.Excludes = append(backupProfile.Excludes, report.Paths...)
		} else {
			destination.Privileged = true
			cfgPath, pathErr := config.Path()
			if pathErr != nil {
				return pathErr
			}
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
		}
	}
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	if *connections > 0 {
		repo.Connections = *connections
	}
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return err
	}
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	started := time.Now()
	previous, _ := store.Load()
	_ = store.Save(contracts.Status{State: "backing-up", Destination: destination.ID,
		LastHealthy: previous.LastHealthy, LastReminder: previous.LastReminder, TravelUntil: previous.TravelUntil, Progress: &contracts.Progress{Phase: "backup"}})
	err = restic.NewService(stderr).BackupMachineWithProgress(ctx, repo, backupProfile.Name, cfg.Machine.ID, backupProfile.Includes, backupProfile.Excludes, *dryRun, incomplete, nil)
	if err != nil {
		_ = store.Save(contracts.Status{State: "failed", Destination: destination.ID, Error: "backup failed; open Weazlback for details", LastHealthy: previous.LastHealthy, LastReminder: previous.LastReminder, TravelUntil: previous.TravelUntil})
		return err
	}
	if !*dryRun {
		if catalogErr := catalog.Refresh(ctx, v, destination.ID, repo, cfg.Machine.ID, backupProfile.Name); catalogErr != nil {
			fmt.Fprintf(stderr, "warning: history catalog update deferred: %v\n", catalogErr)
		}
	}
	if *dryRun {
		_, err = fmt.Fprintf(stdout, "dry run complete; no snapshot created (%d unreadable paths would be skipped)\n", len(report.Paths))
		return err
	}
	state := "healthy"
	status := contracts.Status{State: state, Destination: destination.ID, LastHealthy: ptrTime(time.Now()), LastReminder: previous.LastReminder, TravelUntil: previous.TravelUntil, Progress: &contracts.Progress{Phase: "complete", Elapsed: time.Since(started)}}
	if incomplete {
		manifestPath, manifestErr := writeCLIManifest(profile.Name, destination.ID, report.Paths)
		if manifestErr != nil {
			return manifestErr
		}
		state, status.State, status.Incomplete, status.Skipped, status.Manifest = "incomplete", "incomplete", true, report.Unreadable, manifestPath
		status.LastHealthy = previous.LastHealthy
	}
	_ = store.Save(status)
	_, err = fmt.Fprintf(stdout, "backup %s in %s\n", state, time.Since(started).Round(time.Millisecond))
	return err
}

func writeCLIManifest(profile, destination string, skipped []string) (string, error) {
	path, err := preflight.ManifestPath()
	if err != nil {
		return "", err
	}
	err = preflight.WriteManifest(path, preflight.Manifest{SchemaVersion: 1, CreatedAt: time.Now(), Profile: profile, Destination: destination, Reason: "sudo declined", Skipped: skipped})
	return path, err
}

func snapshotsCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	destinationID, err := destinationFlag("list", args, stderr)
	if err != nil {
		return err
	}
	cfg, destination, v, err := loadRuntime(destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	snapshots, err := restic.NewService(stderr).SnapshotsForMachine(ctx, repo, cfg.Machine.ID)
	if err != nil {
		return err
	}
	return writeJSON(stdout, snapshots)
}

func checkCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	readData := flags.Bool("read-data", false, "read and verify all encrypted pack data")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	if err := restic.NewService(stderr).Check(ctx, repo, *readData); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "repository check passed")
	return err
}
