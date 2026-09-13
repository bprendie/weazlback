package app

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/targetpackages"
)

func architecturePackages(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return architecturePackageMenu(ctx, stdout, stderr)
	}
	action := args[0]
	flags := flag.NewFlagSet("packages architectures "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("target-dir", "", "exported target environment directory (repositories and public keyring)")
	source := flags.String("source", "", "source application manifest JSON")
	snapshot := flags.String("snapshot", "", "source Core snapshot ID (or latest)")
	input := flags.String("input", "", "prepared capsule directory to verify/import")
	output := flags.String("output", "", "output directory or source inventory JSON")
	machine := flags.String("machine", "", "source machine identity; defaults to current configured identity")
	destinationID := flags.String("destination", "", "repository destination")
	build := flags.Bool("build-missing-aur", false, "explicitly execute AUR builds on a matching native target")
	inspectOnly := flags.Bool("dry-run", false, "prepare locally without saving to repository")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	run := packagecapsule.ExecRunner{Context: ctx}
	if action == "export-target" {
		if *output == "" {
			return fmt.Errorf("--output directory is required")
		}
		return targetpackages.ExportEnvironment(*output, run)
	}
	if action == "export-source" {
		if *output == "" {
			return fmt.Errorf("--output JSON is required")
		}
		m, err := inventory.CaptureApplications(ctx)
		if err != nil {
			return err
		}
		return inventory.WriteApplications(*output, m)
	}
	if *target == "" {
		return fmt.Errorf("--target-dir is required; export it once on your target PC")
	}
	env, err := targetpackages.LoadEnvironment(*target)
	if err != nil {
		return err
	}
	isolated := targetpackages.IsolatedRunner{Base: run, Root: *target, Environment: env}
	if action == "verify" || action == "import" {
		m, err := packagecapsule.Load(*input)
		if err != nil {
			return err
		}
		if m.TargetFingerprint != env.Fingerprint || !architecture.Compatible(m.Architecture, env.Architecture) {
			return fmt.Errorf("capsule belongs to a different target environment")
		}
		if _, err := packagecapsule.VerifyArtifactsFor(*input, m, isolated, env.Architecture); err != nil {
			return err
		}
		if action == "verify" {
			return writeJSON(stdout, m)
		}
		return storeArchitectureCapsule(ctx, *destinationID, *input, m, stdout, stderr)
	}
	var manifest inventory.ApplicationManifest
	if strings.HasPrefix(*source, "snapshot:") {
		*snapshot = strings.TrimPrefix(*source, "snapshot:")
	}
	if *source == "current" {
		manifest, err = inventory.CaptureApplications(ctx)
	} else if *snapshot != "" {
		manifest, err = architectureSourceSnapshot(ctx, *destinationID, *snapshot, *machine, stderr)
	} else {
		if *source == "" {
			return fmt.Errorf("--source JSON or --snapshot is required")
		}
		var b []byte
		b, err = os.ReadFile(*source)
		if err == nil {
			err = json.Unmarshal(b, &manifest)
		}
	}
	if err != nil {
		return err
	}
	if err := inventory.ValidateApplications(manifest); err != nil {
		return err
	}
	if action == "inspect" {
		if err := isolated.Initialize(); err != nil {
			return err
		}
		return writeJSON(stdout, targetpackages.ResolveEnvironment(manifest, env, isolated))
	}
	if action != "prepare" {
		return fmt.Errorf("unknown architecture package action %q", action)
	}
	if *machine == "" {
		*machine = manifest.MachineID
	}
	if *machine == "" {
		return fmt.Errorf("--machine must identify the source machine, so prepared packages remain associated with its snapshots")
	}
	stage, err := packageStagingRoot()
	if err != nil {
		return err
	}
	m, root, cleanup, err := targetpackages.Prepare(ctx, *target, stage, *machine, manifest, true, *build, func(p packagecapsule.Progress) {
		fmt.Fprintf(stderr, "%s %d/%d %s\n", p.Phase, p.Completed, p.Total, p.Package)
	})
	if err != nil {
		return err
	}
	defer cleanup()
	if *output != "" {
		if _, err := os.Lstat(*output); !os.IsNotExist(err) {
			return fmt.Errorf("output must be a new directory")
		}
		if err := run.Run("cp", "-a", "--", root, *output); err != nil {
			return err
		}
	}
	if !*inspectOnly {
		if err := storeArchitectureCapsule(ctx, *destinationID, root, m, stdout, stderr); err != nil {
			return err
		}
	}
	return writeJSON(stdout, m)
}

func storeArchitectureCapsule(ctx context.Context, destinationID, root string, m packagecapsule.Manifest, stdout, stderr io.Writer) error {
	cfg, destination, v, err := loadRuntime(destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	if m.MachineID == "" {
		return fmt.Errorf("prepared capsule is missing source machine identity")
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return err
	}
	profile := "packages-" + architecture.Go(m.Architecture)
	if err := restic.NewService(stderr).BackupMachineWithProgress(ctx, repo, profile, m.MachineID, []string{root}, nil, false, false, nil); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Saved independent %s capsule for source machine %s. Native System Snapshot policy unchanged.\n", m.Architecture, m.MachineID)
	return err
}

func architectureSourceSnapshot(ctx context.Context, destinationID, selected, machine string, stderr io.Writer) (inventory.ApplicationManifest, error) {
	var m inventory.ApplicationManifest
	cfg, destination, v, err := loadRuntime(destinationID, stderr)
	if err != nil {
		return m, err
	}
	defer v.Lock()
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return m, err
	}
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return m, err
	}
	if machine == "" {
		machine = cfg.Machine.ID
	}
	service := restic.NewService(stderr)
	points, err := service.SnapshotsForMachine(ctx, repo, machine)
	if err != nil {
		return m, err
	}
	var chosen *restic.Snapshot
	for _, point := range points {
		if restic.Profile(point.Tags) != "core" {
			continue
		}
		if selected != "latest" && selected != point.ID && selected != point.ShortID {
			continue
		}
		if chosen == nil || point.Time.After(chosen.Time) {
			copy := point
			chosen = &copy
		}
	}
	if chosen == nil {
		return m, fmt.Errorf("no matching source Core snapshot")
	}
	files, err := service.Files(ctx, repo, chosen.ID)
	if err != nil {
		return m, err
	}
	for _, file := range files {
		if filepath.Base(file.Path) == backupmeta.ManifestName {
			b, err := service.Dump(ctx, repo, chosen.ID, file.Path)
			if err != nil {
				return m, err
			}
			err = json.Unmarshal(b, &m)
			m.MachineID = machine
			return m, err
		}
	}
	return m, fmt.Errorf("snapshot has no application inventory")
}

func architecturePackageMenu(ctx context.Context, stdout, stderr io.Writer) error {
	reader := bufio.NewReader(os.Stdin)
	prompt := func(label string) (string, error) {
		fmt.Fprint(stdout, label+": ")
		value, err := reader.ReadString('\n')
		return strings.TrimSpace(value), err
	}
	fmt.Fprintln(stdout, "OTHER ARCHITECTURES / ARM64\nIndependent package preparation; System Snapshots never run this workflow.\n1 Export target environment (run on Snapdragon PC)\n2 Export current source application inventory\n3 Inspect ARM availability\n4 Download, verify, and save ARM capsule\n5 Explicit native AUR build + save\n6 Verify prepared capsule\n7 Import prepared capsule\nq Return")
	choice, err := prompt("Action")
	if err != nil {
		return err
	}
	actions := map[string]string{"1": "export-target", "2": "export-source", "3": "inspect", "4": "prepare", "5": "prepare", "6": "verify", "7": "import"}
	action := actions[choice]
	if action == "" {
		return nil
	}
	args := []string{action}
	ask := func(flag, label string) error {
		value, err := prompt(label)
		if err == nil {
			args = append(args, flag, value)
		}
		return err
	}
	if choice == "1" || choice == "2" {
		if err := ask("--output", "Output path"); err != nil {
			return err
		}
	} else {
		if err := ask("--target-dir", "Target environment directory exported from Snapdragon PC"); err != nil {
			return err
		}
		if choice == "6" || choice == "7" {
			if err := ask("--input", "Prepared capsule directory"); err != nil {
				return err
			}
		} else {
			if err := ask("--source", "Source inventory: current, snapshot:ID, or JSON path"); err != nil {
				return err
			}
			if action == "prepare" {
				if err := ask("--machine", "Source machine ID (Enter uses exported inventory identity)"); err != nil {
					return err
				}
			}
		}
	}
	if choice == "5" {
		fmt.Fprintln(stdout, "This executes AUR PKGBUILDs and may install build dependencies on the native ARM host.")
		answer, err := prompt("Type BUILD to continue")
		if err != nil {
			return err
		}
		if answer != "BUILD" {
			return nil
		}
		args = append(args, "--build-missing-aur")
	}
	err = architecturePackages(ctx, args, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
	}
	_, _ = prompt("Press Enter to return")
	return err
}
