package freshrestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/platform"
)

func TestScopePlannerKeepsThreePresetsSimple(t *testing.T) {
	omarchy := platform.Identity{Family: "arch", Desktop: "omarchy-shell"}
	for _, test := range []struct {
		scope             string
		core, home, heavy bool
	}{
		{"core", true, false, false}, {"core-home", true, true, false}, {"everything", true, true, true},
	} {
		got := PlanScope(test.scope, omarchy, omarchy, nil)
		if got.IncludeCore != test.core || got.IncludeHome != test.home || got.IncludeHeavy != test.heavy || got.Warning != "" {
			t.Fatalf("%s: %+v", test.scope, got)
		}
	}
}

func TestCrossPlatformPlannerWithholdsCoreAndContinues(t *testing.T) {
	claim := platform.Claim{Path: "/home/me/.config/omarchy", Owner: "desktop", Domain: "omarchy-shell"}
	got := PlanScope("everything", platform.Identity{Family: "arch", Desktop: "omarchy-shell"}, platform.Identity{Family: "debian", Desktop: "gnome"}, []platform.Claim{claim})
	if got.IncludeCore || !got.IncludeHome || !got.IncludeHeavy || !got.IncludeApplications || got.Warning != PlatformMismatchWarning || len(got.WithheldClaims) != 1 {
		t.Fatalf("decision=%+v", got)
	}
}

func TestCrossPlatformHomeRemovesClaimsButKeepsHiddenData(t *testing.T) {
	stage, home := t.TempDir(), "/home/me"
	for _, path := range []string{".config/omarchy/shell.json", ".config/chromium/Default/History", ".ollama/models/model", "Code/app/.gobuildcache/object"} {
		full := stagedPath(stage, filepath.Join(home, path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := removeWithheldCore(stage, home, []platform.Claim{{Path: filepath.Join(home, ".config/omarchy")}})
	if err != nil || len(removed) != 1 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	for _, path := range []string{".config/chromium/Default/History", ".ollama/models/model", "Code/app/.gobuildcache/object"} {
		if _, err := os.Stat(stagedPath(stage, filepath.Join(home, path))); err != nil {
			t.Fatalf("portable hidden data removed: %s: %v", path, err)
		}
	}
}
