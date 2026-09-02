package packagecapsule

import "testing"

func TestResolveDeltaNewerCompatibleVersionWins(t *testing.T) {
	manifest := Manifest{Packages: []Package{
		{Name: "iso-newer", Installed: "1", ArtifactVersion: "1", Artifact: "a", Compatible: true, Source: "official"},
		{Name: "local-newer", Installed: "2", ArtifactVersion: "3", Artifact: "b", Compatible: true, Source: "official"},
		{Name: "missing-foreign", Installed: "1", Source: "foreign"},
		{Name: "equal", Installed: "4", ArtifactVersion: "4", Artifact: "d", Compatible: true, Source: "official"},
	}}
	installed := map[string]string{"iso-newer": "2", "local-newer": "1", "equal": "4"}
	delta := ResolveDelta(manifest, installed, numericCompare)
	if len(delta.Kept) != 2 || len(delta.Local) != 1 || delta.Local[0].Name != "local-newer" {
		t.Fatalf("delta=%+v", delta)
	}
	if len(delta.ForeignOnline) != 1 || delta.ForeignOnline[0] != "missing-foreign" {
		t.Fatalf("foreign=%v", delta.ForeignOnline)
	}
}

func TestResolveDeltaDoesNotUseOlderArtifact(t *testing.T) {
	manifest := Manifest{Packages: []Package{{Name: "demo", Installed: "3", ArtifactVersion: "2", Artifact: "demo", Compatible: true, Source: "official"}}}
	delta := ResolveDelta(manifest, nil, numericCompare)
	if len(delta.Local) != 0 || len(delta.OfficialOnline) != 1 {
		t.Fatalf("delta=%+v", delta)
	}
}

func numericCompare(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
