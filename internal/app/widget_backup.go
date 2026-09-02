package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/contracts"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/preflight"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func widgetBackup(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	profileNames := []string{"core", "home"}
	if len(args) == 1 && args[0] == "heavy" {
		profileNames = []string{"heavy"}
	} else if len(args) != 0 {
		return errors.New("widget backup accepts only the optional heavy profile")
	}
	passphrase, err := bufio.NewReader(io.LimitReader(stdin, 64*1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pass := []byte(strings.TrimSuffix(strings.TrimSuffix(passphrase, "\n"), "\r"))
	if len(pass) == 0 {
		return errors.New("Vault passphrase is required")
	}
	defer zero(pass)
	cfg, destination, v, err := loadRuntimeWithPassphrase("", pass)
	if err != nil {
		return err
	}
	defer v.Lock()
	return runUnlockedWidgetBackup(ctx, cfg, destination, v, profileNames, stdout)
}

func runUnlockedWidgetBackup(ctx context.Context, cfg config.Config, destination *config.Destination, v *vault.File, profileNames []string, stdout io.Writer) error {
	if destination.Privileged {
		if destination.Kind != "local" {
			return errors.New("privileged SSH sources require the full Weazlback interface")
		}
		if err := adoptWidgetRepository(cfg, destination); err != nil {
			return err
		}
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return err
	}
	profiles := make([]config.Profile, 0, len(profileNames))
	manifestPath, cleanupManifest, err := backupmeta.PrepareApplications(ctx, "core")
	if err != nil {
		return fmt.Errorf("application manifest: %w", err)
	}
	defer cleanupManifest()
	for _, name := range profileNames {
		profile := findProfile(cfg, name)
		if profile == nil {
			return fmt.Errorf("profile %q is not configured", name)
		}
		prepared := *profile
		prepared.Includes = append([]string(nil), profile.Includes...)
		prepared.Excludes = append([]string(nil), profile.Excludes...)
		if name == "core" || name == "home" {
			home, _ := os.UserHomeDir()
			prepared.Excludes = append(prepared.Excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
		}
		if name == "core" && manifestPath != "" {
			prepared.Includes = append(prepared.Includes, manifestPath)
		}
		if name == "heavy" {
			report := heavy.Inspect(prepared.Includes)
			if !report.Safe {
				return errors.New("Heavy backup refused: stop active VMs or containers and retry")
			}
		}
		report := preflight.Scan(prepared.Includes, prepared.Excludes)
		if report.Unreadable > 0 && !destination.Privileged {
			return fmt.Errorf("%s needs sudo for %d unreadable paths; open Weazlback", strings.ToUpper(name), report.Unreadable)
		}
		profiles = append(profiles, prepared)
	}
	return runWidgetProfiles(ctx, repo, *destination, cfg.Machine.ID, profiles, v, stdout)
}

func adoptWidgetRepository(cfg config.Config, destination *config.Destination) error {
	path := filepath.Clean(destination.Repository)
	home, _ := os.UserHomeDir()
	if !filepath.IsAbs(path) || path == "/" || path == home || len(strings.Split(path, string(os.PathSeparator))) < 3 {
		return errors.New("refusing graphical ownership repair for an unsafe repository path")
	}
	owner := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	command := exec.Command("pkexec", "/usr/bin/chown", "-R", owner, "--", path)
	if err := command.Run(); err != nil {
		return errors.New("administrator authentication was cancelled or repository ownership repair failed")
	}
	destination.Privileged = false
	configPath, err := config.Path()
	if err != nil {
		return err
	}
	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("save repaired destination: %w", err)
	}
	return nil
}

func widgetUnlock(stdin io.Reader, stdout io.Writer) error {
	passphrase, err := bufio.NewReader(io.LimitReader(stdin, 64*1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pass := []byte(strings.TrimSuffix(strings.TrimSuffix(passphrase, "\n"), "\r"))
	if len(pass) == 0 {
		return errors.New("Vault passphrase is required")
	}
	defer zero(pass)
	_, _, v, err := loadRuntimeWithPassphrase("", pass)
	if err != nil {
		return err
	}
	v.Lock()
	_, err = fmt.Fprintln(stdout, "Vault Open")
	return err
}

func runWidgetProfiles(ctx context.Context, repo restic.Repository, destination config.Destination, machineID string, profiles []config.Profile, v *vault.File, stdout io.Writer) error {
	caffeine := acquireCaffeine()
	defer caffeine.Release()
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	previous, _ := store.Load()
	started := time.Now()
	status := contracts.Status{OperationID: operationID(), OperationPID: os.Getpid(), State: "backing-up", Destination: destination.ID,
		LastHealthy: previous.LastHealthy, LastRoutine: previous.LastRoutine, LastReminder: previous.LastReminder, VaultState: "unlocked", Cancellable: true, TravelUntil: previous.TravelUntil,
		Progress: &contracts.Progress{Phase: "discovering"}}
	for _, profile := range profiles {
		status.Profiles = append(status.Profiles, contracts.ProfileProgress{Profile: strings.ToUpper(profile.Name), State: "discovering"})
	}
	_ = store.Save(status)
	var mu sync.Mutex
	var detail lockedBuffer
	var wg sync.WaitGroup
	wireCtx, stopWire := context.WithCancel(ctx)
	defer stopWire()
	go restic.SampleWireRate(wireCtx, restic.NewWireCounter(destination.Repository), func(rate float64) {
		mu.Lock()
		status.Progress.WireBytesPerSecond = rate
		_ = store.Save(status)
		mu.Unlock()
	})
	errs := make([]error, len(profiles))
	for i := range profiles {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			profile := profiles[index]
			laneStarted := time.Now()
			errs[index] = restic.NewService(&detail).BackupMachineWithProgress(ctx, repo, profile.Name, machineID, profile.Includes, profile.Excludes, false, false, func(event restic.BackupProgress) {
				mu.Lock()
				lane := &status.Profiles[index]
				lane.State, lane.Files, lane.Total = "backing-up", event.FilesDone, event.TotalFiles
				lane.Bytes, lane.TotalBytes, lane.Percent = event.BytesDone, event.TotalBytes, event.PercentDone
				elapsed := time.Since(laneStarted).Seconds()
				if event.SecondsElapsed > 0 {
					elapsed = float64(event.SecondsElapsed)
				}
				if elapsed > 0 {
					lane.FilesPerSecond = float64(event.FilesDone) / elapsed
				}
				aggregateWidgetProgress(&status, started)
				_ = store.Save(status)
				mu.Unlock()
			})
			mu.Lock()
			if errs[index] != nil {
				status.Profiles[index].State = "failed"
			} else {
				status.Profiles[index].State, status.Profiles[index].Percent = "complete", 1
			}
			aggregateWidgetProgress(&status, started)
			_ = store.Save(status)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	var completedProfiles []string
	for i, laneErr := range errs {
		if laneErr == nil {
			completedProfiles = append(completedProfiles, profiles[i].Name)
		}
	}
	if len(completedProfiles) > 0 {
		if catalogErr := catalog.Refresh(ctx, v, destination.ID, repo, machineID, completedProfiles...); catalogErr != nil {
			detail.Write([]byte("history catalog update deferred: " + catalogErr.Error() + "\n"))
		}
	}
	_ = writeEncryptedOperationLog(v, status.OperationID, detail.Bytes())
	status.Cancellable, status.VaultState, status.OperationPID = false, "locked", 0
	status.Progress.Phase, status.Progress.Elapsed = "complete", time.Since(started)
	failed := 0
	for _, laneErr := range errs {
		if laneErr != nil {
			failed++
		}
	}
	if failed > 0 {
		status.State, status.Incomplete = "failed", true
		status.Error = fmt.Sprintf("%d of %d backup profiles failed; open Weazlback for encrypted details", failed, len(errs))
	} else {
		now, until := time.Now().UTC(), time.Now().Add(15*time.Second).UTC()
		status.State, status.SuccessUntil, status.Error = "healthy", &until, ""
		if isRoutineProfiles(profiles) {
			status.LastHealthy, status.LastRoutine, status.TravelUntil = &now, &now, nil
		}
	}
	_ = store.Save(status)
	if failed > 0 {
		return errors.New(status.Error)
	}
	labels := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		labels = append(labels, strings.ToUpper(profile.Name))
	}
	_, err = fmt.Fprintf(stdout, "%s backup complete\n", strings.Join(labels, " + "))
	return err
}

func isRoutineProfiles(profiles []config.Profile) bool {
	seen := map[string]bool{}
	for _, profile := range profiles {
		seen[profile.Name] = true
	}
	return seen["core"] && seen["home"]
}

func aggregateWidgetProgress(status *contracts.Status, started time.Time) {
	var chosen *contracts.ProfileProgress
	allComplete := len(status.Profiles) > 0
	for index := range status.Profiles {
		lane := &status.Profiles[index]
		if lane.State != "complete" {
			allComplete = false
		}
		if lane.Profile == "HOME" && lane.TotalBytes > 0 {
			chosen = lane
		} else if chosen == nil && lane.TotalBytes > 0 {
			chosen = lane
		}
	}
	status.Progress.Phase, status.Progress.Elapsed = "backup", time.Since(started)
	if chosen != nil {
		status.Progress.Files, status.Progress.TotalFiles = chosen.Files, chosen.Total
		status.Progress.LogicalBytes = chosen.Bytes
		percent := float64(chosen.Bytes) / float64(chosen.TotalBytes)
		if allComplete {
			percent = 1
		} else if percent > 0.99 {
			percent = 0.99
		}
		if percent > status.Progress.Percent {
			status.Progress.Percent = percent
		}
	} else {
		status.Progress.TotalFiles = 0
		if allComplete {
			status.Progress.Percent = 1
		}
	}
}

func operationID() string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("op-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}
