package freshrestore

import (
	"context"
	"fmt"
	"github.com/bprendie/weazlback/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/recovery"
)

func (r *Restore) stageNativeRuntime(stageHome string) error {
	root := filepath.Dir(r.Options.RecoveryPath)
	if _, err := recovery.ValidateBundle(root, ""); err != nil {
		return fmt.Errorf("cross-architecture recovery needs a verified universal kit: %w", err)
	}
	for _, rel := range []string{".weazlback", ".weazlback/bin"} {
		if info, err := os.Lstat(filepath.Join(stageHome, rel)); err == nil && !info.IsDir() {
			return fmt.Errorf("native runtime parent is not a directory: %s", rel)
		}
	}
	native := filepath.Join(root, "linux-"+architecture.Go(r.Plan.Migration.TargetArchitecture))
	for _, name := range []string{"weazlback", "weazlback-restore"} {
		target := filepath.Join(stageHome, ".weazlback", "bin", name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := copyAcrossFilesystems(filepath.Join(native, name), target); err != nil {
			return err
		}
	}
	return nil
}

func (r *Restore) installNativeRuntime(ctx context.Context) error {
	if !r.Plan.Migration.Active || r.Plan.Scope == "applications" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if filepath.Clean(home) != filepath.Clean(r.Plan.TargetHome) {
		return fmt.Errorf("native runtime installation must run as the target home owner")
	}
	root := filepath.Dir(r.Options.RecoveryPath)
	if err := recovery.VerifyMediaDirectory(root); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts", "install.sh"))
	command.Env = append(os.Environ(), "WEAZLBACK_HOME="+filepath.Join(r.Plan.TargetHome, ".weazlback"), "WEAZLBACK_SKIP_LAUNCH=1", "WEAZLBACK_SKIP_RECOVERY_REFRESH=1")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return err
	}
	return r.remapNativeConfig()
}

func (r *Restore) remapNativeConfig() error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	mapped := func(value string) string {
		if value == r.Plan.OriginalHome {
			return r.Plan.TargetHome
		}
		if strings.HasPrefix(value, r.Plan.OriginalHome+"/") {
			return r.Plan.TargetHome + strings.TrimPrefix(value, r.Plan.OriginalHome)
		}
		return value
	}
	for i := range cfg.Profiles {
		for j, value := range cfg.Profiles[i].Includes {
			cfg.Profiles[i].Includes[j] = mapped(value)
		}
		for j, value := range cfg.Profiles[i].Excludes {
			cfg.Profiles[i].Excludes[j] = mapped(value)
		}
	}
	for i := range cfg.Destinations {
		cfg.Destinations[i].SSHKnownHosts = mapped(cfg.Destinations[i].SSHKnownHosts)
		if cfg.Destinations[i].Kind == "local" {
			cfg.Destinations[i].Repository = mapped(cfg.Destinations[i].Repository)
		}
		if r.Options.Repository != "" && cfg.Destinations[i].ID == r.Session.Destination.ID {
			cfg.Destinations[i].Repository = r.Session.Repository.Location
		}
	}
	return config.Save(path, cfg)
}
