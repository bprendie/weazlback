package freshrestore

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePacmanItem(t *testing.T) {
	for _, test := range []struct {
		line      string
		completed int
		total     int
		current   string
	}{
		{"(  1/122) installing btop", 1, 122, "btop"},
		{"\x1b[1m(79/145)\x1b[0m upgrading linux", 79, 145, "linux"},
		{"(3/3) reinstalling brave-bin", 3, 3, "brave-bin"},
	} {
		completed, total, current, ok := parsePacmanItem(test.line)
		if !ok || completed != test.completed || total != test.total || current != test.current {
			t.Fatalf("parse %q = %d/%d %q %v", test.line, completed, total, current, ok)
		}
	}
}

func TestRunPacmanProgressStreamsTransactionItems(t *testing.T) {
	script := filepath.Join(t.TempDir(), "pacman")
	body := "#!/bin/sh\nprintf '( 1/3) installing alpha\\n'\nprintf '( 2/3) installing beta\\r'\nprintf '( 3/3) installing gamma\\n'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	var events []RestoreProgress
	err := runPacmanProgress(context.Background(), script, nil, "applications", "official packages", []string{"alpha", "beta", "gamma"}, func(value RestoreProgress) {
		events = append(events, value)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Current != "beta" || events[2].Completed != 3 || events[2].Total != 3 {
		t.Fatalf("events=%+v", events)
	}
}

func TestSplitPacmanRecordsHandlesCarriageReturnProgress(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("first\r(2/3) installing second\nthird"))
	scanner.Split(splitPacmanRecords)
	var records []string
	for scanner.Scan() {
		records = append(records, scanner.Text())
	}
	if strings.Join(records, "|") != "first|(2/3) installing second|third" {
		t.Fatalf("records=%q", records)
	}
}
