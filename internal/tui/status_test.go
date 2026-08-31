package tui

import (
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	statusstore "github.com/bprendie/weazlback/internal/status"
)

func TestTUIBackupPublishesSelectedProfileLane(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := Model{selectedProfile: "home", cfg: config.Config{ActiveDestination: "remote",
		Destinations: []config.Destination{{ID: "remote"}}}, progress: restic.BackupProgress{
		MessageType: "status", PercentDone: 0.42, FilesDone: 42, TotalFiles: 100,
	}}
	m.publishOperationStatus("backing-up", "")
	path, _ := statusstore.DefaultPath()
	value, err := (statusstore.Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Profiles) != 1 || value.Profiles[0].Profile != "HOME" || value.Profiles[0].Percent != 0.42 {
		t.Fatalf("profiles=%+v", value.Profiles)
	}
}
