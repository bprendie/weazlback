package tui

import (
	"os"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/contracts"
	statusstore "github.com/bprendie/weazlback/internal/status"
)

func (m Model) publishOperationStatus(state, message string) {
	path, err := statusstore.DefaultPath()
	if err != nil {
		return
	}
	store := statusstore.Store{Path: path}
	current, _ := store.Load()
	destination := ""
	if len(m.cfg.Destinations) > 0 {
		destination = m.cfg.Active().ID
	}
	value := contracts.Status{State: state, Destination: destination, LastHealthy: current.LastHealthy, Error: message}
	if m.incomplete {
		value.Incomplete, value.Skipped, value.Manifest = true, uint64(len(m.skippedPaths)), m.skippedManifest
	}
	if state == "backing-up" {
		value.OperationPID = os.Getpid()
		value.Progress = &contracts.Progress{Phase: m.progress.MessageType, Files: m.progress.FilesDone,
			TotalFiles: m.progress.TotalFiles, LogicalBytes: m.progress.TotalBytes,
			UploadedBytes: m.progress.BytesDone, Percent: m.progress.PercentDone,
			Elapsed:            time.Duration(m.progress.SecondsElapsed) * time.Second,
			ETA:                time.Duration(m.progress.SecondsRemaining) * time.Second,
			WireBytesPerSecond: m.progress.WireBytesPerSecond}
		value.Profiles = []contracts.ProfileProgress{{Profile: strings.ToUpper(m.selectedProfile), State: "backing-up",
			Percent: m.progress.PercentDone, Files: m.progress.FilesDone, Total: m.progress.TotalFiles,
			Bytes: m.progress.BytesDone, TotalBytes: m.progress.TotalBytes,
			FilesPerSecond: ratePerSecond(m.progress.FilesDone, m.progress.SecondsElapsed)}}
	}
	if state == "healthy" {
		now := time.Now()
		value.LastHealthy = &now
	}
	_ = store.Save(value)
}

func ratePerSecond(value, seconds uint64) float64 {
	if seconds == 0 {
		return 0
	}
	return float64(value) / float64(seconds)
}
