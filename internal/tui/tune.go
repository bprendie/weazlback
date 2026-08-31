package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type tuneConnectionMsg struct {
	connection int
	active     bool
	events     <-chan tea.Msg
}

type tuneConnectionsDoneMsg struct {
	tuning restic.ConnectionTuning
	err    error
}

type tuneBandwidthMsg struct {
	written int64
	total   int64
	elapsed time.Duration
	events  <-chan tea.Msg
}

type tuneBandwidthDoneMsg struct {
	probe restic.UploadProbe
	err   error
}

type tuneTickMsg struct{}

func (m Model) startTune() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy, m.operation = cancel, true, "repository tune"
	m.tuneStage, m.tuneTrials, m.tuneFrame, m.err = "connections", nil, 0, ""
	m.status = "measuring repository connections"
	events := make(chan tea.Msg, 8)
	go runTUIConnectionTune(ctx, repo, m.cfg.Machine.ID, events)
	return m, tea.Batch(waitTuneEvent(events), tuneTick())
}

func runTUIConnectionTune(ctx context.Context, repo restic.Repository, machineID string, events chan tea.Msg) {
	defer close(events)
	service := restic.NewService(io.Discard)
	snapshots, err := service.SnapshotsForMachine(ctx, repo, machineID)
	if err != nil {
		events <- tuneConnectionsDoneMsg{err: err}
		return
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
	var snapshot *restic.Snapshot
	for i := range snapshots {
		if hasTuneCoreTag(snapshots[i].Tags) {
			snapshot = &snapshots[i]
			break
		}
	}
	if snapshot == nil {
		events <- tuneConnectionsDoneMsg{err: errors.New("connection tuning requires a Core Restore Point")}
		return
	}
	files, err := service.Files(ctx, repo, snapshot.ID)
	if err != nil {
		events <- tuneConnectionsDoneMsg{err: err}
		return
	}
	workDir, err := os.MkdirTemp("", "weazlback-tui-tune-*")
	if err != nil {
		events <- tuneConnectionsDoneMsg{err: err}
		return
	}
	defer os.RemoveAll(workDir)
	tuning := service.TuneRestoreConnectionsWithProgress(ctx, repo, snapshot.ID, files, workDir, func(connection int, active bool) {
		events <- tuneConnectionMsg{connection: connection, active: active, events: events}
	})
	events <- tuneConnectionsDoneMsg{tuning: tuning}
}

func hasTuneCoreTag(tags []string) bool {
	for _, tag := range tags {
		if tag == "profile:core" {
			return true
		}
	}
	return false
}

func (m Model) tuneConnectionsFinished(msg tuneConnectionsDoneMsg) (tea.Model, tea.Cmd) {
	m.busy, m.cancel, m.tuneActiveConnection = false, nil, 0
	if msg.err != nil {
		m.tuneStage, m.err, m.status = "", msg.err.Error(), "connection tuning failed"
		return m, nil
	}
	m.tuneTrials, m.tuneConnection = msg.tuning.Trials, msg.tuning.Selected
	m.tuneStage, m.status = "choose-connections", "choose measured repository concurrency"
	return m, nil
}

func (m Model) updateTuneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if (msg.String() == "tab" || msg.String() == "shift+tab") && !m.busy {
		m.railFocused = msg.String() == "tab"
		return m, nil
	}
	if msg.String() == "q" {
		if os.Getenv("TMUX") != "" {
			m.status = "detaching — tuning state remains in the tmux backend"
			return m, detachClientCmd()
		}
		if m.busy {
			m.status = "tuning still running — Ctrl+C cancels it"
			return m, nil
		}
		return m, tea.Quit
	}
	if msg.String() == "ctrl+c" && m.busy && m.cancel != nil {
		m.cancel()
		m.status = "cancelling repository tune"
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.tuneStage {
	case "connections", "bandwidth":
		return m, nil
	case "choose-connections":
		choices := successfulTuneConnections(m.tuneTrials)
		if len(choices) == 0 {
			m.tuneStage, m.err = "", "no connection trial completed successfully"
			return m, nil
		}
		index := connectionChoiceIndex(choices, m.tuneConnection)
		if msg.String() == "up" || msg.String() == "k" {
			index = max(0, index-1)
			m.tuneConnection = choices[index]
		} else if msg.String() == "down" || msg.String() == "j" {
			index = min(len(choices)-1, index+1)
			m.tuneConnection = choices[index]
		} else if msg.String() == "enter" {
			return m.startTuneBandwidth()
		}
	case "choose-bandwidth":
		if msg.String() == "esc" {
			m.tuneStage, m.err = "choose-connections", ""
			m.tuneInput.Blur()
			return m, nil
		}
		if msg.String() == "enter" {
			value, err := strconv.Atoi(strings.TrimSpace(m.tuneInput.Value()))
			if err != nil || value < 0 || value > 1_000_000 {
				m.err = "upload ceiling must be 0–1000000 MiB/s"
				return m, nil
			}
			return m.saveTune(value)
		}
		var cmd tea.Cmd
		m.tuneInput, cmd = m.tuneInput.Update(msg)
		return m, cmd
	case "done":
		if msg.String() == "enter" {
			m.tuneStage = ""
		}
	}
	return m, nil
}

func successfulTuneConnections(trials []restic.ConnectionTrial) []int {
	var values []int
	for _, trial := range trials {
		if trial.Error == "" {
			values = append(values, trial.Connections)
		}
	}
	return values
}

func connectionChoiceIndex(choices []int, selected int) int {
	for i, value := range choices {
		if value == selected {
			return i
		}
	}
	return 0
}

func (m Model) startTuneBandwidth() (tea.Model, tea.Cmd) {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if destination.Kind != "ssh" {
		return m.saveTune(0)
	}
	repo.Connections = m.tuneConnection
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy, m.operation = cancel, true, "bandwidth tune"
	m.tuneStage, m.tuneProbeWritten, m.tuneProbeElapsed, m.err = "bandwidth", 0, 0, ""
	m.status = "uploading ephemeral 100 MiB bandwidth probe"
	events := make(chan tea.Msg, 1)
	go func() {
		defer close(events)
		probe, probeErr := restic.ProbeSFTPUploadWithProgress(ctx, repo, func(written, total int64, elapsed time.Duration) {
			sendLatestOperationEvent(events, tuneBandwidthMsg{written: written, total: total, elapsed: elapsed, events: events})
		})
		sendLatestOperationEvent(events, tuneBandwidthDoneMsg{probe: probe, err: probeErr})
	}()
	return m, waitTuneEvent(events)
}

func (m Model) tuneBandwidthFinished(msg tuneBandwidthDoneMsg) (tea.Model, tea.Cmd) {
	m.busy, m.cancel = false, nil
	if msg.err != nil {
		m.tuneStage, m.err, m.status = "choose-connections", msg.err.Error(), "bandwidth probe failed"
		return m, nil
	}
	m.tuneProbe = msg.probe
	recommended := restic.RecommendedUploadMiB(msg.probe.MiBPerS)
	m.tuneInput = textinput.New()
	m.tuneInput.Prompt = "upload ceiling MiB/s > "
	m.tuneInput.SetValue(strconv.Itoa(recommended))
	m.tuneInput.Focus()
	m.tuneStage, m.status = "choose-bandwidth", "choose aggregate SSH upload ceiling"
	return m, textinput.Blink
}

func (m Model) saveTune(uploadMiB int) (tea.Model, tea.Cmd) {
	destination := m.cfg.Active()
	if destination == nil {
		m.err = "no active destination"
		return m, nil
	}
	destination.Connections = m.tuneConnection
	if destination.Kind == "ssh" {
		destination.UploadLimitKiB = uploadMiB * 1024
	}
	path, err := config.Path()
	if err == nil {
		err = config.Save(path, m.cfg)
	}
	if err != nil {
		m.err, m.status = err.Error(), "tuning save failed"
		return m, nil
	}
	m.tuneInput.Blur()
	m.tuneStage, m.err, m.status = "done", "", "repository tuning saved"
	return m, nil
}

func waitTuneEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return tuneConnectionsDoneMsg{err: errors.New("tuning stream ended unexpectedly")}
		}
		return message
	}
}

func tuneTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tuneTickMsg{} })
}
