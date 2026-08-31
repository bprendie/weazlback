package benchmark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

type engine interface {
	name() string
	version(context.Context) (string, error)
	init(context.Context, string) error
	backup(context.Context, string, string, string) error
	restore(context.Context, string, string) error
}

func selectEngine(options Options) (engine, error) {
	sshArgs := ""
	if options.SSHKey != "" {
		sshArgs = "-i " + options.SSHKey + " -o IdentitiesOnly=yes"
	}
	if options.KnownHosts != "" {
		sshArgs += " -o UserKnownHostsFile=" + options.KnownHosts + " -o StrictHostKeyChecking=yes"
	}
	switch options.Engine {
	case "borg":
		return borgEngine{sshArgs: sshArgs}, nil
	case "restic":
		return resticEngine{sshArgs: sshArgs}, nil
	default:
		return nil, fmt.Errorf("engine must be borg or restic")
	}
}

type borgEngine struct{ sshArgs string }

func (borgEngine) name() string { return "borg" }
func (borgEngine) version(ctx context.Context) (string, error) {
	return commandOutput(ctx, nil, "borg", "--version")
}
func (e borgEngine) init(ctx context.Context, repo string) error {
	return command(ctx, e.env(), "borg", "init", "--encryption=repokey-blake2", repo)
}
func (e borgEngine) backup(ctx context.Context, repo, source, archive string) error {
	return command(ctx, e.env(), "borg", "create", "--compression=lz4", repo+"::"+archive, source)
}
func (e borgEngine) restore(ctx context.Context, repo, target string) error {
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	return commandDir(ctx, e.env(), target, "borg", "extract", repo+"::changed")
}

func (e borgEngine) env() []string {
	env := benchmarkEnv("BORG_PASSPHRASE")
	if e.sshArgs != "" {
		env = append(env, "BORG_RSH=ssh "+e.sshArgs)
	}
	return env
}

type resticEngine struct{ sshArgs string }

func (resticEngine) name() string { return "restic" }
func (resticEngine) version(ctx context.Context) (string, error) {
	return commandOutput(ctx, nil, "restic", "version")
}
func (e resticEngine) init(ctx context.Context, repo string) error {
	return command(ctx, benchmarkEnv("RESTIC_PASSWORD"), "restic", e.args(repo, "init")...)
}
func (e resticEngine) backup(ctx context.Context, repo, source, _ string) error {
	return command(ctx, benchmarkEnv("RESTIC_PASSWORD"), "restic", e.args(repo, "backup", "--no-scan", source)...)
}
func (e resticEngine) restore(ctx context.Context, repo, target string) error {
	return command(ctx, benchmarkEnv("RESTIC_PASSWORD"), "restic", e.args(repo, "restore", "latest", "--target", target, "--sparse")...)
}

func (e resticEngine) args(repo string, operation ...string) []string {
	args := []string{"-r", repo}
	if e.sshArgs != "" {
		args = append(args, "-o", "sftp.args="+e.sshArgs)
	}
	return append(args, operation...)
}

func benchmarkEnv(key string) []string {
	return append(os.Environ(), key+"=weazlback-phase-zero-benchmark-only")
}

func command(ctx context.Context, env []string, name string, args ...string) error {
	return commandDir(ctx, env, "", name, args...)
}

func commandDir(ctx context.Context, env []string, dir, name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return errorsForTool(name)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func commandOutput(ctx context.Context, env []string, name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", errorsForTool(name)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func treeBytes(root string) (logical, allocated int64) {
	_ = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		logical += info.Size()
		allocated += info.Size()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			allocated += stat.Blocks*512 - info.Size()
		}
		return nil
	})
	return logical, allocated
}
