package restic

import (
	"context"
	"errors"
	"strings"
)

func (s Service) CheckSubset(ctx context.Context, repo Repository, subset string) error {
	if strings.TrimSpace(subset) == "" {
		return errors.New("read-data subset is required")
	}
	_, err := s.Runner.Run(ctx, repo, "check", "--read-data-subset", subset)
	return err
}

func (s Service) TagSnapshots(ctx context.Context, repo Repository, snapshotIDs, add, remove []string) error {
	if len(snapshotIDs) == 0 {
		return errors.New("Restore Point IDs are required")
	}
	args := []string{"tag"}
	for _, tag := range add {
		args = append(args, "--add", tag)
	}
	for _, tag := range remove {
		args = append(args, "--remove", tag)
	}
	args = append(args, snapshotIDs...)
	_, err := s.Runner.Run(ctx, repo, args...)
	return err
}

func (s Service) ForgetSnapshots(ctx context.Context, repo Repository, snapshotIDs []string) error {
	if len(snapshotIDs) == 0 {
		return errors.New("Restore Point IDs are required")
	}
	_, err := s.Runner.Run(ctx, repo, append([]string{"forget"}, snapshotIDs...)...)
	return err
}

func (s Service) PruneUnreferenced(ctx context.Context, repo Repository) error {
	_, err := s.Runner.Run(ctx, repo, "prune")
	return err
}
