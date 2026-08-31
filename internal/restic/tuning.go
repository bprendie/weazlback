package restic

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type ConnectionTrial struct {
	Connections int           `json:"connections"`
	Elapsed     time.Duration `json:"elapsed"`
	Error       string        `json:"error,omitempty"`
}

type ConnectionTuning struct {
	Selected int               `json:"selected"`
	Trials   []ConnectionTrial `json:"trials,omitempty"`
}

func (s Service) TuneRestoreConnections(ctx context.Context, repo Repository, snapshot string, files []FileEntry, workDir string) ConnectionTuning {
	return s.TuneRestoreConnectionsWithProgress(ctx, repo, snapshot, files, workDir, nil)
}

func (s Service) TuneRestoreConnectionsWithProgress(ctx context.Context, repo Repository, snapshot string, files []FileEntry, workDir string, progress func(int, bool)) ConnectionTuning {
	result := ConnectionTuning{Selected: 4}
	paths, bytes := tuningSample(files)
	if len(paths) == 0 || bytes < 4<<20 {
		return result
	}
	for _, connections := range []int{4, 2, 10} {
		target := filepath.Join(workDir, ".tune-"+strconv.Itoa(connections))
		candidate := repo
		candidate.Connections = connections
		if progress != nil {
			progress(connections, true)
		}
		started := time.Now()
		err := s.Restore(ctx, candidate, snapshot, target, paths)
		trial := ConnectionTrial{Connections: connections, Elapsed: time.Since(started)}
		if progress != nil {
			progress(connections, false)
		}
		if err != nil {
			trial.Error = err.Error()
		}
		result.Trials = append(result.Trials, trial)
		_ = os.RemoveAll(target)
	}
	baseline := result.Trials[0]
	if baseline.Error != "" {
		for _, trial := range result.Trials[1:] {
			if trial.Error == "" {
				result.Selected = trial.Connections
				return result
			}
		}
		return result
	}
	best := baseline
	for _, trial := range result.Trials[1:] {
		if trial.Error == "" && trial.Elapsed*100 < best.Elapsed*85 {
			best = trial
		}
	}
	result.Selected = best.Connections
	return result
}

func tuningSample(files []FileEntry) ([]string, uint64) {
	regular := make([]FileEntry, 0, len(files))
	for _, file := range files {
		if file.Type == "file" && file.Size >= 64<<10 && file.Size <= 16<<20 {
			regular = append(regular, file)
		}
	}
	sort.Slice(regular, func(i, j int) bool { return regular[i].Size > regular[j].Size })
	var paths []string
	var bytes uint64
	for _, file := range regular {
		if len(paths) == 16 || bytes >= 16<<20 {
			break
		}
		paths, bytes = append(paths, file.Path), bytes+file.Size
	}
	return paths, bytes
}
