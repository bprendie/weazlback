package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareMediaPreservesUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "media")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(target, "keep-me")
	if err := os.WriteFile(keep, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := func(name, body string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	err := PrepareMedia(target, MediaSources{
		Weazlback: source("app", "app"), Restore: source("restore", "restore"), Kit: source("kit", "kit"), Version: "test-version", Source: "/test/bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(keep); string(body) != "safe" {
		t.Fatal("unrelated file changed")
	}
	checksums, err := os.ReadFile(filepath.Join(target, "SHA256SUMS"))
	if err != nil || !strings.Contains(string(checksums), "weazlback-recovery.wzrk") {
		t.Fatalf("checksums=%q err=%v", checksums, err)
	}
	for _, name := range []string{"weazlback", "weazlback-restore", "weazlback-recovery.wzrk", "WEAZLBACK-VERSION.json", "RESTORE.txt", "THIRD_PARTY_NOTICES.txt"} {
		if _, err := os.Stat(filepath.Join(target, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	provenance, err := os.ReadFile(filepath.Join(target, "WEAZLBACK-VERSION.json"))
	if err != nil || !strings.Contains(string(provenance), `"version": "test-version"`) || !strings.Contains(string(provenance), `"source": "/test/bin"`) {
		t.Fatalf("provenance=%q err=%v", provenance, err)
	}
	for name, want := range map[string]os.FileMode{"weazlback": 0o755, "weazlback-restore": 0o755, "weazlback-recovery.wzrk": 0o644} {
		info, err := os.Stat(filepath.Join(target, name))
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s mode=%v err=%v", name, info.Mode().Perm(), err)
		}
	}
	if err := VerifyMediaDirectory(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "weazlback"), []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMediaDirectory(target); err == nil {
		t.Fatal("tampered recovery binary passed media verification")
	}
}
