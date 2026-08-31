package app

import (
	"errors"
	"testing"
)

func TestRetryableBackupError(t *testing.T) {
	for _, message := range []string{"connection refused", "network unreachable", "i/o timeout", "reset by peer"} {
		if !retryableBackupError(errors.New(message)) {
			t.Fatalf("expected retryable: %s", message)
		}
	}
	if retryableBackupError(errors.New("repository password is invalid")) {
		t.Fatal("authentication/configuration errors must not retry")
	}
}

func TestScheduleLockExcludesOverlap(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	first, err := acquireScheduleLock()
	if err != nil || first == nil {
		t.Fatalf("first lock: %v", err)
	}
	defer releaseScheduleLock(first)
	second, err := acquireScheduleLock()
	if err != nil {
		t.Fatal(err)
	}
	if second != nil {
		releaseScheduleLock(second)
		t.Fatal("overlapping schedule acquired lock")
	}
}
