package restic

import "testing"

func TestTuningSampleIsBounded(t *testing.T) {
	files := []FileEntry{{Path: "/tiny", Type: "file", Size: 1}, {Path: "/large", Type: "file", Size: 17 << 20}}
	for i := 0; i < 20; i++ {
		files = append(files, FileEntry{Path: "/sample", Type: "file", Size: 1 << 20})
	}
	paths, bytes := tuningSample(files)
	if len(paths) != 16 || bytes != 16<<20 {
		t.Fatalf("paths=%d bytes=%d", len(paths), bytes)
	}
}
