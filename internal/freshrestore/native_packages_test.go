package freshrestore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/packagecapsule"
)

type nativeFake struct{ versions map[string]string }

func (r nativeFake) Output(name string, args ...string) (string, error) {
	key := args[len(args)-1]
	if v, ok := r.versions[key]; ok {
		return "Version : " + v, nil
	}
	return "", fmt.Errorf("unavailable")
}
func (r nativeFake) Run(string, ...string) error            { return nil }
func (r nativeFake) RunDir(string, string, ...string) error { return nil }
func TestNativeCapsuleCoverageUsesCorrectArchitectureAndVersions(t *testing.T) {
	native := NativePackagePlan{Official: []string{"ready", "stale", "wrong", "missing"}, Build: []string{"aur-ready"}}
	capsule := packagecapsule.Manifest{Packages: []packagecapsule.Package{
		{Name: "ready", Architecture: "aarch64", ArtifactVersion: "2", Artifact: "ready.pkg", Compatible: true, Source: "official"},
		{Name: "stale", Architecture: "aarch64", ArtifactVersion: "1", Artifact: "stale.pkg", Compatible: true, Source: "official"},
		{Name: "wrong", Architecture: "x86_64", ArtifactVersion: "2", Artifact: "wrong.pkg", Compatible: true, Source: "official"},
		{Name: "aur-ready", Architecture: "aarch64", ArtifactVersion: "1", Artifact: "aur.pkg", Compatible: true, Source: "foreign"},
	}}
	r := Restore{Plan: Plan{PackageReview: []string{"aur-ready: needs native build"}}}
	r.applyNativeCapsule(native, capsule, "arm64", nativeFake{versions: map[string]string{"ready": "2", "stale": "2", "wrong": "2"}})
	if len(r.Plan.PackageDelta.Local) != 2 {
		t.Fatalf("local=%+v", r.Plan.PackageDelta.Local)
	}
	if got := strings.Join(r.Plan.Official, ","); got != "stale,wrong,missing" {
		t.Fatal(got)
	}
	if len(r.Plan.PackageReview) != 0 || len(r.Plan.AUR) != 0 {
		t.Fatalf("review=%v aur=%v", r.Plan.PackageReview, r.Plan.AUR)
	}
}
func TestNativeBuildFallbackDoesNotExecuteAUR(t *testing.T) {
	r := Restore{Plan: Plan{Migration: Migration{Active: true}, PackageDelta: packagecapsule.Delta{Local: []packagecapsule.Install{{Name: "custom", Source: "foreign"}}}}}
	r.fallbackAllCapsulePackages()
	if len(r.Plan.AUR) != 0 || len(r.Plan.PackageReview) == 0 {
		t.Fatal("foreign fallback did not require explicit action")
	}
}
