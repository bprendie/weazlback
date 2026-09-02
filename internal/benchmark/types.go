package benchmark

import "time"

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Engine        string    `json:"engine"`
	EngineVersion string    `json:"engine_version"`
	StartedAt     time.Time `json:"started_at"`
	WorkDir       string    `json:"work_dir"`
	Results       []Result  `json:"results"`
	Baseline      Baseline  `json:"baseline"`
}

type Baseline struct {
	PayloadBytes      int64         `json:"payload_bytes"`
	Preflight         time.Duration `json:"preflight_ns"`
	TimeToUsable      time.Duration `json:"time_to_usable_ns"`
	TimeToDurable     time.Duration `json:"time_to_durable_ns"`
	PackageCompletion time.Duration `json:"package_completion_ns"`
	Total             time.Duration `json:"total_ns"`
	SourceBytesPerSec float64       `json:"source_bytes_per_second"`
	TargetBytesPerSec float64       `json:"target_bytes_per_second"`
	Files             int64         `json:"files"`
}

type Result struct {
	Fixture            string          `json:"fixture"`
	LogicalBytes       int64           `json:"logical_bytes"`
	AllocatedBytes     int64           `json:"allocated_bytes"`
	InitialDuration    time.Duration   `json:"initial_duration_ns"`
	NoChangeDuration   time.Duration   `json:"no_change_duration_ns"`
	ChangedDuration    time.Duration   `json:"changed_duration_ns"`
	RestoreDuration    time.Duration   `json:"restore_duration_ns"`
	InitialRepoBytes   int64           `json:"initial_repo_bytes"`
	NoChangeRepoGrowth int64           `json:"no_change_repo_growth_bytes"`
	ChangedRepoGrowth  int64           `json:"changed_repo_growth_bytes"`
	RestoredBytes      int64           `json:"restored_logical_bytes"`
	RestoredAllocated  int64           `json:"restored_allocated_bytes"`
	RestoreVerified    bool            `json:"restore_verified"`
	RestoreTrials      []time.Duration `json:"restore_trials_ns,omitempty"`
	ColdRestore        time.Duration   `json:"cold_restore_ns,omitempty"`
	WarmRestoreMedian  time.Duration   `json:"warm_restore_median_ns,omitempty"`
	PreflightDuration  time.Duration   `json:"preflight_duration_ns,omitempty"`
	TimeToUsable       time.Duration   `json:"time_to_usable_ns,omitempty"`
	TimeToDurable      time.Duration   `json:"time_to_durable_ns,omitempty"`
	FileCount          int64           `json:"file_count,omitempty"`
}

type Options struct {
	Engine      string
	Fixture     string
	WorkDir     string
	Output      string
	Repository  string
	SSHKey      string
	KnownHosts  string
	Trials      int
	Connections int
}
