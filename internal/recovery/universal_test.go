package recovery

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fakeBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	m := BundleManifest{Version: "test", ResticVersion: "v0.19.1", Files: map[string]string{}}
	write := func(name string, b []byte) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0755); err != nil {
			t.Fatal(err)
		}
		sum, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		m.Files[name] = sum
	}
	for _, arch := range BundleArchitectures {
		b := make([]byte, 64)
		copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
		binary.LittleEndian.PutUint16(b[16:], 2)
		machine := uint16(62)
		if arch == "arm64" {
			machine = 183
		}
		binary.LittleEndian.PutUint16(b[18:], machine)
		binary.LittleEndian.PutUint32(b[20:], 1)
		binary.LittleEndian.PutUint16(b[52:], 64)
		for _, tool := range BundleTools {
			write("linux-"+arch+"/"+tool, b)
		}
	}
	for _, asset := range BundleRuntime {
		write(asset, []byte("fixture"))
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(root, BundleManifestName), b, 0644); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestUniversalRefreshIsAtomicAndVerifiesAllArchitectures(t *testing.T) {
	bundle := fakeBundle(t)
	media := t.TempDir()
	kit := filepath.Join(t.TempDir(), "kit")
	os.WriteFile(kit, []byte("encrypted-fixture"), 0600)
	if err := PrepareUniversal(media, bundle, kit, "test"); err != nil {
		t.Fatal(err)
	}
	first, err := ActiveMedia(media)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMediaDirectory(media); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(bundle, "linux-arm64", "restic"), []byte("wrong"), 0755)
	if err := PrepareUniversal(media, bundle, kit, "test"); err == nil {
		t.Fatal("bad refresh accepted")
	}
	active, _ := ActiveMedia(media)
	if active != first {
		t.Fatal("failed refresh changed active kit")
	}
	if err := VerifyMediaDirectory(media); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(active, "linux-amd64", "weazlback"), []byte("wrong"), 0755)
	if err := VerifyMediaDirectory(media); err == nil {
		t.Fatal("tampered architecture accepted")
	}
}
func TestUniversalRejectsMissingChecksumAndTraversal(t *testing.T) {
	bundle := fakeBundle(t)
	media := t.TempDir()
	kit := filepath.Join(t.TempDir(), "kit")
	os.WriteFile(kit, []byte("fixture"), 0600)
	if err := PrepareUniversal(media, bundle, kit, "test"); err != nil {
		t.Fatal(err)
	}
	root, _ := ActiveMedia(media)
	b, _ := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "linux-arm64/restic") {
			lines = append(lines, line)
		}
	}
	os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(strings.Join(lines, "\n")), 0644)
	if err := VerifyMediaDirectory(media); err == nil {
		t.Fatal("missing required tool checksum accepted")
	}
	os.WriteFile(filepath.Join(media, "CURRENT"), []byte("kit-../../outside"), 0644)
	if _, err := ActiveMedia(media); err == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestMediaRejectsSymlinkAsset(t *testing.T) {
	root := fakeBundle(t)
	path := filepath.Join(root, "linux-arm64", "restic")
	b, _ := os.ReadFile(path)
	outside := filepath.Join(t.TempDir(), "restic")
	os.WriteFile(outside, b, 0755)
	os.Remove(path)
	os.Symlink(outside, path)
	if _, err := ValidateBundle(root, "test"); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestUniversalBuiltToolsSmoke(t *testing.T) {
	bundle := os.Getenv("WEAZLBACK_TEST_BUNDLE")
	if bundle == "" {
		t.Skip("set WEAZLBACK_TEST_BUNDLE for compiled media smoke test")
	}
	m, err := ValidateBundle(bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	kit := filepath.Join(t.TempDir(), "kit")
	os.WriteFile(kit, []byte("not unlocked by --version"), 0600)
	if err := PrepareUniversal(root, bundle, kit, m.Version); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"weazlback", "weazlback-restore", "weazlback-turbo-restore"} {
		command := exec.Command(filepath.Join(root, tool), "--version")
		command.Dir = t.TempDir()
		out, err := command.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != m.Version {
			t.Fatalf("%s: %s %v", tool, out, err)
		}
	}
}
