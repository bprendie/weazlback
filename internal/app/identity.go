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

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
)

func identityCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	action := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("identity "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	legacyHost := flags.String("legacy-host", "", "legacy hostname to adopt")
	apply := flags.Bool("apply", false, "apply legacy identity adoption")
	name := flags.String("name", "", "machine display name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if action == "rename" {
		if strings.TrimSpace(*name) == "" {
			return errors.New("--name is required")
		}
		path, err := config.Path()
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		cfg.Machine.Name = strings.TrimSpace(*name)
		if err := config.Save(path, cfg); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "machine %s is now named %s\n", cfg.Machine.ID, cfg.Machine.Name)
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
		return err
	}
	if action == "list" {
		fmt.Fprintf(stdout, "Current machine  %s  %s\n", cfg.Machine.Name, cfg.Machine.ID)
		for _, identity := range restic.GroupIdentities(snapshots, cfg.Machine.ID, cfg.Machine.Name) {
			state := "stable"
			if identity.Legacy {
				state = "legacy / adoption required before prune"
			}
			fmt.Fprintf(stdout, "%-24s %-18s %4d points  %s\n", identity.ID, identity.Hostname, identity.Points, state)
		}
		return nil
	}
	if action != "adopt" {
		return fmt.Errorf("unknown identity action %q", action)
	}
	if *legacyHost == "" {
		return errors.New("--legacy-host is required")
	}
	var selected []string
	for _, snapshot := range snapshots {
		if restic.MachineID(snapshot.Tags) == "" && snapshot.Hostname == *legacyHost {
			selected = append(selected, snapshot.ID)
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no unassigned legacy Restore Points found for %q", *legacyHost)
	}
	fmt.Fprintf(stdout, "%d legacy Restore Points from %s will join %s (%s)\n", len(selected), *legacyHost, cfg.Machine.Name, cfg.Machine.ID)
	if !*apply {
		fmt.Fprintln(stdout, "No repository metadata changed. Re-run with --apply to confirm adoption.")
		return nil
	}
	expected := "ADOPT " + *legacyHost
	fmt.Fprintf(stderr, "Type %s to assign this history: ", expected)
	answer, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if strings.TrimSpace(answer) != expected {
		return errors.New("identity adoption cancelled")
	}
	if err := restic.NewService(stderr).TagMachine(ctx, repo, cfg.Machine.ID, selected); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "adopted %d Restore Points into machine %s\n", len(selected), cfg.Machine.Name)
	return nil
}
