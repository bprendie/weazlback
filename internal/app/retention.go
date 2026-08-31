package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
)

func pruneCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	profileName := flags.String("profile", "all", "core, home, heavy, or all")
	apply := flags.Bool("apply", false, "apply the displayed retention plan after exact confirmation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	snapshots, err := restic.NewService(stderr).Snapshots(ctx, repo)
	if err != nil {
		return fmt.Errorf("inspect repository identities: %w", err)
	}
	for _, snapshot := range snapshots {
		if restic.MachineID(snapshot.Tags) == "" {
			return errors.New("retention blocked: legacy Restore Points need explicit machine identity adoption")
		}
	}
	fmt.Fprintf(stdout, "Retention preview\nRepository  %s\nProfile     %s\n", destination.ID, *profileName)
	fmt.Fprintf(stdout, "Machine     %s (%s)\n", cfg.Machine.Name, cfg.Machine.ID)
	if !*apply {
		fmt.Fprintln(stdout, "No data changed. Re-run with --apply to continue to exact confirmation.")
		return nil
	}
	if err := confirmPrune(destination.ID, stderr); err != nil {
		return err
	}
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	service := restic.NewService(stderr)
	profiles := []string{*profileName}
	if *profileName == "all" {
		profiles = []string{"core", "home", "heavy"}
	}
	for _, profile := range profiles {
		if profile != "core" && profile != "home" && profile != "heavy" {
			return fmt.Errorf("invalid retention profile %q", profile)
		}
		retention := cfg.Retention
		if profile == "heavy" {
			retention = cfg.HeavyRetention
		}
		if err := service.PruneMachineProfile(ctx, repo, cfg.Machine.ID, profile, retention.Hourly, retention.Daily, retention.Weekly, retention.Monthly); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(stdout, "retention applied and unused data pruned")
	return err
}

func confirmPrune(destination string, stderr io.Writer) error {
	expected := "PRUNE " + destination
	fmt.Fprintf(stderr, "Type %s to delete unretained repository data: ", expected)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != expected {
		return errors.New("retention cancelled; confirmation did not match")
	}
	return nil
}
