package freshrestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/browserrepair"
)

type noBrowserProcesses struct{}

func (noBrowserProcesses) Running(_ browserrepair.Family, _ int) bool { return false }

func TestBrowserCompatibilityRunsOnlyForEligibleHostnameTransition(t *testing.T) {
	for _, test := range []struct {
		scope, source, target string
		removed               int
	}{
		{"core", "old-host", "new-host", 1}, {"home", "old-host", "new-host", 1}, {"everything", "old-host", "new-host", 1},
		{"applications", "old-host", "new-host", 0}, {"core", "same-host", "same-host", 0}, {"core", "", "new-host", 0},
	} {
		t.Run(test.scope+test.source+test.target, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".config", "chromium")
			fixtureFile(t, filepath.Join(root, "Local State"))
			fixtureFile(t, filepath.Join(root, "Default", "History"))
			fixtureFile(t, filepath.Join(root, "SingletonLock"))
			r := Restore{Options: Options{BrowserProcesses: noBrowserProcesses{}}, Plan: Plan{Scope: test.scope, SourceHostname: test.source, Hostname: test.target, TargetHome: home}}
			result, issues := r.repairBrowserCompatibility()
			if result.Removed != test.removed || len(issues) != 0 {
				t.Fatalf("result=%+v issues=%v", result, issues)
			}
			_, err := os.Lstat(filepath.Join(root, "SingletonLock"))
			if test.removed == 1 && !os.IsNotExist(err) {
				t.Fatal("eligible lock survived")
			}
			if test.removed == 0 && err != nil {
				t.Fatal("ineligible restore changed lock")
			}
		})
	}
}

func fixtureFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
}
