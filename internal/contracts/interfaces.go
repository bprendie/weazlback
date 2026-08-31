package contracts

import (
	"context"
	"io"
)

type Engine interface {
	Name() string
	Initialize(context.Context, Destination, SecretSource) error
	Backup(context.Context, Destination, BackupRequest, chan<- Progress) (Snapshot, error)
	Snapshots(context.Context, Destination) ([]Snapshot, error)
	Restore(context.Context, Destination, RestoreRequest, chan<- Progress) error
	Check(context.Context, Destination, bool) error
	Prune(context.Context, Destination, Retention) error
}

type Transport interface {
	Probe(context.Context, Destination) error
	PinnedHostKey(context.Context, Destination) (string, error)
}

type Vault interface {
	Exists() (bool, error)
	Create([]byte) error
	Unlock([]byte) error
	Lock()
	Put(string, []byte) error
	Get(string) ([]byte, error)
	Unlocked() bool
}

type Inventory interface {
	Capture(context.Context) (MachineManifest, error)
}

type StatusStore interface {
	Load() (Status, error)
	Save(Status) error
}

type SecretSource interface {
	WriteSecret(io.Writer) error
}

type BackupRequest struct {
	Profile string
	Paths   []string
	DryRun  bool
}

type RestoreRequest struct {
	SnapshotID string
	Target     string
	Hostname   string
}

type Retention struct {
	Hourly  int
	Daily   int
	Weekly  int
	Monthly int
}
