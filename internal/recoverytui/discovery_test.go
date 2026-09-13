package recoverytui

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDiscoveryFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryUsesCanonicalKitInsteadOfOlderCopies(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WEAZLBACK_MEDIA_ROOT", root)
	canonical := filepath.Join(root, "weazlback-recovery.wzrk")
	for _, name := range []string{"000-old.wzrk", ".weazlback-recovery-interrupted.wzrk", "weazlback-recovery.wzrk"} {
		writeDiscoveryFixture(t, filepath.Join(root, name), "fixture")
	}
	m := New()
	if m.stage != "kit" || len(m.kits) != 1 || m.input.Value() != canonical {
		t.Fatalf("canonical kit lost priority: stage=%s kits=%v input=%s", m.stage, m.kits, m.input.Value())
	}
}

func TestRecoveryFollowsActiveKitUpdateWithoutSelectingOldPayload(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WEAZLBACK_MEDIA_ROOT", root)
	writeDiscoveryFixture(t, filepath.Join(root, "weazlback-recovery.wzrk"), "stale legacy kit")
	for _, revision := range []string{"kit-first", "kit-second"} {
		canonical := filepath.Join(root, "payloads", revision, "weazlback-recovery.wzrk")
		writeDiscoveryFixture(t, canonical, revision)
		writeDiscoveryFixture(t, filepath.Join(root, "CURRENT"), revision+"\n")
		m := New()
		if len(m.kits) != 1 || m.input.Value() != canonical {
			t.Fatalf("refresh selected wrong kit: %v, want %s", m.kits, canonical)
		}
	}
}

func TestMissingCanonicalKitDoesNotSelectUnrelatedKit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WEAZLBACK_MEDIA_ROOT", root)
	writeDiscoveryFixture(t, filepath.Join(root, "BUNDLE.json"), "{}")
	writeDiscoveryFixture(t, filepath.Join(root, "old.wzrk"), "old kit")
	m := New()
	if len(m.kits) != 1 || m.input.Value() != filepath.Join(root, "weazlback-recovery.wzrk") {
		t.Fatalf("missing canonical kit fell back to old credentials: %v", m.kits)
	}
}
