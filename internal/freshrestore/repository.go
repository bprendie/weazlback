package freshrestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bprendie/weazlback/internal/config"
)

func ClassifyRepositoryError(destination config.Destination, location string, err error) error {
	permission := strings.Contains(strings.ToLower(err.Error()), "permission denied")
	if destination.Kind == "ssh" && permission {
		return fmt.Errorf("SSH repository permission denied for %s; verify the configured remote account owns and can traverse the repository path (Weazlback will not run remote sudo): %w", location, err)
	}
	if destination.Kind != "local" || !permission {
		return fmt.Errorf("repository verification: %w", err)
	}
	info, statErr := os.Stat(location)
	owner := "unknown"
	if statErr == nil {
		owner = ownerText(info)
	}
	return fmt.Errorf("local repository permission denied at %s (owner %s); use --repository if the mount path changed, or --adopt-local-repository to deliberately adopt this exact repository: %w", location, owner, err)
}

func ownerText(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("UID %d / GID %d", stat.Uid, stat.Gid)
	}
	return "unknown"
}

func pathOwner(path string) (uint32, uint32) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Uid, stat.Gid
	}
	return 0, 0
}

func AdoptLocalRepository(ctx context.Context, destination config.Destination, location string) error {
	clean := filepath.Clean(location)
	if destination.Kind != "local" {
		return fmt.Errorf("cannot adopt non-local repository")
	}
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return fmt.Errorf("refusing unsafe repository adoption target %q", location)
	}
	if info, err := os.Stat(clean); err != nil || !info.IsDir() {
		return fmt.Errorf("repository adoption target must be an existing directory: %s", clean)
	}
	probe := exec.CommandContext(ctx, "sudo", "test", "-f", filepath.Join(clean, "config"))
	probe.Stdin, probe.Stdout, probe.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := probe.Run(); err != nil {
		return fmt.Errorf("refusing adoption: %s is not a recognizable Restic repository", clean)
	}
	owner := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	root := exec.CommandContext(ctx, "sudo", "chown", owner, "--", clean, filepath.Join(clean, "config"))
	root.Stdin, root.Stdout, root.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := root.Run(); err != nil {
		return fmt.Errorf("adopt local repository root %s: %w", clean, err)
	}
	for _, name := range []string{"data", "index", "keys", "locks", "snapshots"} {
		path := filepath.Join(clean, name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		cmd := exec.CommandContext(ctx, "sudo", "chown", "-R", owner, "--", path)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("adopt local repository object %s: %w", path, err)
		}
	}
	return nil
}
