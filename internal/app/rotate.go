package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bprendie/weazlback/internal/restic"
)

func rotateCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "repository-key" {
		return errors.New("usage: weazlback rotate repository-key [--destination ID]")
	}
	destinationID, err := destinationFlag("rotate repository-key", args[1:], stderr)
	if err != nil {
		return err
	}
	_, destination, v, err := loadRuntime(destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	defer zero(repo.Password)
	defer zero(repo.SSHKey)
	if repo.Elevated {
		return errors.New("repository key rotation requires user-owned repository access")
	}
	newPassword, err := randomSecret(32)
	if err != nil {
		return err
	}
	defer zero(newPassword)
	service := restic.NewService(stderr)
	keys, err := service.AddPassword(ctx, repo, newPassword)
	if err != nil {
		return fmt.Errorf("add replacement repository key: %w", err)
	}
	oldID := currentKeyID(keys)
	if oldID == "" {
		return errors.New("replacement key added, but current key could not be identified; old key preserved")
	}
	// Persist the usable replacement before removing the old key. A failure after
	// this point remains recoverable with the local vault.
	if err := v.Put(destination.PasswordKey, newPassword); err != nil {
		return fmt.Errorf("replacement key added but vault update failed; old key preserved: %w", err)
	}
	newRepo := repo
	newRepo.Password = newPassword
	if err := service.RemoveKey(ctx, newRepo, oldID); err != nil {
		return fmt.Errorf("replacement key is vaulted, but old key removal failed: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "repository encryption key rotated for %s; refresh every recovery kit now\n", destination.ID)
	return err
}

func currentKeyID(keys []restic.Key) string {
	for _, key := range keys {
		if key.Current {
			return key.ID
		}
	}
	return ""
}
