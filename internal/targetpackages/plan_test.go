package targetpackages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/inventory"
)

type fakeRunner struct {
	values map[string]string
	calls  []string
}

func (r *fakeRunner) Output(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, key)
	v, ok := r.values[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	return v, nil
}
func (r *fakeRunner) Run(name string, args ...string) error {
	_, err := r.Output(name, args...)
	return err
}
func (r *fakeRunner) RunDir(dir, name string, args ...string) error { return r.Run(name, args...) }
func TestResolveNativeIntentAndPreserveTarget(t *testing.T) {
	run := &fakeRunner{values: map[string]string{
		"pacman -Qq":             "linux-snapdragon\ninstalled\n",
		"pacman -Si -- editor":   "Name : editor\nVersion : 2\nArchitecture : aarch64\nConflicts With : None\nReplaces : None\n",
		"pacman -Si -- xonly":    "Version : 1\nArchitecture : x86_64\n",
		"pacman -Si -- conflict": "Version : 1\nArchitecture : aarch64\nConflicts With : installed\n",
	}}
	source := inventory.ApplicationManifest{PackagePlan: inventory.PackageRestorePlan{Official: []string{"editor", "linux", "installed", "xonly", "conflict", "missing"}}, Packages: inventory.PackageInventory{OfficialInstalled: []inventory.InstalledPackage{{Name: "unused-x64-dependency"}}}}
	p := Resolve(source, "arm64", "target", run)
	if strings.Join(p.Official, ",") != "editor" || !strings.Contains(strings.Join(p.Kept, ","), "installed") || p.Versions["editor"] != "2" {
		t.Fatalf("%+v", p)
	}
	if len(p.Review) != 3 {
		t.Fatalf("review=%v", p.Review)
	}
	for _, call := range run.calls {
		if strings.Contains(call, "unused-x64") || strings.Contains(call, "-- linux") {
			t.Fatal(call)
		}
	}
}
func TestTransactionRejectsTargetUpgrade(t *testing.T) {
	run := &fakeRunner{values: map[string]string{"pacman -Si -- libc": "Name : libc", "pacman -Si -- editor": "Name : editor", "pacman -Qq": "libc", "pacman -Sp --needed --print-format %n -- editor": "libc\neditor"}}
	if err := CheckTransaction(run, []string{"editor"}); err == nil {
		t.Fatal("allowed target baseline upgrade")
	}
	run.values["pacman -Sp --needed --print-format %n -- editor"] = "editor"
	if err := CheckTransaction(run, []string{"editor"}); err != nil {
		t.Fatal(err)
	}
}
func TestInventoryDigestIgnoresOrderAndCaptureTime(t *testing.T) {
	a := inventory.ApplicationManifest{PackagePlan: inventory.PackageRestorePlan{Official: []string{"one", "two"}}}
	b := a
	b.PackagePlan.Official = []string{"two", "one"}
	if Digest(a) != Digest(b) {
		t.Fatal("unstable digest")
	}
	b.PackagePlan.Official = append(b.PackagePlan.Official, "three")
	if Digest(a) == Digest(b) {
		t.Fatal("missing coverage change")
	}
}
func TestIsolatedRunnerNeverUsesHostDatabase(t *testing.T) {
	root := t.TempDir()
	base := &fakeRunner{values: map[string]string{}}
	r := IsolatedRunner{Base: base, Root: root, Environment: Environment{Architecture: "aarch64"}}
	if out, err := r.Output("pacman", "-Qq"); err != nil || out != "" || len(base.calls) != 0 {
		t.Fatal("host inventory leaked")
	}
	_, _ = r.Output("pacman", "-Si", "--", "editor")
	if len(base.calls) != 1 || !strings.Contains(base.calls[0], "--dbpath "+root+"/db") || !strings.Contains(base.calls[0], "--arch aarch64") {
		t.Fatal(base.calls)
	}
}
