package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/vault"
)

func recoveryCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: weazlback recovery <export|verify|prepare|refresh>")
	}
	switch args[0] {
	case "export":
		return recoveryExport(args[1:], stdout, stderr)
	case "verify":
		return recoveryVerify(args[1:], stdout, stderr)
	case "prepare":
		return recoveryPrepare(args[1:], stdout, stderr)
	case "refresh":
		return recoveryRefresh(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown recovery action %q", args[0])
	}
}

func recoveryRefresh(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recovery refresh", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("target", "/mnt/WEAZLBACK-RECOVERY", "existing writable recovery-media directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	resticPath, _ := exec.LookPath("restic")
	err = recovery.RefreshMedia(*target, executable, filepath.Join(filepath.Dir(executable), "weazlback-restore"), resticPath)
	if err == nil {
		_, err = fmt.Fprintf(stdout, "recovery media binaries and checksums refreshed at %s\n", *target)
	}
	return err
}

func recoveryPrepare(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recovery prepare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	target := flags.String("target", "/mnt/WEAZLBACK-RECOVERY", "existing writable recovery-media directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	info, err := os.Stat(*target)
	if err != nil || !info.IsDir() {
		return errors.New("--target must be an existing directory; Weazlback never formats or mounts devices")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	restoreBinary := filepath.Join(filepath.Dir(executable), "weazlback-restore")
	temporaryKit, err := os.CreateTemp(*target, ".weazlback-recovery-*.wzrk")
	if err != nil {
		return err
	}
	temporaryPath := temporaryKit.Name()
	temporaryKit.Close()
	os.Remove(temporaryPath)
	defer os.Remove(temporaryPath)
	if err := recoveryExport([]string{"--output", temporaryPath}, io.Discard, stderr); err != nil {
		return err
	}
	if err := recovery.PrepareMedia(*target, recovery.MediaSources{
		Weazlback: executable, Restore: restoreBinary, Kit: temporaryPath, Restic: toolPath("restic"),
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recovery media prepared and verified at %s\nNO RECOVERY: losing the vault passphrase makes this media useless.\n", *target)
	return err
}

func toolPath(name string) string {
	path, _ := exec.LookPath(name)
	return path
}

func recoveryExport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recovery export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "", "output .wzrk path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("--output is required; choose removable storage")
	}
	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	vaultPath, err := vault.Path(cfg.ActiveVault)
	if err != nil {
		return err
	}
	passphrase, err := readPassphrase(stderr, false)
	if err != nil {
		return err
	}
	defer zero(passphrase)
	v := vault.New(vaultPath)
	if err := v.Unlock(passphrase); err != nil {
		return err
	}
	v.Lock()
	knownHosts := filepath.Join(filepath.Dir(cfgPath), "known_hosts")
	if _, err := os.Stat(knownHosts); errors.Is(err, os.ErrNotExist) {
		knownHosts = ""
	}
	if err := recovery.Export(*output, recovery.Sources{Vault: vaultPath, Config: cfgPath, KnownHosts: knownHosts}, passphrase); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recovery kit written to %s\nNO RECOVERY: losing the vault passphrase makes this kit useless.\n", *output)
	return err
}

func recoveryVerify(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("recovery verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "input .wzrk path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("--input is required")
	}
	passphrase, err := readPassphrase(stderr, false)
	if err != nil {
		return err
	}
	defer zero(passphrase)
	manifest, err := recovery.Verify(*input, passphrase)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recovery kit verified (%d files, created %s)\n%s\n", len(manifest.Files), manifest.CreatedAt.Local().Format("2006-01-02 15:04:05"), manifest.Warning)
	return err
}
