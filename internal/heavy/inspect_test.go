package heavy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectFindsSparseDiskAndLiveWriter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guest.qcow2")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(4 << 30); err != nil {
		t.Fatal(err)
	}
	report := Inspect([]string{root})
	if report.Safe || len(report.Writers) == 0 {
		t.Fatalf("live writer not detected: %+v", report)
	}
	if len(report.Images) != 1 || report.Images[0].Format != "qcow2" || report.Images[0].Logical != 4<<30 {
		t.Fatalf("image inventory=%+v", report.Images)
	}
}

func TestInspectAllowsClosedDisk(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guest.img")
	if err := os.WriteFile(path, []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Inspect([]string{root})
	if !report.Safe || len(report.Writers) != 0 || report.Images[0].Format != "raw-candidate" {
		t.Fatalf("report=%+v", report)
	}
}
