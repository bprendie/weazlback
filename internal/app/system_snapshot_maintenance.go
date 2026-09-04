package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func listSystemGenerations(ctx context.Context, repo restic.Repository, stdout io.Writer) error {
	points, err := restic.NewService(io.Discard).Snapshots(ctx, repo)
	if err != nil {
		return err
	}
	gens := generation.Catalog(points)
	for index, g := range gens {
		state := "INCOMPLETE"
		if g.Complete {
			state = "COMPLETE"
		}
		if g.Failed {
			state = "FAILED"
		}
		if g.Abandoned {
			state = "ABANDONED"
		}
		latest := ""
		if index == 0 && g.Complete {
			latest = "  LATEST COMPLETE"
		}
		fmt.Fprintf(stdout, "%s  %-10s  %s → %s  %d/%d lanes%s\n", g.StartedAt.Local().Format("2006-01-02 15:04"), state, g.ID, g.EndedAt.Local().Format("15:04"), len(g.Members), len(generation.RequiredProfiles), latest)
	}
	if len(gens) == 0 {
		fmt.Fprintln(stdout, "No System Snapshots found")
	}
	return nil
}

func verifySystemGeneration(ctx context.Context, repo restic.Repository, v *vault.File, repositoryID, wanted, level, subset string, stdout io.Writer) error {
	service := restic.NewService(stdout)
	points, err := service.Snapshots(ctx, repo)
	if err != nil {
		return err
	}
	g, err := selectGeneration(generation.Catalog(points), wanted, true)
	if err != nil {
		return err
	}
	if !generation.HasAll(g, generation.RequiredProfiles) {
		return errors.New("generation membership validation failed")
	}
	switch level {
	case "quick":
		err = service.Check(ctx, repo, false)
	case "sample":
		err = service.CheckSubset(ctx, repo, subset)
	case "full":
		err = service.Check(ctx, repo, true)
	default:
		return fmt.Errorf("unknown verification level %q", level)
	}
	result := "verified"
	if err != nil {
		result = "failed"
	}
	_, auditErr := generation.SaveAudit(v, generation.Audit{GenerationID: g.ID, RepositoryID: repositoryID, Action: "verify", Level: level, Result: result, Details: []string{"coverage=" + subset}})
	if err != nil {
		return err
	}
	if auditErr != nil {
		return auditErr
	}
	fmt.Fprintf(stdout, "%s  %s VERIFIED  %s\n", g.StartedAt.Local().Format("2006-01-02 15:04"), strings.ToUpper(level), g.ID)
	return nil
}

func scrubSystemGeneration(ctx context.Context, repo restic.Repository, v *vault.File, repositoryID, wanted string, apply bool, confirm string, stdout io.Writer) error {
	service := restic.NewService(stdout)
	points, err := service.Snapshots(ctx, repo)
	if err != nil {
		return err
	}
	g, err := selectGeneration(generation.Catalog(points), wanted, false)
	if err != nil {
		return err
	}
	if g.Complete && !g.Failed && !g.Abandoned {
		return errors.New("healthy complete generations cannot be scrubbed")
	}
	ids := memberIDs(g.Members)
	phrase := "SCRUB " + g.StartedAt.Local().Format("2006-01-02 15:04")
	fmt.Fprintf(stdout, "System Snapshot %s  %s\nState: incomplete/failed  Restore Points: %d\nRequired confirmation: %s\n", g.ID, g.StartedAt.Local().Format(time.RFC3339), len(ids), phrase)
	if !apply {
		fmt.Fprintln(stdout, "Preview only; no repository data changed.")
		return nil
	}
	if confirm != phrase {
		return errors.New("scrub confirmation did not match exact generation date/time")
	}
	if len(ids) == 0 {
		return errors.New("generation has no Restore Points to scrub")
	}
	if err := service.Check(ctx, repo, false); err != nil {
		return fmt.Errorf("pre-scrub check: %w", err)
	}
	if err := service.ForgetSnapshots(ctx, repo, ids); err != nil {
		return err
	}
	if err := service.PruneUnreferenced(ctx, repo); err != nil {
		return err
	}
	if err := service.Check(ctx, repo, false); err != nil {
		return fmt.Errorf("post-scrub check: %w", err)
	}
	_, err = generation.SaveAudit(v, generation.Audit{GenerationID: g.ID, RepositoryID: repositoryID, Action: "scrub", Result: "complete", Details: ids})
	if err == nil {
		fmt.Fprintln(stdout, "Scrub complete; unreferenced encrypted data pruned and repository rechecked.")
	}
	return err
}

func selectGeneration(gens []generation.Generation, wanted string, requireComplete bool) (generation.Generation, error) {
	sort.Slice(gens, func(i, j int) bool { return gens[i].EndedAt.After(gens[j].EndedAt) })
	for _, g := range gens {
		if wanted != "" && g.ID != wanted {
			continue
		}
		if requireComplete && !g.Complete {
			continue
		}
		if !requireComplete && g.Complete && !g.Failed && !g.Abandoned {
			continue
		}
		return g, nil
	}
	if wanted != "" {
		return generation.Generation{}, fmt.Errorf("eligible System Snapshot %q not found", wanted)
	}
	return generation.Generation{}, errors.New("no eligible System Snapshot found")
}
