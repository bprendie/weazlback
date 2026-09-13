package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/targetpackages"
	tea "github.com/charmbracelet/bubbletea"
)

func armEnvironmentFixture(t *testing.T, arch string) string {
	t.Helper()
	root := t.TempDir()
	env := targetpackages.Environment{Architecture: arch, Config: "[options]\n[core]\nServer = https://fixture.invalid/$arch\n"}
	digest := sha256.Sum256([]byte(env.Config + "\n\n\n\n" + arch))
	env.Fingerprint = hex.EncodeToString(digest[:])
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "keyring"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pubring.gpg", "trustdb.gpg"} {
		if err := os.WriteFile(filepath.Join(root, "keyring", name), []byte("public fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestARMSetupRemembersPrivateCopyAndRejectsX64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("WEAZLBACK_CONFIG", path)
	cfg := config.Default()
	cfg.ActiveDestination = "default-repo"
	source := armEnvironmentFixture(t, "aarch64")
	if err := rememberARMTarget(source, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ARMTargetEnvironment == source {
		t.Fatal("retained removable source path")
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if err := validateARMTarget(cfg.ARMTargetEnvironment); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil || saved.ARMTargetEnvironment != cfg.ARMTargetEnvironment || saved.ActiveDestination != "default-repo" {
		t.Fatalf("saved config: %+v %v", saved, err)
	}
	old := cfg.ARMTargetEnvironment
	if err := rememberARMTarget(armEnvironmentFixture(t, "x86_64"), &cfg); err == nil {
		t.Fatal("accepted x64 settings")
	}
	if cfg.ARMTargetEnvironment != old {
		t.Fatal("failed import changed remembered target")
	}
}

func TestARMXUsesInAppSetupAndConsumesPathKeys(t *testing.T) {
	m := Model{cfg: config.Default(), mode: modeProfiles, styles: newStyles()}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = updated.(Model)
	if m.armStage != "setup" || !strings.Contains(m.applicationsScreen(), "ONE-TIME SNAPDRAGON SETUP") {
		t.Fatal("X did not open in-app setup")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/mnt/snapdragon-target")})
	m = updated.(Model)
	if m.armInput.Value() != "/mnt/snapdragon-target" || m.mode != modeProfiles {
		t.Fatal("path input triggered navigation")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).armStage != "" {
		t.Fatal("escape did not leave setup")
	}
}

func TestARMRunArchivesInSelectedRepoWithIndependentProfile(t *testing.T) {
	for _, archiveFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "archive failure"}[archiveFails], func(t *testing.T) {
			cfg := config.Default()
			cfg.Machine.ID = "source-x64"
			cfg.ARMTargetEnvironment = armEnvironmentFixture(t, "aarch64")
			repo := restic.Repository{Location: "fixture-default-repository"}
			var verified, cleaned, archived bool
			lanes := map[string]systemSnapshotLane{}
			ops := armCaptureOperations{
				inventory: func(context.Context) (inventory.ApplicationManifest, error) {
					if !verified {
						t.Fatal("inventory ran before repository verification")
					}
					return inventory.ApplicationManifest{MachineID: "source-x64"}, nil
				},
				prepare: func(ctx context.Context, target, stage, machine string, source inventory.ApplicationManifest, download, build bool, emit func(packagecapsule.Progress)) (packagecapsule.Manifest, string, func(), error) {
					if target != cfg.ARMTargetEnvironment || stage != "managed-staging" || machine != "source-x64" || !download || build {
						t.Fatal("incorrect preparation settings")
					}
					emit(packagecapsule.Progress{Phase: "download", Package: "firefox", Completed: 1, Total: 2, Source: "official"})
					return packagecapsule.Manifest{Architecture: "aarch64", Summary: packagecapsule.Summary{Captured: 2}}, "prepared-capsule", func() { cleaned = true }, nil
				},
				archive: func(ctx context.Context, got restic.Repository, profile, machine string, paths []string, emit func(restic.BackupProgress)) error {
					archived = true
					if got.Location != repo.Location || profile != "packages-arm64" || machine != "source-x64" || len(paths) != 1 || paths[0] != "prepared-capsule" {
						t.Fatal("ARM capsule sent to wrong repository/profile")
					}
					emit(restic.BackupProgress{FilesDone: 1, TotalFiles: 2, BytesDone: 1024})
					if archiveFails {
						return errors.New("repository unavailable")
					}
					return nil
				},
			}
			_, err := captureARMWith(context.Background(), cfg, "managed-staging", repo, func() error { verified = true; return nil }, func(key string, lane systemSnapshotLane) { lanes[key] = lane }, ops)
			if (err != nil) != archiveFails || !cleaned || !archived {
				t.Fatalf("err=%v cleaned=%v archived=%v", err, cleaned, archived)
			}
			if lanes["repository"].BytesDone != 1024 || lanes["packages"].State != "prepared" {
				t.Fatal("missing live lane feedback")
			}
		})
	}
}

func TestARMResultPreservesNativeScheduleAndSystemSet(t *testing.T) {
	cfg := config.Default()
	cfg.PackagePolicy.Scheduled = true
	native := &packagecapsule.Manifest{Architecture: "x86_64"}
	for _, failure := range []bool{false, true} {
		m := Model{cfg: cfg, styles: newStyles(), mode: modeProfiles, busy: true, armStage: "running", packageManifest: native, systemSnapshotLanes: map[string]systemSnapshotLane{"packages": {State: "complete"}}, armLanes: map[string]systemSnapshotLane{}}
		msg := armDoneMsg{manifest: packagecapsule.Manifest{Architecture: "aarch64", Summary: packagecapsule.Summary{Captured: 8}}}
		if failure {
			msg.err = errors.New("cancelled")
		}
		updated, _ := m.Update(msg)
		m = updated.(Model)
		if m.busy || m.armStage != "done" || m.packageManifest != native || !m.cfg.PackagePolicy.Scheduled || m.systemSnapshotLanes["packages"].State != "complete" {
			t.Fatal("ARM result changed native/System Set state")
		}
		view := m.applicationsScreen()
		if failure {
			if m.armManifest != nil || strings.Contains(view, "capsule saved") {
				t.Fatal("failed run claimed success")
			}
		} else if !strings.Contains(view, "8 artifacts") || m.armLanes["repository"].State != "complete" {
			t.Fatal("success missing from rails")
		}
	}
}
