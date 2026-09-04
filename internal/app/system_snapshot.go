package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/backupmeta"
	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func systemSnapshotCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	action := "create"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("system snapshot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	generationID := flags.String("generation", "", "generation ID")
	level := flags.String("level", "quick", "verification level: quick, sample, or full")
	subset := flags.String("subset", "5%", "sample verification coverage")
	apply := flags.Bool("apply", false, "apply destructive scrub")
	confirm := flags.String("confirm", "", "exact SCRUB confirmation")
	buildAUR := flags.Bool("build-missing-aur", false, "build missing AUR artifacts")
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
	if err := verifyRepositoryIdentity(ctx, &cfg, destination, repo, true); err != nil {
		return err
	}
	switch action {
	case "create", "retry":
		return runSystemGeneration(ctx, cfg, *destination, repo, v, action, *generationID, *buildAUR, stdout, stderr)
	case "list":
		return listSystemGenerations(ctx, repo, stdout)
	case "verify":
		return verifySystemGeneration(ctx, repo, v, destination.RepositoryID, *generationID, *level, *subset, stdout)
	case "scrub":
		return scrubSystemGeneration(ctx, repo, v, destination.RepositoryID, *generationID, *apply, *confirm, stdout)
	default:
		return fmt.Errorf("unknown system snapshot action %q", action)
	}
}

func runSystemGeneration(ctx context.Context, cfg config.Config, destination config.Destination, repo restic.Repository, v *vault.File, action, wanted string, buildAUR bool, stdout, stderr io.Writer) error {
	service := restic.NewService(stderr)
	points, err := service.SnapshotsForMachine(ctx, repo, cfg.Machine.ID)
	if err != nil {
		return err
	}
	id := wanted
	if action == "retry" {
		if id == "" {
			id = latestIncompleteID(generation.Catalog(points), cfg.Machine.ID)
		}
		if id == "" {
			return errors.New("no incomplete System Snapshot to retry")
		}
	} else if id == "" {
		id, err = generation.NewID(time.Now())
		if err != nil {
			return err
		}
	}
	existing := generationMembers(points, id)
	fmt.Fprintf(stdout, "System Snapshot %s\n", id)
	if _, ok := existing["generation-ledger"]; !ok {
		if err := createGenerationLedger(ctx, repo, cfg.Machine.ID, id); err != nil {
			return fmt.Errorf("create generation ledger: %w", err)
		}
	}
	for _, profile := range generation.RequiredProfiles {
		if _, ok := existing[profile]; ok {
			fmt.Fprintf(stdout, "%-10s retained\n", profile)
			continue
		}
		fmt.Fprintf(stdout, "%-10s running\n", profile)
		if err := captureGenerationLane(ctx, cfg, destination, repo, profile, id, buildAUR, stderr); err != nil {
			markGeneration(ctx, service, repo, id, generation.TagFailed)
			_, _ = generation.SaveAudit(v, generation.Audit{GenerationID: id, RepositoryID: destination.RepositoryID, Action: "capture", Result: "failed", Details: []string{profile + ": " + err.Error()}})
			return fmt.Errorf("System Snapshot %s incomplete at %s: %w", id, profile, err)
		}
	}
	points, err = service.SnapshotsForMachine(ctx, repo, cfg.Machine.ID)
	if err != nil {
		return err
	}
	members := generationMembers(points, id)
	if !generation.HasAll(generation.Generation{Members: members}, generation.RequiredProfiles) {
		return errors.New("generation validation failed: required lanes are missing")
	}
	ids := memberIDs(members)
	if err := service.TagSnapshots(ctx, repo, ids, []string{generation.TagComplete}, []string{generation.TagFailed, generation.TagAbandoned}); err != nil {
		return err
	}
	_, err = generation.SaveAudit(v, generation.Audit{GenerationID: id, RepositoryID: destination.RepositoryID, Action: "capture", Result: "complete", Details: ids})
	if err == nil {
		fmt.Fprintf(stdout, "System Snapshot %s COMPLETE\n", id)
	}
	return err
}

func createGenerationLedger(ctx context.Context, repo restic.Repository, machineID, id string) error {
	dir, err := os.MkdirTemp("", "weazlback-generation-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	body := []byte(fmt.Sprintf("{\"schema_version\":1,\"generation_id\":%q,\"started_at\":%q}\n", id, time.Now().UTC().Format(time.RFC3339Nano)))
	if err := os.WriteFile(filepath.Join(dir, "generation.json"), body, 0o600); err != nil {
		return err
	}
	return restic.NewService(io.Discard).BackupMachineTaggedWithProgress(ctx, repo, "generation-ledger", machineID, []string{generation.TagPrefix + id}, []string{dir}, nil, false, false, nil)
}

func captureGenerationLane(ctx context.Context, cfg config.Config, destination config.Destination, repo restic.Repository, profile, id string, buildAUR bool, stderr io.Writer) error {
	if profile == "packages" {
		return captureGenerationPackages(ctx, cfg, repo, id, buildAUR, stderr)
	}
	p := findProfile(cfg, profile)
	if p == nil {
		return fmt.Errorf("profile %q is not configured", profile)
	}
	if profile == "heavy" {
		report := heavy.Inspect(p.Includes)
		if !report.Safe {
			return errors.New("Heavy contains writable open files; stop workloads and retry")
		}
	}
	manifest, cleanup, err := backupmeta.PrepareApplications(ctx, profile)
	if err != nil {
		return err
	}
	defer cleanup()
	includes := append([]string(nil), p.Includes...)
	if manifest != "" {
		includes = append(includes, manifest)
	}
	excludes := append([]string(nil), p.Excludes...)
	if profile == "core" || profile == "home" {
		home, _ := os.UserHomeDir()
		excludes = append(excludes, browserrepair.Exclusions(browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})...)
	}
	return restic.NewService(stderr).BackupMachineTaggedWithProgress(ctx, repo, profile, cfg.Machine.ID, []string{generation.TagPrefix + id}, includes, excludes, false, false, nil)
}

func captureGenerationPackages(ctx context.Context, cfg config.Config, repo restic.Repository, id string, buildAUR bool, stderr io.Writer) error {
	if err := authorizePackageCapture(io.Discard, stderr); err != nil {
		return err
	}
	staging, err := packageStagingRoot()
	if err != nil {
		return err
	}
	_, root, cleanup, err := packagecapsule.Capture(packagecapsule.Options{Context: ctx, MachineID: cfg.Machine.ID, StagingRoot: staging, Download: true, BuildMissingAUR: buildAUR, Run: packagecapsule.ExecRunner{Context: ctx}})
	if err != nil {
		return err
	}
	defer cleanup()
	return restic.NewService(stderr).BackupMachineTaggedWithProgress(ctx, repo, "packages", cfg.Machine.ID, []string{generation.TagPrefix + id}, []string{root}, nil, false, false, nil)
}

func generationMembers(points []restic.Snapshot, id string) map[string]restic.Snapshot {
	result := map[string]restic.Snapshot{}
	for _, point := range points {
		if generation.ID(point.Tags) == id {
			profile := restic.Profile(point.Tags)
			if old, ok := result[profile]; !ok || point.Time.After(old.Time) {
				result[profile] = point
			}
		}
	}
	return result
}

func memberIDs(members map[string]restic.Snapshot) []string {
	result := make([]string, 0, len(members))
	for _, point := range members {
		result = append(result, point.ID)
	}
	sort.Strings(result)
	return result
}

func markGeneration(ctx context.Context, service restic.Service, repo restic.Repository, id, tag string) {
	points, _ := service.Snapshots(ctx, repo)
	members := generationMembers(points, id)
	if len(members) > 0 {
		_ = service.TagSnapshots(ctx, repo, memberIDs(members), []string{tag}, nil)
	}
}

func latestIncompleteID(gens []generation.Generation, machine string) string {
	for _, g := range gens {
		if g.MachineID == machine && !g.Complete && !g.Abandoned {
			return g.ID
		}
	}
	return ""
}
