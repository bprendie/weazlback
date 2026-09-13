package targetpackages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/packagecapsule"
)

type omaProfileRunner struct {
	t       *testing.T
	profile string
}

func (r omaProfileRunner) Output(name string, args ...string) (string, error) {
	r.t.Helper()
	if name != "pacman-conf" || len(args) != 2 || args[0] != "--config" {
		r.t.Fatalf("unexpected command: %s %v", name, args)
	}
	b, err := os.ReadFile(args[1])
	if err != nil {
		r.t.Fatal(err)
	}
	if !strings.Contains(string(b), "Include = "+filepath.Join(r.profile, "mirrorlist")) || !strings.Contains(string(b), "Architecture = aarch64") {
		r.t.Fatal("host mirrorlist was not isolated")
	}
	return "[options]\nArchitecture = aarch64\nIgnorePkg = hyprland\nHoldPkg = glibc\n[core]\nServer = http://fixture.invalid/aarch64/core\n[omarchy]\nServer = https://fixture.invalid/edge/aarch64\n", nil
}
func (r omaProfileRunner) Run(string, ...string) error {
	r.t.Fatal("import attempted mutation command")
	return nil
}
func (r omaProfileRunner) RunDir(string, string, ...string) error {
	r.t.Fatal("import attempted build")
	return nil
}

func TestOmaSnapImportsOnlineProfileAndPublicKeysOnly(t *testing.T) {
	project := t.TempDir()
	profile := filepath.Join(project, "profiles", "t14s-lcd")
	image := filepath.Join(project, "build", "snapdragon-v0_1_2-installer-root")
	write := func(name, value string) {
		t.Helper()
		path := filepath.Join(project, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("profiles/t14s-lcd/pacman.conf", "[options]\nArchitecture = auto\nIgnorePkg = hyprland\n[core]\nInclude = /etc/pacman.d/mirrorlist\n")
	write("profiles/t14s-lcd/mirrorlist", "Server = http://fixture.invalid/$arch/$repo\n")
	write("build/snapdragon-v0_1_2-installer-root/etc/os-release", "ID=omarchy\nVERSION_ID=4.0.3\n")
	write("build/snapdragon-v0_1_2-installer-root/etc/pacman.conf", "[offline]\nServer = file:///installer-only\n")
	write("build/snapdragon-v0_1_2-installer-root/etc/pacman.d/gnupg/pubring.gpg", "public keys")
	write("build/snapdragon-v0_1_2-installer-root/etc/pacman.d/gnupg/trustdb.gpg", "public trust")
	write("build/snapdragon-v0_1_2-installer-root/etc/pacman.d/gnupg/secring.gpg", "never copy")
	output := t.TempDir()
	if err := ImportOmaSnap(project, output, omaProfileRunner{t, profile}); err != nil {
		t.Fatal(err)
	}
	env, err := LoadEnvironment(output)
	if err != nil {
		t.Fatal(err)
	}
	if env.Architecture != "aarch64" || env.Platform.Version != "4.0.3" || env.Platform.ID != "omarchy" || strings.Contains(env.Config, "file://") || strings.Contains(env.Config, "Architecture =") || !matchesHeld("hyprland", heldPackages(env.Config)) {
		t.Fatalf("%+v", env)
	}
	entries, err := os.ReadDir(filepath.Join(output, "keyring"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("keyring copied unexpected files: %v %v", entries, err)
	}
	if err := os.Remove(filepath.Join(image, "etc/pacman.d/gnupg/trustdb.gpg")); err != nil {
		t.Fatal(err)
	}
	if err := ImportOmaSnap(project, t.TempDir(), omaProfileRunner{t, profile}); err == nil {
		t.Fatal("accepted incomplete trust material")
	}
}

func TestLocalOmaSnapProfileSmoke(t *testing.T) {
	project := os.Getenv("WEAZLBACK_TEST_OMA_SNAP")
	if project == "" {
		t.Skip("set WEAZLBACK_TEST_OMA_SNAP to verify the actual local build")
	}
	output := t.TempDir()
	if err := ImportOmaSnap(project, output, packagecapsule.ExecRunner{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	env, err := LoadEnvironment(output)
	if err != nil {
		t.Fatal(err)
	}
	if env.Architecture != "aarch64" || !strings.Contains(env.Config, "pkgs.omarchy.org/edge/") || !strings.Contains(env.Config, "mirror.archlinuxarm.org/") || strings.Contains(env.Config, "file://") || !matchesHeld("hyprland", heldPackages(env.Config)) {
		t.Fatalf("unexpected local profile: %+v", env)
	}
	t.Logf("Imported ARM64 Omarchy %s online profile with holds and public keys", env.Platform.Version)
}
