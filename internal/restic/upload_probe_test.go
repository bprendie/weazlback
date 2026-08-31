package restic

import (
	"bytes"
	"testing"
	"time"
)

func TestRecommendedUploadMiBUsesSeventyNinePercent(t *testing.T) {
	if got := RecommendedUploadMiB(100); got != 79 {
		t.Fatalf("recommendation=%d", got)
	}
	if got := RecommendedUploadMiB(0.5); got != 1 {
		t.Fatalf("minimum recommendation=%d", got)
	}
}

func TestParseSFTPLocation(t *testing.T) {
	user, host, root, err := parseSFTPLocation("sftp:weazlback@backuper.local:/srv/weazlback/repositories/id")
	if err != nil || user != "weazlback" || host != "backuper.local" || root != "/srv/weazlback/repositories/id" {
		t.Fatalf("user=%q host=%q root=%q err=%v", user, host, root, err)
	}
}

func TestUploadProgressReaderReportsBytes(t *testing.T) {
	target := bytes.NewBufferString("data")
	var written, total int64
	reader := uploadProgressReader{reader: target, total: 4, started: time.Now(), progress: func(current, size int64, _ time.Duration) {
		written, total = current, size
	}}
	value := make([]byte, 4)
	if _, err := reader.Read(value); err != nil {
		t.Fatal(err)
	}
	if written != 4 || total != 4 || string(value) != "data" {
		t.Fatalf("written=%d total=%d value=%q", written, total, value)
	}
}
