package apprestore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/inventory"
)

type xorCryptor byte

func (c xorCryptor) Encrypt(value []byte) ([]byte, error) {
	out := append([]byte(nil), value...)
	for i := range out {
		out[i] ^= byte(c)
	}
	return out, nil
}

type fakeResolver map[string]string

func (f fakeResolver) Available(name string, source Source) (string, bool) {
	value, ok := f[string(source)+"/"+name]
	return value, ok
}

func manifest(host string, official, aur []inventory.InstalledPackage) inventory.ApplicationManifest {
	var officialNames, aurNames []string
	for _, pkg := range official {
		officialNames = append(officialNames, pkg.Name)
	}
	for _, pkg := range aur {
		aurNames = append(aurNames, pkg.Name)
	}
	return inventory.ApplicationManifest{SchemaVersion: 1, CapturedAt: time.Now(), Hostname: host,
		Packages:    inventory.PackageInventory{OfficialExplicit: officialNames, ForeignExplicit: aurNames, OfficialInstalled: official, ForeignInstalled: aur},
		PackagePlan: inventory.PackageRestorePlan{Official: officialNames, AUR: aurNames}}
}

func TestMachineManifestPlansRemainIdentitySpecific(t *testing.T) {
	thinkpad := manifest("thinkpad", []inventory.InstalledPackage{{Name: "base", Version: "1"}}, []inventory.InstalledPackage{{Name: "wwan-control", Version: "2"}})
	hp := manifest("hp", []inventory.InstalledPackage{{Name: "base", Version: "1"}}, nil)
	current := manifest("desktop", nil, nil)
	resolver := fakeResolver{"official/base": "1", "aur/wwan-control": "3"}
	thinkPlan := Build("thinkpad", "one", thinkpad, current, resolver)
	hpPlan := Build("hp", "two", hp, current, resolver)
	if len(thinkPlan.Substitutions) != 1 || thinkPlan.Substitutions[0].Name != "wwan-control" {
		t.Fatalf("thinkpad plan=%#v", thinkPlan)
	}
	for _, group := range [][]Package{hpPlan.Install, hpPlan.Substitutions, hpPlan.Unavailable} {
		for _, pkg := range group {
			if pkg.Name == "wwan-control" {
				t.Fatal("HP plan inherited ThinkPad WWAN intent")
			}
		}
	}
}

type fakeRunner struct {
	commands []string
	failAUR  bool
}

type hangingRunner struct{}

func (hangingRunner) Run(ctx context.Context, _ string, _ ...string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	command := name + " " + strings.Join(args, " ")
	r.commands = append(r.commands, command)
	if r.failAUR && (name == "paru" || name == "yay") {
		return errors.New("timeout")
	}
	return nil
}

func TestExecutionReportsFailuresAndNeverRunsRemoval(t *testing.T) {
	plan := Plan{Install: []Package{{Name: "aur-slow", Source: AUR}}, InstalledLater: []Package{{Name: "keep-me", Source: Official}},
		Unavailable: []Package{{Name: "gone"}}, Conflicts: []Package{{Name: "conflict"}}}
	runner := &fakeRunner{failAUR: true}
	result := Execute(context.Background(), plan, runner, nil)
	if len(result.Failures) != 1 || len(result.RemovalCommands) != 1 {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Unavailable) != 1 || len(result.Conflicts) != 1 {
		t.Fatalf("summary lost unresolved sets: %#v", result)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, " -R") || strings.Contains(command, "uninstall") {
			t.Fatalf("restore executed removal: %s", command)
		}
	}
}

func TestEncryptedJournalRetainsSubstitutionsAndFailures(t *testing.T) {
	path := t.TempDir() + "/apps.enc"
	plan := Plan{MachineID: "thinkpad", Substitutions: []Package{{Name: "wwan-control"}}}
	result := Result{Substituted: []string{"wwan-control 1 → 2"}, Failures: []string{"abcde timeout"}}
	if err := SaveResult(path, xorCryptor(0x31), plan, result); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("wwan-control")) || bytes.Contains(raw, []byte("abcde")) {
		t.Fatal("application journal leaked package metadata")
	}
	plain, _ := xorCryptor(0x31).Encrypt(raw)
	if !bytes.Contains(plain, []byte("wwan-control")) || !bytes.Contains(plain, []byte("abcde timeout")) {
		t.Fatal("application journal lost substitutions or failures")
	}
}

func TestPackageManagerHangIsBoundedAndReported(t *testing.T) {
	original := packageTimeoutFor
	packageTimeoutFor = func(Source) time.Duration { return 5 * time.Millisecond }
	defer func() { packageTimeoutFor = original }()
	result := Execute(context.Background(), Plan{Install: []Package{{Name: "hung", Source: Official}}}, hangingRunner{}, nil)
	if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "deadline exceeded") {
		t.Fatalf("result=%#v", result)
	}
}
