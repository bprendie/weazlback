package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestRecoveryPrepareRequiresUniversalBundle(t *testing.T) {
	t.Setenv("WEAZLBACK_BUNDLE_DIR", t.TempDir())
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := Model{cfg: config.Default()}
	form, _ := m.startRecoveryForm(true)
	m = form.(Model)
	target := t.TempDir()
	m.recoveryInputs[0].SetValue(target)
	m.recoveryInputs[1].SetValue("test-passphrase")
	_, cmd := m.exportRecovery()
	if result := cmd().(recoveryDoneMsg); result.err == nil {
		t.Fatal("preparation accepted a missing universal bundle")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed preparation changed destination: %v, %v", entries, err)
	}
}

func TestRecoveryPrepareWithBuiltBundle(t *testing.T) {
	bundle := os.Getenv("WEAZLBACK_TEST_BUNDLE")
	if bundle == "" {
		t.Skip("set WEAZLBACK_TEST_BUNDLE for TUI universal preparation smoke test")
	}
	t.Setenv("WEAZLBACK_BUNDLE_DIR", bundle)
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	t.Setenv("WEAZLBACK_CONFIG", "")
	cfg := config.Default()
	path, err := vault.Path(cfg.ActiveVault)
	if err != nil {
		t.Fatal(err)
	}
	v := vault.New(path)
	if err := v.Create([]byte("test-passphrase")); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	m := Model{cfg: cfg}
	form, _ := m.startRecoveryForm(true)
	m = form.(Model)
	target := t.TempDir()
	m.recoveryInputs[0].SetValue(target)
	m.recoveryInputs[1].SetValue("test-passphrase")
	_, cmd := m.exportRecovery()
	result := cmd().(recoveryDoneMsg)
	if result.err != nil || !result.prepared {
		t.Fatalf("TUI preparation failed: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(target, "CURRENT")); err != nil {
		t.Fatal("TUI did not produce universal media:", err)
	}
	if err := recovery.VerifyMediaDirectory(target); err != nil {
		t.Fatal(err)
	}
}
