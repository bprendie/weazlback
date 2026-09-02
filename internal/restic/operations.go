package restic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type Snapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Tags     []string  `json:"tags"`
	Paths    []string  `json:"paths"`
}

type FileEntry struct {
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Path    string    `json:"path"`
	Size    uint64    `json:"size"`
	Mode    uint32    `json:"mode"`
	UID     uint32    `json:"uid"`
	GID     uint32    `json:"gid"`
	ModTime time.Time `json:"mtime"`
}

type Service struct{ Runner Runner }

type BackupProgress struct {
	MessageType        string  `json:"message_type"`
	PercentDone        float64 `json:"percent_done"`
	FilesDone          uint64  `json:"files_done"`
	TotalFiles         uint64  `json:"total_files"`
	BytesDone          uint64  `json:"bytes_done"`
	TotalBytes         uint64  `json:"total_bytes"`
	SecondsElapsed     uint64  `json:"seconds_elapsed"`
	SecondsRemaining   uint64  `json:"seconds_remaining"`
	WireBytesPerSecond float64 `json:"-"`
}

type RestoreProgress struct {
	MessageType   string  `json:"message_type"`
	PercentDone   float64 `json:"percent_done"`
	TotalFiles    uint64  `json:"total_files"`
	FilesRestored uint64  `json:"files_restored"`
	TotalBytes    uint64  `json:"total_bytes"`
	BytesRestored uint64  `json:"bytes_restored"`
}

type Key struct {
	ID      string `json:"id"`
	Current bool   `json:"current"`
}

func NewService(stderr io.Writer) Service { return Service{Runner: New(stderr)} }

func (s Service) Initialize(ctx context.Context, repo Repository) error {
	_, err := s.Runner.Run(ctx, repo, "init")
	return err
}

func (s Service) RepositoryID(ctx context.Context, repo Repository) (string, error) {
	var value struct {
		ID string `json:"id"`
	}
	if err := s.Runner.JSON(ctx, repo, &value, "cat", "config"); err != nil {
		return "", err
	}
	if value.ID == "" {
		return "", errors.New("repository configuration has no ID")
	}
	return value.ID, nil
}

func (s Service) Backup(ctx context.Context, repo Repository, profile string, paths, excludes []string, dryRun bool) error {
	return s.BackupMachineWithProgress(ctx, repo, profile, "", paths, excludes, dryRun, false, nil)
}

func (s Service) BackupWithProgress(ctx context.Context, repo Repository, profile string, paths, excludes []string, dryRun, incomplete bool, progress func(BackupProgress)) error {
	return s.BackupMachineWithProgress(ctx, repo, profile, "", paths, excludes, dryRun, incomplete, progress)
}

func (s Service) BackupMachineWithProgress(ctx context.Context, repo Repository, profile, machineID string, paths, excludes []string, dryRun, incomplete bool, progress func(BackupProgress)) error {
	if len(paths) == 0 {
		return fmt.Errorf("backup profile %q has no paths", profile)
	}
	// Restic's scan supplies the file and byte totals used for an honest
	// percentage and ETA. --no-scan is faster to start, but makes progress
	// indeterminate and is therefore intentionally not used here.
	args := []string{"backup", "--json", "--tag", "weazlback", "--tag", "profile:" + profile}
	if machineID != "" {
		args = append(args, "--tag", "machine:"+machineID)
	}
	if incomplete {
		args = append(args, "--tag", "incomplete")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	for _, exclude := range excludes {
		args = append(args, "--exclude", exclude)
	}
	args = append(args, paths...)
	stream := func() error {
		return s.Runner.Stream(ctx, repo, func(line []byte) {
			var event BackupProgress
			if json.Unmarshal(line, &event) == nil && event.MessageType == "status" && progress != nil {
				progress(event)
			}
		}, args...)
	}
	backupErr := stream()
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cleanupErr := s.UnlockStale(cleanupCtx, repo)
	cancel()
	if backupErr != nil {
		if cleanupErr != nil {
			return fmt.Errorf("%w; post-backup stale-lock cleanup failed: %v", backupErr, cleanupErr)
		}
		return backupErr
	}
	if cleanupErr != nil {
		return fmt.Errorf("backup completed but stale-lock cleanup failed: %w", cleanupErr)
	}
	return nil
}

func (s Service) Snapshots(ctx context.Context, repo Repository) ([]Snapshot, error) {
	var snapshots []Snapshot
	err := s.Runner.JSON(ctx, repo, &snapshots, "snapshots", "--json", "--tag", "weazlback")
	return snapshots, err
}

func (s Service) SnapshotsForMachine(ctx context.Context, repo Repository, machineID string) ([]Snapshot, error) {
	args := []string{"snapshots", "--json", "--tag", "weazlback"}
	if machineID != "" {
		args = append(args, "--tag", "machine:"+machineID)
	}
	var snapshots []Snapshot
	err := s.Runner.JSON(ctx, repo, &snapshots, args...)
	return snapshots, err
}

func (s Service) Restore(ctx context.Context, repo Repository, snapshot, target string, include []string) error {
	return s.RestoreWithProgress(ctx, repo, snapshot, target, include, nil)
}

func (s Service) RestoreWithProgress(ctx context.Context, repo Repository, snapshot, target string, include []string, progress func(RestoreProgress)) error {
	args := []string{"restore", snapshot, "--target", target, "--json", "--sparse"}
	for _, path := range include {
		args = append(args, "--include", path)
	}
	return s.Runner.Stream(ctx, repo, func(line []byte) {
		var event RestoreProgress
		if json.Unmarshal(line, &event) == nil && progress != nil && (event.MessageType == "status" || event.MessageType == "summary") {
			progress(event)
		}
	}, args...)
}

func (s Service) Files(ctx context.Context, repo Repository, snapshot string) ([]FileEntry, error) {
	return s.FilesAt(ctx, repo, snapshot, nil)
}

func (s Service) FilesAt(ctx context.Context, repo Repository, snapshot string, paths []string) ([]FileEntry, error) {
	args := []string{"ls", snapshot, "--json"}
	args = append(args, paths...)
	data, err := s.Runner.Run(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	var files []FileEntry
	for decoder.More() {
		var envelope struct {
			StructType string `json:"struct_type"`
			FileEntry
		}
		if err := decoder.Decode(&envelope); err != nil {
			return nil, err
		}
		if envelope.StructType == "node" {
			files = append(files, envelope.FileEntry)
		}
	}
	return files, nil
}

func (s Service) Dump(ctx context.Context, repo Repository, snapshot, path string) ([]byte, error) {
	return s.Runner.Run(ctx, repo, "dump", snapshot, path)
}

func (s Service) Check(ctx context.Context, repo Repository, readData bool) error {
	args := []string{"check"}
	if readData {
		args = append(args, "--read-data")
	}
	_, err := s.Runner.Run(ctx, repo, args...)
	return err
}

// UnlockStale asks Restic to remove only locks whose owning process is no longer
// active. Restic's default unlock deliberately preserves a live repository lock.
func (s Service) UnlockStale(ctx context.Context, repo Repository) error {
	_, err := s.Runner.Run(ctx, repo, "unlock")
	return err
}

func repositoryLocked(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "repository is already locked") || strings.Contains(text, "unable to create lock in backend")
}

func (s Service) AddPassword(ctx context.Context, repo Repository, newPassword []byte) ([]Key, error) {
	if repo.Elevated {
		return nil, fmt.Errorf("repository key rotation requires user-owned repository access")
	}
	if _, err := s.Runner.RunWithNewPassword(ctx, repo, newPassword, "key", "add", "--new-password-file", "/proc/self/fd/4"); err != nil {
		return nil, err
	}
	var keys []Key
	err := s.Runner.JSON(ctx, repo, &keys, "key", "list", "--json")
	return keys, err
}

func (s Service) RemoveKey(ctx context.Context, repo Repository, id string) error {
	_, err := s.Runner.Run(ctx, repo, "key", "remove", id)
	return err
}

func (s Service) TagMachine(ctx context.Context, repo Repository, machineID string, snapshotIDs []string) error {
	if machineID == "" || len(snapshotIDs) == 0 {
		return errors.New("machine ID and Restore Point IDs are required")
	}
	args := []string{"tag", "--add", "machine:" + machineID}
	args = append(args, snapshotIDs...)
	_, err := s.Runner.Run(ctx, repo, args...)
	return err
}

func (s Service) Prune(ctx context.Context, repo Repository, hourly, daily, weekly, monthly int) error {
	return s.PruneProfile(ctx, repo, "", hourly, daily, weekly, monthly)
}

func (s Service) PruneProfile(ctx context.Context, repo Repository, profile string, hourly, daily, weekly, monthly int) error {
	return s.PruneMachineProfile(ctx, repo, "", profile, hourly, daily, weekly, monthly)
}

func (s Service) PruneMachineProfile(ctx context.Context, repo Repository, machineID, profile string, hourly, daily, weekly, monthly int) error {
	tags := "weazlback"
	if profile != "" {
		tags += ",profile:" + profile
	}
	if machineID != "" {
		tags += ",machine:" + machineID
	}
	args := []string{"forget", "--prune", "--tag", tags,
		"--keep-hourly", fmt.Sprint(hourly), "--keep-daily", fmt.Sprint(daily),
		"--keep-weekly", fmt.Sprint(weekly), "--keep-monthly", fmt.Sprint(monthly)}
	_, err := s.Runner.Run(ctx, repo, args...)
	return err
}

func DecodeBackupSummary(stream []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytesReader(stream))
	var summary map[string]any
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			return nil, err
		}
		if event["message_type"] == "summary" {
			summary = event
		}
	}
	return summary, nil
}

type byteReader struct{ data []byte }

func bytesReader(data []byte) *byteReader { return &byteReader{data: data} }
func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
