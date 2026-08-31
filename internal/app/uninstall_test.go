package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUninstallPreservesUserMaterial(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	config := filepath.Join(root, "config")
	bin := filepath.Join(root, "fake-bin")
	for _, dir := range []string{filepath.Join(home, ".weazlback", "vaults"), filepath.Join(config, "systemd", "user"), bin} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"systemctl", "omarchy"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(home, ".weazlback", "vaults", "keep")
	if err := os.WriteFile(marker, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"weazlback-backup.timer", "weazlback-backup.service"} {
		if err := os.WriteFile(filepath.Join(config, "systemd", "user", name), []byte("unit"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("bash", "../../scripts/uninstall.sh")
	command.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+config, "PATH="+bin+":"+os.Getenv("PATH"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("uninstall: %v: %s", err, output)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("vault material was not preserved: %v", err)
	}
	for _, name := range []string{"weazlback-backup.timer", "weazlback-backup.service"} {
		if _, err := os.Stat(filepath.Join(config, "systemd", "user", name)); !os.IsNotExist(err) {
			t.Fatalf("unit %s was not removed", name)
		}
	}
}
