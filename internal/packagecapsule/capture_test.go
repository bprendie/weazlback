package packagecapsule

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	outputs map[string]string
	runs    [][]string
	fail    map[string]error
}

func (f *fakeRunner) Output(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	if err := f.fail[key]; err != nil {
		return "", err
	}
	if name == "bsdtar" && len(args) == 3 {
		if args[2] == ".PKGINFO" {
			base := filepath.Base(args[1])
			pkg := "alpha"
			if strings.HasPrefix(base, "beta-") {
				pkg = "beta"
			}
			return "pkgname = " + pkg + "\npkgver = 1.0-1\narch = any\ndepend = glibc\n", nil
		}
		return "buildenv = !distcc\n", nil
	}
	if name == "vercmp" {
		return "0\n", nil
	}
	return f.outputs[key], nil
}

func (f *fakeRunner) Run(name string, args ...string) error {
	f.runs = append(f.runs, append([]string{name}, args...))
	return f.fail[name]
}

func (f *fakeRunner) RunDir(dir, name string, args ...string) error {
	f.runs = append(f.runs, append([]string{dir, name}, args...))
	return f.fail[name]
}

func TestCaptureBuildsCuratedManifestWithoutScanningCore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cache := filepath.Join(home, ".cache", "yay", "alpha")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(cache, "alpha-1.0-1-x86_64.pkg.tar.zst")
	if err := os.WriteFile(artifact, []byte("stable package bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{outputs: map[string]string{
		"pacman -Q":   "alpha 1.0-1\nbeta 1.0-1\n",
		"pacman -Qqe": "alpha\n",
		"pacman -Qqm": "beta\n",
		"flatpak list --app --columns=application":                     "org.example.App\n",
		"systemctl list-unit-files --state=enabled --no-legend":        "sshd.service enabled\n",
		"systemctl --user list-unit-files --state=enabled --no-legend": "widget.service enabled\n",
	}}
	manifest, root, cleanup, err := Capture(Options{MachineID: strings.Repeat("a", 32), StagingRoot: filepath.Join(home, "stage"), Run: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if manifest.Summary.Installed != 2 || manifest.Summary.Captured != 1 || manifest.Summary.Missing != 1 {
		t.Fatalf("summary=%+v", manifest.Summary)
	}
	if manifest.Packages[0].Name != "alpha" || !manifest.Packages[0].Compatible || manifest.Packages[0].Reason != "explicit" {
		t.Fatalf("alpha=%+v", manifest.Packages[0])
	}
	if manifest.Packages[1].Source != "foreign" || manifest.Packages[1].Artifact != "" {
		t.Fatalf("beta=%+v", manifest.Packages[1])
	}
	if _, err := os.Stat(filepath.Join(root, ManifestName)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, manifest.Packages[0].Artifact))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact mode=%v", info.Mode().Perm())
	}
}

func TestDownloadFailureIsVisibleException(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runner := &fakeRunner{outputs: map[string]string{"pacman -Q": "alpha 1.0-1\n", "pacman -Qqe": "alpha\n", "pacman -Qqm": ""}, fail: map[string]error{"sudo": errors.New("denied")}}
	manifest, _, cleanup, err := Capture(Options{StagingRoot: filepath.Join(home, "stage"), Download: true, Run: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(manifest.Exceptions) < 2 || manifest.Exceptions[0].Code != "official-download-failed" {
		t.Fatalf("exceptions=%+v", manifest.Exceptions)
	}
}

func TestNativeArtifactIsRejected(t *testing.T) {
	meta := artifactMetadata{Name: "alpha", Version: "1", Architecture: "any"}
	if ok, _ := compatible(meta, "cflags = -O2 -march=native"); ok {
		t.Fatal("native artifact accepted")
	}
}

func TestValidateRejectsArtifactTamperingAndPathEscape(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "artifact.pkg.tar.zst")
	if err := os.WriteFile(artifact, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _ := digest(artifact)
	manifest := Manifest{SchemaVersion: SchemaVersion, CapturedAt: time.Now(), Hostname: "host",
		Packages: []Package{{Name: "alpha", Installed: "1", Source: "official", Artifact: filepath.Base(artifact), SHA256: hash, Compatible: true}},
		Summary:  Summary{Installed: 1, Captured: 1}}
	if err := Validate(root, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, manifest); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("tamper error=%v", err)
	}
	manifest.Packages[0].Artifact = "../escape"
	if err := Validate(root, manifest); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escape error=%v", err)
	}
}
