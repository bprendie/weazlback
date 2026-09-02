package recovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportAndVerify(t *testing.T) {
	dir := t.TempDir()
	vault, config := filepath.Join(dir, "vault"), filepath.Join(dir, "config")
	if err := os.WriteFile(vault, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(dir, "kit.wzrk")
	if err := Export(kit, Sources{Vault: vault, Config: config}, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(kit, []byte("wrong")); err == nil {
		t.Fatal("wrong password verified kit")
	}
	manifest, err := Verify(kit, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != SchemaVersion || len(manifest.Files) != 2 {
		t.Fatalf("manifest = %#v", manifest)
	}
	bundle, err := Open(kit, []byte("x"))
	if err != nil || string(bundle.Vault) != "encrypted" || string(bundle.Config) != "{}" {
		t.Fatalf("bundle=%#v err=%v", bundle, err)
	}
	bundle.Close()
	if len(bundle.Vault) > 0 && bundle.Vault[0] != 0 {
		t.Fatal("bundle close did not clear sensitive bytes")
	}
	info, err := os.Stat(kit)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestOpensV050RecoveryKit(t *testing.T) {
	bundle, err := Open(filepath.Join("testdata", "v050.wzrk"), []byte("weazlback-v050-compatibility"))
	if err != nil {
		t.Fatalf("open pre-upgrade recovery kit: %v", err)
	}
	defer bundle.Close()
	if bundle.Manifest.SchemaVersion != SchemaVersion || string(bundle.Config) != "{\"schema_version\":1}\n" {
		t.Fatalf("unexpected pre-upgrade bundle: schema=%d config=%q", bundle.Manifest.SchemaVersion, bundle.Config)
	}
	if string(bundle.KnownHosts) != "backup.example.test ssh-ed25519 AAAAsynthetic\n" {
		t.Fatalf("unexpected known-host fixture: %q", bundle.KnownHosts)
	}
}
