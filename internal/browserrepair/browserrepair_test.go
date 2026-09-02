package browserrepair

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type fakeProcesses map[Family]bool

func (f fakeProcesses) Running(family Family, _ int) bool { return f[family] }

func TestChromiumRepairOnlyRemovesExactLocks(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
	mustFile(t, filepath.Join(root, "Local State"))
	mustFile(t, filepath.Join(root, "Default", "History"))
	for _, name := range append(append([]string{}, chromiumLocks...), "SingletonLock.keep") {
		mustFile(t, filepath.Join(root, name))
	}
	options := Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}
	plan := Detect(options)
	result := Apply(options, plan)
	if result.Removed != 3 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, name := range chromiumLocks {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived", name)
		}
	}
	for _, name := range []string{"Default/History", "SingletonLock.keep"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("protected %s: %v", name, err)
		}
	}
	second := Apply(options, Detect(options))
	if second.Removed != 0 || second.Failed != 0 {
		t.Fatalf("not idempotent: %+v", second)
	}
}

func TestMozillaDeclaredProfileRepair(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".mozilla", "firefox")
	profile := filepath.Join(root, "abc.default")
	mustFile(t, filepath.Join(profile, "prefs.js"))
	mustFile(t, filepath.Join(profile, "compatibility.ini"))
	mustFile(t, filepath.Join(profile, "cookies.sqlite"))
	mustFile(t, filepath.Join(profile, ".parentlock"))
	mustFile(t, filepath.Join(profile, "lock"))
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte("[Profile0]\nPath=abc.default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Apply(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}, Detect(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}))
	if result.Removed != 2 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(profile, "cookies.sqlite")); err != nil {
		t.Fatal("protected profile data changed")
	}
}

func TestLiveBrowserAndAmbiguousRootsRemainUntouched(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".config", "chromium")
	mustFile(t, filepath.Join(root, "Local State"))
	mustFile(t, filepath.Join(root, "Default", "prefs"))
	mustFile(t, filepath.Join(root, "SingletonLock"))
	options := Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{Chromium: true}}
	plan := Detect(options)
	result := Apply(options, plan)
	if result.Live != 1 || result.Removed != 0 {
		t.Fatalf("unexpected live result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "SingletonLock")); err != nil {
		t.Fatal("live lock removed")
	}
	unknown := filepath.Join(home, ".config", "electron-app")
	mustFile(t, filepath.Join(unknown, "Local State"))
	mustFile(t, filepath.Join(unknown, "Default", "prefs"))
	mustFile(t, filepath.Join(unknown, "SingletonLock"))
	_ = Apply(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}, Detect(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}))
	if _, err := os.Stat(filepath.Join(unknown, "SingletonLock")); err != nil {
		t.Fatal("electron fixture touched")
	}
}

func TestSymlinkedRootAndSymlinkTargetAreSafe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	home, outside := t.TempDir(), t.TempDir()
	real := filepath.Join(outside, "chromium")
	mustFile(t, filepath.Join(real, "Local State"))
	mustFile(t, filepath.Join(real, "Default", "prefs"))
	mustFile(t, filepath.Join(real, "SingletonLock"))
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, ".config", "chromium")); err != nil {
		t.Fatal(err)
	}
	plan := Detect(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}})
	if len(plan.Entries) != 1 || plan.Entries[0].Action != SkipBoundary {
		t.Fatalf("unsafe plan: %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(real, "SingletonLock")); err != nil {
		t.Fatal("outside target touched")
	}

	home2 := t.TempDir()
	root := filepath.Join(home2, ".config", "chromium")
	target := filepath.Join(outside, "target")
	mustFile(t, filepath.Join(root, "Local State"))
	mustFile(t, filepath.Join(root, "Default", "prefs"))
	mustFile(t, target)
	if err := os.Symlink(target, filepath.Join(root, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	result := Apply(Options{Home: home2, UID: os.Getuid(), Processes: fakeProcesses{}}, Detect(Options{Home: home2, UID: os.Getuid(), Processes: fakeProcesses{}}))
	if result.Removed != 1 {
		t.Fatalf("link not removed: %+v", result)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("symlink target removed")
	}
}

func TestExclusionsRequireValidatedMarkers(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".config", "chromium")
	mustFile(t, filepath.Join(root, "SingletonLock"))
	if got := Exclusions(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}); len(got) != 0 {
		t.Fatalf("ambiguous exclusion: %v", got)
	}
	mustFile(t, filepath.Join(root, "Local State"))
	mustFile(t, filepath.Join(root, "Default", "prefs"))
	if got := Exclusions(Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{Chromium: true}}); len(got) != 1 {
		t.Fatalf("missing live exclusion: %v", got)
	}
}

func TestProtectedBrowserDataNamesSurvive(t *testing.T) {
	home := t.TempDir()
	chromium := filepath.Join(home, ".config", "chromium")
	mustFile(t, filepath.Join(chromium, "Local State"))
	mustFile(t, filepath.Join(chromium, "Default", "marker"))
	mustFile(t, filepath.Join(chromium, "SingletonLock"))
	for _, name := range []string{"Cookies", "History", "Login Data", "Preferences", "Secure Preferences", "Sessions/Session_1"} {
		mustFile(t, filepath.Join(chromium, "Default", name))
	}
	mozilla := filepath.Join(home, ".mozilla", "firefox", "safe.default")
	mustFile(t, filepath.Join(mozilla, "prefs.js"))
	mustFile(t, filepath.Join(mozilla, "times.json"))
	mustFile(t, filepath.Join(mozilla, ".parentlock"))
	for _, name := range []string{"cookies.sqlite", "places.sqlite", "logins.json", "key4.db", "sessionstore.jsonlz4", "sessionstore-backups/recovery.jsonlz4", "extensions.json"} {
		mustFile(t, filepath.Join(mozilla, name))
	}
	options := Options{Home: home, UID: os.Getuid(), Processes: fakeProcesses{}}
	result := Apply(options, Detect(options))
	if result.Removed != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, path := range []string{filepath.Join(chromium, "Default", "Cookies"), filepath.Join(chromium, "Default", "History"), filepath.Join(chromium, "Default", "Login Data"), filepath.Join(chromium, "Default", "Preferences"), filepath.Join(mozilla, "cookies.sqlite"), filepath.Join(mozilla, "places.sqlite"), filepath.Join(mozilla, "logins.json"), filepath.Join(mozilla, "key4.db"), filepath.Join(mozilla, "sessionstore.jsonlz4"), filepath.Join(mozilla, "sessionstore-backups", "recovery.jsonlz4")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected path changed: %v", err)
		}
	}
}

func TestProcessMatchingIsExact(t *testing.T) {
	if !matchesProcess(Chromium, "brave-browser", nil) || !matchesProcess(Mozilla, "firefox", nil) {
		t.Fatal("known browser not recognized")
	}
	for _, name := range []string{"brave-browser-helper", "my-firefox-backup", "electron", "firefox.old"} {
		if matchesProcess(Chromium, name, nil) || matchesProcess(Mozilla, name, nil) {
			t.Fatalf("broad process match for %q", name)
		}
	}
}

func TestCustomXDGConfigHomeInsideHome(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "settings")
	root := filepath.Join(configHome, "chromium")
	mustFile(t, filepath.Join(root, "Local State"))
	mustFile(t, filepath.Join(root, "Default", "History"))
	mustFile(t, filepath.Join(root, "SingletonLock"))
	options := Options{Home: home, ConfigHome: configHome, UID: os.Getuid(), Processes: fakeProcesses{}}
	result := Apply(options, Detect(options))
	if result.Removed != 1 {
		t.Fatalf("custom XDG root missed: %+v", result)
	}
}

func mustFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}
