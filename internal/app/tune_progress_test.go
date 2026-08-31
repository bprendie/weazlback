package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTuneBarReportsPercentRateAndCompletion(t *testing.T) {
	var output bytes.Buffer
	bar := tuneBar{writer: &output}
	bar.update(50<<20, 100<<20, time.Second)
	bar.last = time.Time{}
	bar.update(100<<20, 100<<20, 2*time.Second)
	text := output.String()
	if !strings.Contains(text, "50%") || !strings.Contains(text, "100%") || !strings.Contains(text, "50.0 MiB/s") {
		t.Fatalf("output=%q", text)
	}
}
