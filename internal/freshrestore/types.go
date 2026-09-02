package freshrestore

import (
	"time"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

const JournalSchemaVersion = 2

type Options struct {
	RecoveryPath          string
	Destination           string
	Passphrase            []byte
	Snapshot              string
	Hostname              string
	TargetHome            string
	WorkDir               string
	Yes                   bool
	DryRun                bool
	Connections           int
	Repository            string
	AdoptLocal            bool
	Progress              func(RestoreProgress)
	Scope                 string
	MachineID             string
	AdoptSourceIdentity   bool
	TargetMachineID       string
	PersistTargetIdentity bool
	BrowserProcesses      browserrepair.ProcessChecker
	Engine                string
	TurboPolicy           TurboPolicy
	RestoreEngine         RestoreEngine
	FallbackEngine        RestoreEngine
}

type TurboPolicy struct {
	MemoryPercent int  `json:"memory_percent"`
	BreakGlass    bool `json:"break_glass"`
	Recompress    bool `json:"recompress"`
	FullLink      bool `json:"full_link"`
}

type ResourceBudget struct {
	MemoryAvailable uint64 `json:"memory_available"`
	MemoryLimit     uint64 `json:"memory_limit,omitempty"`
	MemoryBudget    uint64 `json:"memory_budget"`
}

type Qualification struct {
	Eligible           bool           `json:"eligible"`
	SourceTransport    string         `json:"source_transport"`
	SourceFilesystem   string         `json:"source_filesystem,omitempty"`
	SourceMount        string         `json:"source_mount,omitempty"`
	SourceDevice       string         `json:"source_device,omitempty"`
	SourceReadAheadKiB int            `json:"source_read_ahead_kib,omitempty"`
	TargetFilesystem   string         `json:"target_filesystem"`
	TargetMount        string         `json:"target_mount"`
	TargetDevice       string         `json:"target_device"`
	TargetReadAheadKiB int            `json:"target_read_ahead_kib,omitempty"`
	PreservesOwnership bool           `json:"preserves_ownership"`
	PreservesXattrs    bool           `json:"preserves_xattrs"`
	PreservesACLs      bool           `json:"preserves_acls"`
	PreservesSparse    bool           `json:"preserves_sparse"`
	FreeBytes          uint64         `json:"free_bytes"`
	Budget             ResourceBudget `json:"resource_budget"`
	HardFailures       []string       `json:"hard_failures,omitempty"`
	SoftFindings       []string       `json:"soft_findings,omitempty"`
	BreakGlassApplied  bool           `json:"break_glass_applied,omitempty"`
}

type RecoveryTiming struct {
	StartedAt      time.Time     `json:"started_at,omitempty"`
	UsableAt       time.Time     `json:"usable_at,omitempty"`
	DurableAt      time.Time     `json:"durable_at,omitempty"`
	TimeToUsable   time.Duration `json:"time_to_usable_ns,omitempty"`
	TimeToDurable  time.Duration `json:"time_to_durable_ns,omitempty"`
	PackagesDoneAt time.Time     `json:"packages_done_at,omitempty"`
}

type RestoreProgress struct {
	Phase, Lane, Current     string
	Completed, Failed, Total int
	BytesDone, BytesTotal    uint64
	BytesPerSecond           float64
	WireBytes                uint64
	WireBytesPerSecond       float64
	FilesPerSecond           float64
	MemoryUsed               uint64
	QueueDepth               int
}

type Session struct {
	Config      config.Config
	Destination config.Destination
	Repository  restic.Repository
	Vault       *vault.File
	PrivateDir  string
}

type Plan struct {
	SourceMachineID       string                         `json:"source_machine_id"`
	TargetMachineID       string                         `json:"target_machine_id"`
	AdoptSourceIdentity   bool                           `json:"adopt_source_identity"`
	PersistTargetIdentity bool                           `json:"persist_target_identity"`
	Snapshot              restic.Snapshot                `json:"snapshot"`
	HomeSnapshot          *restic.Snapshot               `json:"home_snapshot,omitempty"`
	HeavySnapshot         *restic.Snapshot               `json:"heavy_snapshot,omitempty"`
	PackageSnapshot       *restic.Snapshot               `json:"package_snapshot,omitempty"`
	Scope                 string                         `json:"scope"`
	OriginalHome          string                         `json:"original_home"`
	TargetHome            string                         `json:"target_home"`
	Hostname              string                         `json:"hostname"`
	SourceHostname        string                         `json:"source_hostname"`
	SourceUID             uint32                         `json:"source_uid"`
	SourceGID             uint32                         `json:"source_gid"`
	TargetUID             uint32                         `json:"target_uid"`
	TargetGID             uint32                         `json:"target_gid"`
	Applications          *inventory.ApplicationManifest `json:"applications,omitempty"`
	PackageCapsule        *packagecapsule.Manifest       `json:"-"`
	PackageManifestPath   string                         `json:"package_manifest_path,omitempty"`
	PackageDelta          packagecapsule.Delta           `json:"package_delta,omitempty"`
	CapsuleArtifactFiles  map[string]string              `json:"-"`
	CapsuleFallbackReason string                         `json:"capsule_fallback_reason,omitempty"`
	Official              []string                       `json:"official_packages,omitempty"`
	AUR                   []string                       `json:"aur_packages,omitempty"`
	Flatpak               []string                       `json:"flatpaks,omitempty"`
	SystemServices        []string                       `json:"system_services,omitempty"`
	UserServices          []string                       `json:"user_services,omitempty"`
	LocalApps             []string                       `json:"local_apps,omitempty"`
	PlacementPaths        []string                       `json:"placement_paths,omitempty"`
	HeavyPlacementPaths   []string                       `json:"heavy_placement_paths,omitempty"`
	ArtifactFiles         map[string]string              `json:"artifact_files,omitempty"`
}

type Journal struct {
	SchemaVersion         int                  `json:"schema_version"`
	RepositoryID          string               `json:"repository_id"`
	SnapshotID            string               `json:"snapshot_id"`
	HomeSnapshotID        string               `json:"home_snapshot_id,omitempty"`
	HeavySnapshotID       string               `json:"heavy_snapshot_id,omitempty"`
	PackageSnapshotID     string               `json:"package_snapshot_id,omitempty"`
	Scope                 string               `json:"scope,omitempty"`
	Stage                 string               `json:"stage"`
	Hostname              string               `json:"hostname"`
	TargetHome            string               `json:"target_home"`
	UpdatedAt             time.Time            `json:"updated_at"`
	Connections           int                  `json:"connections,omitempty"`
	PackageErrors         []string             `json:"package_errors,omitempty"`
	CapsuleInstalled      []string             `json:"capsule_installed,omitempty"`
	CapsuleFallback       []string             `json:"capsule_fallback,omitempty"`
	CapsuleFallbackReason string               `json:"capsule_fallback_reason,omitempty"`
	CommittedPaths        []string             `json:"committed_paths,omitempty"`
	PendingSource         string               `json:"pending_source,omitempty"`
	PendingTarget         string               `json:"pending_target,omitempty"`
	PendingBackup         string               `json:"pending_backup,omitempty"`
	BrowserRepair         browserrepair.Result `json:"browser_repair,omitempty"`
	BrowserIssues         []string             `json:"browser_issues,omitempty"`
	BrowserJournal        string               `json:"browser_journal,omitempty"`
	Engine                string               `json:"engine,omitempty"`
	RequestedEngine       string               `json:"requested_engine,omitempty"`
	FallbackEngine        string               `json:"fallback_engine,omitempty"`
	FallbackPhase         string               `json:"fallback_phase,omitempty"`
	FallbackReason        string               `json:"fallback_reason,omitempty"`
	Qualification         Qualification        `json:"qualification,omitempty"`
	Timing                RecoveryTiming       `json:"timing,omitempty"`
	Recompression         string               `json:"recompression,omitempty"`
}

type Report struct {
	SnapshotID            string               `json:"snapshot_id"`
	Hostname              string               `json:"hostname"`
	RestoredPaths         []string             `json:"restored_paths,omitempty"`
	PackageErrors         []string             `json:"package_errors,omitempty"`
	CapsuleInstalled      []string             `json:"capsule_installed,omitempty"`
	CapsuleFallback       []string             `json:"capsule_fallback,omitempty"`
	CapsuleFallbackReason string               `json:"capsule_fallback_reason,omitempty"`
	ManualReview          []string             `json:"manual_review,omitempty"`
	HeavyDeferred         bool                 `json:"heavy_deferred"`
	JournalPath           string               `json:"journal_path"`
	Complete              bool                 `json:"complete"`
	BrowserRepair         browserrepair.Result `json:"browser_repair,omitempty"`
	BrowserIssues         []string             `json:"browser_issues,omitempty"`
	Engine                string               `json:"engine,omitempty"`
	FallbackReason        string               `json:"fallback_reason,omitempty"`
	Qualification         Qualification        `json:"qualification,omitempty"`
	Timing                RecoveryTiming       `json:"timing,omitempty"`
	Recompression         string               `json:"recompression,omitempty"`
}
