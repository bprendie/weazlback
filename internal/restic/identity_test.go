package restic

import "testing"

func TestMachineIdentityGroupsAndFiltersLegacyHistory(t *testing.T) {
	snapshots := []Snapshot{
		{ID: "think-1", Hostname: "thinkpad", Tags: []string{"weazlback", "profile:home", "machine:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{ID: "think-2", Hostname: "thinkpad-new", Tags: []string{"weazlback", "profile:core", "machine:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{ID: "hp-1", Hostname: "hp", Tags: []string{"weazlback", "profile:home", "machine:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		{ID: "old", Hostname: "dragonfly", Tags: []string{"weazlback", "profile:home"}},
	}
	groups := GroupIdentities(snapshots, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "ThinkPad")
	if len(groups) != 3 || groups[0].Name != "ThinkPad" || groups[0].Points != 2 {
		t.Fatalf("groups=%#v", groups)
	}
	legacy := FilterIdentity(snapshots, "legacy:dragonfly")
	if len(legacy) != 1 || legacy[0].ID != "old" {
		t.Fatalf("legacy=%#v", legacy)
	}
	hp := FilterIdentity(snapshots, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if len(hp) != 1 || hp[0].ID != "hp-1" {
		t.Fatalf("hp=%#v", hp)
	}
}

func TestSnapshotHealthStates(t *testing.T) {
	for _, test := range []struct {
		tags []string
		want string
	}{{nil, "healthy"}, {[]string{"unverified"}, "unverified"}, {[]string{"incomplete"}, "incomplete"}, {[]string{"failed", "incomplete"}, "failed"}} {
		if got := SnapshotHealth(test.tags); got != test.want {
			t.Fatalf("tags=%v got=%q want=%q", test.tags, got, test.want)
		}
	}
}
