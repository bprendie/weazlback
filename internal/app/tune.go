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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"golang.org/x/term"
)

func tuneCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("tune", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	connections := flags.Int("connections", 0, "manual repository connections (1-64; 0 runs automatic trials)")
	uploadMiB := flags.Int("upload-limit-mib", -1, "manual aggregate upload ceiling in MiB/s (0 removes the limit)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *connections < 0 || *connections > 64 {
		return errors.New("--connections must be between 1 and 64, or 0 for automatic tuning")
	}
	if *uploadMiB < -1 {
		return errors.New("--upload-limit-mib must be zero or greater")
	}
	interactive := *connections == 0 && *uploadMiB == -1 && term.IsTerminal(int(os.Stdin.Fd()))
	cfg, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	var tuning restic.ConnectionTuning
	if *connections > 0 {
		destination.Connections = *connections
	} else {
		service := restic.NewService(stderr)
		snapshots, err := service.SnapshotsForMachine(ctx, repo, cfg.Machine.ID)
		if err != nil {
			return err
		}
		sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
		var snapshot *restic.Snapshot
		for i := range snapshots {
			for _, tag := range snapshots[i].Tags {
				if tag == "profile:core" {
					snapshot = &snapshots[i]
					break
				}
			}
			if snapshot != nil {
				break
			}
		}
		if snapshot == nil {
			return errors.New("connection tuning requires a Core Restore Point")
		}
		files, err := service.Files(ctx, repo, snapshot.ID)
		if err != nil {
			return err
		}
		workDir, err := os.MkdirTemp("", "weazlback-tune-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(workDir)
		var spinner *tuneSpinner
		tuning = service.TuneRestoreConnectionsWithProgress(ctx, repo, snapshot.ID, files, workDir, func(count int, active bool) {
			if !interactive {
				return
			}
			if active {
				spinner = startTuneSpinner(stdout, fmt.Sprintf("testing %d connections", count))
			} else {
				spinner.stop()
				spinner = nil
			}
		})
		destination.Connections = tuning.Selected
	}
	var probe restic.UploadProbe
	var probeErr error
	for _, trial := range tuning.Trials {
		fmt.Fprintf(stdout, "%d connections  %s", trial.Connections, trial.Elapsed.Round(1e6))
		if trial.Error != "" {
			fmt.Fprintf(stdout, "  failed: %s", trial.Error)
		}
		fmt.Fprintln(stdout)
	}
	reader := bufio.NewReader(os.Stdin)
	if interactive {
		chosen, err := readTuneValue(reader, stdout, "Choose connections", destination.Connections, 1, 64)
		if err != nil {
			return err
		}
		destination.Connections = chosen
	}
	if interactive && destination.Kind == "ssh" {
		fmt.Fprintln(stdout, "\nBandwidth probe  uploading 100 MiB of ephemeral random data…")
		repo.Connections = destination.Connections
		bar := &tuneBar{writer: stdout}
		probe, probeErr = restic.ProbeSFTPUploadWithProgress(ctx, repo, bar.update)
		if probeErr != nil {
			fmt.Fprintln(stdout)
		}
	}
	if *uploadMiB >= 0 {
		if destination.Kind != "ssh" && *uploadMiB > 0 {
			return errors.New("upload limiting applies only to SSH destinations")
		}
		destination.UploadLimitKiB = *uploadMiB * 1024
	} else if interactive && destination.Kind == "ssh" {
		if probeErr != nil {
			fmt.Fprintf(stderr, "bandwidth probe failed: %v\n", probeErr)
			fmt.Fprintln(stdout, "Bandwidth result unavailable; retaining the existing upload ceiling.")
		} else {
			recommended := restic.RecommendedUploadMiB(probe.MiBPerS)
			fmt.Fprintf(stdout, "Bandwidth result  %.1f MiB/s sustained in %s\n", probe.MiBPerS, probe.Elapsed.Round(time.Millisecond))
			fmt.Fprintf(stdout, "79%% guard         %d MiB/s\n", recommended)
			chosen, err := readTuneValue(reader, stdout, "Choose aggregate upload ceiling in MiB/s (0 unlimited)", recommended, 0, 1_000_000)
			if err != nil {
				return err
			}
			destination.UploadLimitKiB = chosen * 1024
		}
	}
	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	selection := "selected"
	if *connections > 0 {
		selection = "set manual"
	}
	fmt.Fprintf(stdout, "%s %d connections for %s; backup and restore will share it\n", selection, destination.Connections, filepath.Base(destination.Repository))
	if destination.UploadLimitKiB > 0 {
		_, err = fmt.Fprintf(stdout, "upload ceiling %d MiB/s aggregate across all connections\n", destination.UploadLimitKiB/1024)
	} else {
		_, err = fmt.Fprintln(stdout, "upload ceiling unlimited")
	}
	return err
}

func readTuneValue(reader *bufio.Reader, stdout io.Writer, label string, fallback, minimum, maximum int) (int, error) {
	for {
		fmt.Fprintf(stdout, "%s [%d]: ", label, fallback)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			return fallback, nil
		}
		parsed, parseErr := strconv.Atoi(value)
		if parseErr == nil && parsed >= minimum && parsed <= maximum {
			return parsed, nil
		}
		fmt.Fprintf(stdout, "Enter a value from %d to %d.\n", minimum, maximum)
		if errors.Is(err, io.EOF) {
			return 0, errors.New("tuning selection requires interactive input")
		}
	}
}
