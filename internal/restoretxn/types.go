package restoretxn

import (
	"context"
	"time"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/restic"
)

type ConflictPolicy string

const (
	ReplacePreserving ConflictPolicy = "replace-preserving"
	OverlayPreserving ConflictPolicy = "safe-overlay"
	SkipExisting      ConflictPolicy = "skip-existing"
)

type Item struct {
	SourcePath string           `json:"source_path"`
	TargetPath string           `json:"target_path"`
	Entry      restic.FileEntry `json:"entry"`
}

type Plan struct {
	ID, Snapshot, SourceMachineID, TargetMachineID string
	Repository                                     restic.Repository
	Items                                          []Item
	StageRoot, JournalPath                         string
	Conflict                                       ConflictPolicy
	StagingOnly                                    bool
	SourceUID, SourceGID, TargetUID, TargetGID     uint32
}

type Preflight struct {
	Files, Symlinks        int
	BytesRequired          uint64
	BytesAvailable         uint64
	CrossFilesystem        bool
	MountBoundaries        []string
	OwnershipMappingNeeded bool
}

type Progress struct {
	Phase                    string
	FilesDone, FilesTotal    uint64
	BytesDone, BytesTotal    uint64
	BytesPerSecond           float64
	Elapsed, EstimatedRemain time.Duration
}

type Result struct {
	JournalPath   string
	StagedAt      string
	Placed        []string
	Rollback      []string
	Skipped       []string
	BrowserRepair browserrepair.Result
}

type Service interface {
	Check(context.Context, restic.Repository, bool) error
	FilesAt(context.Context, restic.Repository, string, []string) ([]restic.FileEntry, error)
	RestoreWithProgress(context.Context, restic.Repository, string, string, []string, func(restic.RestoreProgress)) error
}

type Cryptor interface {
	Encrypt([]byte) ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}
