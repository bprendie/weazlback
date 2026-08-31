package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigratesAndPersistsStableMachineIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := Default()
	legacy.Machine = Machine{}
	b, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := Load(path)
	if err != nil || !ValidMachineID(first.Machine.ID) {
		t.Fatalf("first=%#v err=%v", first.Machine, err)
	}
	second, err := Load(path)
	if err != nil || second.Machine.ID != first.Machine.ID {
		t.Fatalf("identity changed: first=%q second=%q err=%v", first.Machine.ID, second.Machine.ID, err)
	}
}

func TestLoadVersionsIdentityAndPreservesHostnameHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.Machine.Version = 0
	cfg.Machine.Hostnames = []string{"old-name"}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Machine.Version != MachineSchemaVersion || len(loaded.Machine.Hostnames) < 2 {
		t.Fatalf("machine=%#v", loaded.Machine)
	}
}

func TestSaveIsPrivateAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "config.json")
	cfg := Default()
	cfg.Destinations = append(cfg.Destinations, Destination{ID: "local", Repository: "/repo"})
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Destinations) != 1 || got.Destinations[0].ID != "local" {
		t.Fatalf("config=%#v", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestActiveDestinationFallsBackAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.Destinations = []Destination{{ID: "ssh", Name: "remote"}, {ID: "local", Name: "drive"}}
	if got := cfg.Active(); got == nil || got.ID != "ssh" {
		t.Fatalf("fallback active=%#v", got)
	}
	cfg.ActiveDestination = "local"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active() == nil || got.Active().ID != "local" {
		t.Fatalf("round-trip active=%#v", got.Active())
	}
}

func TestCoreIncludesInstalledWeazlApplicationRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".subweazl")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	for _, profile := range cfg.Profiles {
		if profile.Name == "core" {
			for _, path := range profile.Includes {
				if path == root {
					return
				}
			}
		}
	}
	t.Fatalf("Core profile omitted %s", root)
}

func TestHeavyRetentionIsIndependent(t *testing.T) {
	cfg := Default()
	if cfg.HeavyRetention.Daily != 7 || cfg.HeavyRetention.Monthly != 3 {
		t.Fatalf("heavy retention=%+v", cfg.HeavyRetention)
	}
	if cfg.HeavyRetention == cfg.Retention {
		t.Fatal("Heavy retention unexpectedly matches Core/Home")
	}
}
