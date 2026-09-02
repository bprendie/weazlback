package freshrestore

import (
	"time"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

const JournalSchemaVersion = 1

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
}

type RestoreProgress struct {
	Phase, Lane, Current     string
	Completed, Failed, Total int
	BytesDone, BytesTotal    uint64
	BytesPerSecond           float64
	FilesPerSecond           float64
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
	SchemaVersion   int                  `json:"schema_version"`
	RepositoryID    string               `json:"repository_id"`
	SnapshotID      string               `json:"snapshot_id"`
	HomeSnapshotID  string               `json:"home_snapshot_id,omitempty"`
	HeavySnapshotID string               `json:"heavy_snapshot_id,omitempty"`
	Scope           string               `json:"scope,omitempty"`
	Stage           string               `json:"stage"`
	Hostname        string               `json:"hostname"`
	TargetHome      string               `json:"target_home"`
	UpdatedAt       time.Time            `json:"updated_at"`
	Connections     int                  `json:"connections,omitempty"`
	PackageErrors   []string             `json:"package_errors,omitempty"`
	CommittedPaths  []string             `json:"committed_paths,omitempty"`
	PendingSource   string               `json:"pending_source,omitempty"`
	PendingTarget   string               `json:"pending_target,omitempty"`
	PendingBackup   string               `json:"pending_backup,omitempty"`
	BrowserRepair   browserrepair.Result `json:"browser_repair,omitempty"`
	BrowserIssues   []string             `json:"browser_issues,omitempty"`
	BrowserJournal  string               `json:"browser_journal,omitempty"`
}

type Report struct {
	SnapshotID    string               `json:"snapshot_id"`
	Hostname      string               `json:"hostname"`
	RestoredPaths []string             `json:"restored_paths,omitempty"`
	PackageErrors []string             `json:"package_errors,omitempty"`
	ManualReview  []string             `json:"manual_review,omitempty"`
	HeavyDeferred bool                 `json:"heavy_deferred"`
	JournalPath   string               `json:"journal_path"`
	Complete      bool                 `json:"complete"`
	BrowserRepair browserrepair.Result `json:"browser_repair,omitempty"`
	BrowserIssues []string             `json:"browser_issues,omitempty"`
}
