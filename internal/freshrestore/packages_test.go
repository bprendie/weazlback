package freshrestore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/inventory"
)

func TestValidArtifactRejectsChangedPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.pkg.tar.zst")
	payload := []byte("known package")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	manifest := &inventory.ApplicationManifest{AURArtifacts: []inventory.PackageArtifact{{Package: "demo", SHA256: fmt.Sprintf("%x", digest)}}}
	if !validArtifact(manifest, "demo", path) {
		t.Fatal("valid artifact rejected")
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if validArtifact(manifest, "demo", path) {
		t.Fatal("changed artifact accepted")
	}
}

func TestReconcileBatchesEachInstaller(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	for _, name := range []string{"sudo", "paru", "flatpak", "systemctl"} {
		path := filepath.Join(dir, name)
		body := "#!/bin/sh\nprintf '%s %s\\n' '" + name + "' \"$*\" >> \"$COMMAND_LOG\"\n"
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("COMMAND_LOG", logPath)
	plan := Plan{Official: []string{"one", "two"}, AUR: []string{"aur-one", "aur-two"},
		Flatpak: []string{"org.test.App"}, SystemServices: []string{"test.service"}, UserServices: []string{"user.service"}}
	if failures := ReconcileApplications(context.Background(), plan); len(failures) != 0 {
		t.Fatalf("failures=%v", failures)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(body)
	for _, expected := range []string{
		"sudo -n pacman -S --needed --noconfirm -- one two",
		"paru -S --needed --noconfirm --batchinstall -- aur-one aur-two",
		"flatpak install --user --noninteractive org.test.App",
		"sudo -n systemctl enable test.service",
		"systemctl --user enable user.service",
	} {
		if !strings.Contains(log, expected) {
			t.Errorf("missing %q in:\n%s", expected, log)
		}
	}
}

func TestReconcileReportsEnumeratedQueueProgress(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"sudo", "paru"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	plan := Plan{Official: []string{"one", "two"}, AUR: []string{"three"}}
	var events []RestoreProgress
	if failures := ReconcileApplicationsProgress(context.Background(), plan, func(value RestoreProgress) { events = append(events, value) }); len(failures) != 0 {
		t.Fatal(failures)
	}
	last := events[len(events)-1]
	if last.Completed != 3 || last.Total != 3 || last.Current != "three" {
		t.Fatalf("last=%#v", last)
	}
}
