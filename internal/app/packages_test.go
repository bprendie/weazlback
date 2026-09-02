package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
)

func TestPackageScheduleCommandPersistsIndependentPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	t.Setenv("WEAZLBACK_CONFIG", path)
	if err := config.Save(path, config.Default()); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := packageScheduleCommand([]string{"--days", "14"}, &output, &output); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.PackagePolicy.Scheduled || loaded.PackagePolicy.IntervalDays != 14 {
		t.Fatalf("policy=%+v", loaded.PackagePolicy)
	}
	if !strings.Contains(output.String(), "14 days") {
		t.Fatalf("output=%q", output.String())
	}
	if err := packageScheduleCommand([]string{"--days", "0"}, &output, &output); err != nil {
		t.Fatal(err)
	}
	loaded, _ = config.Load(path)
	if loaded.PackagePolicy.Scheduled {
		t.Fatal("schedule was not disabled")
	}
}
