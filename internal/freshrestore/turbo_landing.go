package freshrestore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bridge "github.com/restic/restic/weazlbridge"
)

// turboLandingEngine retains official Restic as the authenticated materializer.
// On Btrfs it stages into a private, same-filesystem subvolume so final placement
// can remain atomic. Pack-first repository reading belongs to H6/H7.
type turboLandingEngine struct{}

func (turboLandingEngine) Name() string { return EngineTurbo }

func (turboLandingEngine) Stage(ctx context.Context, restore *Restore) error {
	q := restore.Journal.Qualification
	if !q.Eligible {
		return fmt.Errorf("Turbo qualification failed")
	}
	if q.TargetFilesystem != "btrfs" {
		return fmt.Errorf("Btrfs fast landing unavailable")
	}
	parent := filepath.Join(restore.Plan.TargetHome, ".weazlback-recovery")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage := filepath.Join(parent, filepath.Base(restore.StageDir))
	if !pathWithin(stage, parent) {
		return fmt.Errorf("unsafe Turbo staging path")
	}
	if err := deleteBtrfsSubvolume(stage); err != nil {
		return fmt.Errorf("clear prior Turbo stage: %w", err)
	}
	out, err := exec.CommandContext(ctx, "btrfs", "subvolume", "create", stage).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create Btrfs staging subvolume: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err = exec.CommandContext(ctx, "btrfs", "property", "set", stage, "compression", "none").CombinedOutput(); err != nil {
		_ = deleteBtrfsSubvolume(stage)
		return fmt.Errorf("disable staging compression: %w: %s", err, strings.TrimSpace(string(out)))
	}
	standardStage := restore.StageDir
	restore.StageDir = stage
	if err := restore.restoreSelectionTurbo(ctx, stage, false); err != nil {
		restore.StageDir = standardStage
		_ = deleteBtrfsSubvolume(stage)
		return err
	}
	if err := restore.validateStage(); err != nil {
		restore.StageDir = standardStage
		_ = deleteBtrfsSubvolume(stage)
		return err
	}
	return nil
}

func (r *Restore) restoreSelectionTurbo(ctx context.Context, target string, includeHeavy bool) error {
	if r.Plan.HomeSnapshot != nil {
		if err := r.restoreEmbeddedPoint(ctx, "Home", r.Plan.HomeSnapshot.ID, target); err != nil {
			return err
		}
	}
	if includeHeavy && r.Plan.HeavySnapshot != nil {
		if err := r.restoreEmbeddedPoint(ctx, "Heavy", r.Plan.HeavySnapshot.ID, target); err != nil {
			return err
		}
	}
	return r.restoreEmbeddedPoint(ctx, "Core overlay", r.Plan.Snapshot.ID, target)
}

func (r *Restore) restoreEmbeddedPoint(ctx context.Context, label, snapshot, target string) error {
	sshArgs, err := r.embeddedSSHArgs()
	if err != nil {
		return err
	}
	started := time.Now()
	downloadLimit := r.Session.Repository.UploadLimitKiB
	if r.Options.TurboPolicy.FullLink {
		downloadLimit = 0
	}
	options := bridge.Options{Repository: r.Session.Repository.Location, Password: string(r.Session.Repository.Password),
		Snapshot: snapshot, Target: target, SSHArgs: sshArgs, Connections: r.Session.Repository.Connections,
		DownloadLimitKiB: downloadLimit,
		Progress: func(value bridge.Progress) {
			elapsed := time.Since(started).Seconds()
			progress := RestoreProgress{Phase: "filesystem", Lane: label, Current: "authenticated pack restore",
				Completed: int(value.FilesDone), Total: int(value.FilesTotal), BytesDone: value.BytesDone, BytesTotal: value.BytesTotal, WireBytes: value.WireBytes}
			if elapsed > 0 {
				progress.BytesPerSecond = float64(value.BytesDone) / elapsed
				progress.WireBytesPerSecond = float64(value.WireBytes) / elapsed
				progress.FilesPerSecond = float64(value.FilesDone) / elapsed
			}
			emitProgress(r.Options.Progress, progress)
		}}
	_, err = bridge.Restore(ctx, options)
	if err != nil && repositoryLockError(err) {
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "filesystem", Lane: label, Current: "clearing stale repository lock"})
		if unlockErr := r.service.UnlockStale(ctx, r.Session.Repository); unlockErr != nil {
			return fmt.Errorf("Turbo repository lock: %w; safe stale-lock cleanup failed: %v", err, unlockErr)
		}
		_, err = bridge.Restore(ctx, options)
	}
	return err
}

func repositoryLockError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "repository is already locked") ||
		strings.Contains(message, "unable to create lock in backend")
}

func (r *Restore) embeddedSSHArgs() (string, error) {
	repo := r.Session.Repository
	if len(repo.SSHKey) == 0 {
		return "", nil
	}
	keyPath := filepath.Join(r.Session.PrivateDir, "turbo_identity")
	if err := os.WriteFile(keyPath, repo.SSHKey, 0o600); err != nil {
		return "", err
	}
	args := "-F /dev/null -oBatchMode=yes -oIdentitiesOnly=yes -i " + keyPath
	if repo.KnownHosts != "" {
		args += " -oStrictHostKeyChecking=yes -oUserKnownHostsFile=" + repo.KnownHosts
	}
	return args, nil
}

func deleteBtrfsSubvolume(path string) error {
	if _, err := os.Lstat(path); err != nil {
		return nil
	}
	if err := exec.Command("btrfs", "subvolume", "delete", path).Run(); err == nil {
		return nil
	}
	// Unprivileged Btrfs owners may create subvolumes yet lack the ioctl search
	// permission needed by `btrfs subvolume delete`. Removing the emptied tree via
	// rmdir is supported and retains the exact same explicit-path boundary.
	return os.RemoveAll(path)
}

func (r *Restore) cleanupTurboStage() error {
	if r.Journal.RequestedEngine != EngineTurbo {
		return nil
	}
	root := filepath.Join(r.Plan.TargetHome, ".weazlback-recovery")
	if !pathWithin(r.StageDir, root) {
		return nil
	}
	if err := deleteBtrfsSubvolume(r.StageDir); err != nil {
		return err
	}
	_ = os.Remove(root)
	return nil
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
