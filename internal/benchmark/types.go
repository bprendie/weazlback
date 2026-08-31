package benchmark

import "time"

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Engine        string    `json:"engine"`
	EngineVersion string    `json:"engine_version"`
	StartedAt     time.Time `json:"started_at"`
	WorkDir       string    `json:"work_dir"`
	Results       []Result  `json:"results"`
}

type Result struct {
	Fixture            string        `json:"fixture"`
	LogicalBytes       int64         `json:"logical_bytes"`
	AllocatedBytes     int64         `json:"allocated_bytes"`
	InitialDuration    time.Duration `json:"initial_duration_ns"`
	NoChangeDuration   time.Duration `json:"no_change_duration_ns"`
	ChangedDuration    time.Duration `json:"changed_duration_ns"`
	RestoreDuration    time.Duration `json:"restore_duration_ns"`
	InitialRepoBytes   int64         `json:"initial_repo_bytes"`
	NoChangeRepoGrowth int64         `json:"no_change_repo_growth_bytes"`
	ChangedRepoGrowth  int64         `json:"changed_repo_growth_bytes"`
	RestoredBytes      int64         `json:"restored_logical_bytes"`
	RestoredAllocated  int64         `json:"restored_allocated_bytes"`
	RestoreVerified    bool          `json:"restore_verified"`
}

type Options struct {
	Engine     string
	Fixture    string
	WorkDir    string
	Output     string
	Repository string
	SSHKey     string
	KnownHosts string
}
