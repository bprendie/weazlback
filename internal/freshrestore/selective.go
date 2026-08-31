package freshrestore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/bprendie/weazlback/internal/securelog"
)

type RecoveryPointFiles struct {
	Snapshot restic.Snapshot
	Files    []restic.FileEntry
}

type SelectiveOptions struct {
	RecoveryPath, Destination, MachineID, Snapshot, SourcePath string
	TargetHome, TargetMachineID, Repository, WorkDir           string
	Passphrase                                                 []byte
	Progress                                                   func(restoretxn.Progress)
}

func RecoveryPoints(ctx context.Context, kit string, passphrase []byte, destination, machineID, profile string) ([]restic.Snapshot, error) {
	session, err := OpenSessionDestinationAt(kit, passphrase, destination, "")
	if err != nil {
		return nil, err
	}
	defer session.Close()
	points, err := restic.NewService(io.Discard).SnapshotsForMachine(ctx, session.Repository, machineID)
	if err != nil {
		return nil, err
	}
	if profile != "" {
		filtered := points[:0]
		for _, point := range points {
			if restic.Profile(point.Tags) == profile && restic.SnapshotHealth(point.Tags) == "healthy" {
				filtered = append(filtered, point)
			}
		}
		points = filtered
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time.After(points[j].Time) })
	return points, nil
}

func RecoveryFiles(ctx context.Context, kit string, passphrase []byte, destination, snapshot string) (RecoveryPointFiles, error) {
	session, err := OpenSessionDestinationAt(kit, passphrase, destination, "")
	if err != nil {
		return RecoveryPointFiles{}, err
	}
	defer session.Close()
	service := restic.NewService(io.Discard)
	points, err := service.Snapshots(ctx, session.Repository)
	if err != nil {
		return RecoveryPointFiles{}, err
	}
	var point restic.Snapshot
	for _, candidate := range points {
		if candidate.ID == snapshot || candidate.ShortID == snapshot {
			point = candidate
			break
		}
	}
	if point.ID == "" {
		return RecoveryPointFiles{}, fmt.Errorf("Restore Point %q was not found", snapshot)
	}
	files, err := service.Files(ctx, session.Repository, point.ID)
	return RecoveryPointFiles{Snapshot: point, Files: files}, err
}

func RestoreRecoverySelection(ctx context.Context, options SelectiveOptions) (restoretxn.Result, error) {
	session, err := OpenSessionDestinationAt(options.RecoveryPath, options.Passphrase, options.Destination, options.Repository)
	if err != nil {
		return restoretxn.Result{}, err
	}
	defer session.Close()
	service := restic.NewService(io.Discard)
	if err := service.Check(ctx, session.Repository, false); err != nil {
		return restoretxn.Result{}, err
	}
	targetHome := options.TargetHome
	if targetHome == "" {
		targetHome, _ = os.UserHomeDir()
	}
	target := mapRecoveryPath(options.SourcePath, targetHome)
	work := options.WorkDir
	if work == "" {
		work = filepath.Join(os.TempDir(), fmt.Sprintf("weazlback-%d", os.Getuid()), "selective")
	}
	id := fmt.Sprintf("recovery-selective-%d", time.Now().UnixNano())
	plan := restoretxn.Plan{ID: id, Snapshot: options.Snapshot, SourceMachineID: options.MachineID,
		TargetMachineID: options.TargetMachineID, Repository: session.Repository,
		Items:     []restoretxn.Item{{SourcePath: filepath.Clean(options.SourcePath), TargetPath: target}},
		StageRoot: filepath.Join(work, id+"-stage"), JournalPath: filepath.Join(work, "journals", id+".enc"),
		Conflict: restoretxn.ReplacePreserving, TargetUID: uint32(os.Getuid()), TargetGID: uint32(os.Getgid())}
	result, runErr := (restoretxn.Engine{Service: service, Cryptor: session.Vault}).Run(ctx, plan, options.Progress)
	payload, _ := json.Marshal(struct {
		Result restoretxn.Result `json:"result"`
		Error  string            `json:"error,omitempty"`
	}{Result: result, Error: errorString(runErr)})
	_, _ = securelog.Write(session.Vault, "restore", id, payload)
	return result, runErr
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func mapRecoveryPath(source, targetHome string) string {
	clean := filepath.Clean(source)
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) >= 3 && parts[0] == "home" {
		return filepath.Join(targetHome, filepath.Join(parts[2:]...))
	}
	return clean
}
