package contracts

import "time"

type Destination struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Repository    string `json:"repository"`
	SSHHost       string `json:"ssh_host,omitempty"`
	SSHUser       string `json:"ssh_user,omitempty"`
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
	Engine        string `json:"engine"`
}

type Snapshot struct {
	ID        string    `json:"id"`
	MachineID string    `json:"machine_id"`
	Profile   string    `json:"profile"`
	CreatedAt time.Time `json:"created_at"`
	Healthy   bool      `json:"healthy"`
}

type Progress struct {
	Profile            string        `json:"profile,omitempty"`
	Phase              string        `json:"phase"`
	Path               string        `json:"path,omitempty"`
	Files              uint64        `json:"files"`
	TotalFiles         uint64        `json:"total_files"`
	LogicalBytes       uint64        `json:"logical_bytes"`
	UploadedBytes      uint64        `json:"uploaded_bytes"`
	Percent            float64       `json:"percent"`
	Elapsed            time.Duration `json:"elapsed"`
	ETA                time.Duration `json:"eta"`
	WireBytesPerSecond float64       `json:"wire_bytes_per_second,omitempty"`
}

type ProfileProgress struct {
	Profile        string  `json:"profile"`
	State          string  `json:"state"`
	Percent        float64 `json:"percent"`
	Files          uint64  `json:"files"`
	Total          uint64  `json:"total_files"`
	Bytes          uint64  `json:"bytes"`
	TotalBytes     uint64  `json:"total_bytes"`
	BytesPerSecond float64 `json:"bytes_per_second,omitempty"`
	FilesPerSecond float64 `json:"files_per_second,omitempty"`
}

type Status struct {
	SchemaVersion    int               `json:"schema_version"`
	UpdatedAt        time.Time         `json:"updated_at"`
	OperationID      string            `json:"operation_id,omitempty"`
	OperationPID     int               `json:"operation_pid,omitempty"`
	State            string            `json:"state"`
	Destination      string            `json:"destination,omitempty"`
	LastHealthy      *time.Time        `json:"last_healthy,omitempty"`
	LastRoutine      *time.Time        `json:"last_routine,omitempty"`
	Progress         *Progress         `json:"progress,omitempty"`
	Error            string            `json:"error,omitempty"`
	Incomplete       bool              `json:"incomplete,omitempty"`
	Skipped          uint64            `json:"skipped,omitempty"`
	Manifest         string            `json:"skipped_manifest,omitempty"`
	Profiles         []ProfileProgress `json:"profiles,omitempty"`
	VaultState       string            `json:"vault_state,omitempty"`
	RepositoryHealth string            `json:"repository_health,omitempty"`
	Cancellable      bool              `json:"cancellable,omitempty"`
	TravelUntil      *time.Time        `json:"travel_until,omitempty"`
	SuccessUntil     *time.Time        `json:"success_until,omitempty"`
	LastReminder     *time.Time        `json:"last_reminder,omitempty"`
}

type MachineManifest struct {
	SchemaVersion int      `json:"schema_version"`
	MachineID     string   `json:"machine_id"`
	Hostname      string   `json:"hostname"`
	Architecture  string   `json:"architecture"`
	Omarchy       string   `json:"omarchy_version"`
	Profiles      []string `json:"profiles"`
}
