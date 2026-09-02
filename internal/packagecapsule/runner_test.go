package packagecapsule

import (
	"context"
	"strings"
	"testing"
)

func TestQuietRunnerCapturesBoundedFailureOutput(t *testing.T) {
	err := (ExecRunner{Context: context.Background(), Quiet: true}).Run("sh", "-c", "printf 'package output' >&2; exit 7")
	if err == nil || !strings.Contains(err.Error(), "package output") {
		t.Fatalf("error=%v", err)
	}
	if len(err.Error()) > 2200 {
		t.Fatalf("unbounded worker error: %d bytes", len(err.Error()))
	}
}
