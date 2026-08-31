package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTuneConnectionChoiceUsesMeasuredResults(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeTune, tuneStage: "choose-connections", tuneConnection: 4,
		tuneTrials: []restic.ConnectionTrial{{Connections: 4, Elapsed: time.Second}, {Connections: 2, Elapsed: 2 * time.Second}, {Connections: 10, Elapsed: 900 * time.Millisecond}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.tuneConnection != 2 {
		t.Fatalf("connection=%d", got.tuneConnection)
	}
	if view := got.tuneScreen(); !strings.Contains(view, "CONNECTION REPORT") || !strings.Contains(view, ">  2 connections") {
		t.Fatalf("view=%q", view)
	}
}

func TestTuneBandwidthViewShowsLiveEvidence(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeTune, tuneStage: "bandwidth", tuneProbeWritten: 50 << 20, tuneProbeElapsed: time.Second}
	view := m.tuneScreen()
	if !strings.Contains(view, "50%") || !strings.Contains(view, "50.0 / 100 MiB") || !strings.Contains(view, "50.0 MiB/s") {
		t.Fatalf("view=%q", view)
	}
}
