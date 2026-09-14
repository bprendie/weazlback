package freshrestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopHelpersPreserveHPKeysWithoutReplacingPersonalBindings(t *testing.T) {
	stage, target := t.TempDir(), t.TempDir()
	write := func(root, rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(stage, ".config/hypr/bindings.lua", `o.bind("SUPER + SHIFT + M", "Weazltunes", "weazltunes")`)
	write(target, ".config/hypr/bindings.lua", "require(\"hypr.hp_media_keys\")\n")
	write(target, ".config/hypr/hp_media_keys.lua", "-- tested HP workaround")
	write(stage, ".config/omarchy/plugins/custom/BarWidget.qml", "source widget")
	write(target, ".config/omarchy/plugins/custom/BarWidget.qml", "older target widget")
	write(target, ".config/omarchy/plugins/hardware/BarWidget.qml", "target-only widget")
	r := Restore{Plan: Plan{TargetHome: target}}
	for i := 0; i < 2; i++ {
		if err := r.mergeTargetDesktopHelpers(stage); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(stage, ".config/hypr/bindings.lua"))
	if !strings.Contains(string(b), "Weazltunes") || strings.Count(string(b), `require("hypr.hp_media_keys")`) != 1 {
		t.Fatalf("bindings lost or helper duplicated: %s", b)
	}
	for rel, want := range map[string]string{".config/hypr/hp_media_keys.lua": "-- tested HP workaround", ".config/omarchy/plugins/custom/BarWidget.qml": "source widget", ".config/omarchy/plugins/hardware/BarWidget.qml": "target-only widget"} {
		b, err := os.ReadFile(filepath.Join(stage, rel))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %s %v", rel, b, err)
		}
	}
}

func TestChangedMigrationPolicyReappliesCompletedRestore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "journal.json")
	old := Journal{Stage: "system_validated", SnapshotID: "snapshot", CommittedPaths: []string{"/home/bobp/.config"}, Migration: Migration{Active: true}}
	r := Restore{JournalPath: p, Journal: old, Plan: Plan{Migration: migrationPolicy("amd64", "arm64")}}
	if err := r.resetChangedMigrationPolicy(); err != nil {
		t.Fatal(err)
	}
	if r.Journal.Stage != "" {
		t.Fatal("old completion skipped new policy")
	}
	files, _ := filepath.Glob(p + ".before-policy-*")
	if len(files) != 1 {
		t.Fatal("prior journal not retained")
	}
	saved, err := LoadJournal(files[0])
	if err != nil || len(saved.CommittedPaths) != 1 {
		t.Fatalf("invalid saved journal: %+v %v", saved, err)
	}
	r.Journal = old
	r.Journal.PendingTarget = "/home/bobp/.config"
	if err := r.resetChangedMigrationPolicy(); err == nil {
		t.Fatal("reset an unresolved placement")
	}
	r.Journal = old
	r.Journal.Migration = r.Plan.Migration
	if err := r.resetChangedMigrationPolicy(); err != nil || r.Journal.Stage != old.Stage {
		t.Fatal("reset current-policy journal")
	}
}
