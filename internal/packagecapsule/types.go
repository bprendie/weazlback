package packagecapsule

import (
	"context"
	"time"

	"github.com/bprendie/weazlback/internal/platform"
)

const SchemaVersion = 1

type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	CapturedAt    time.Time         `json:"captured_at"`
	Hostname      string            `json:"hostname"`
	Architecture  string            `json:"architecture"`
	MachineID     string            `json:"machine_id"`
	Platform      platform.Identity `json:"platform,omitempty"`
	PackageFamily string            `json:"package_family,omitempty"`
	Packages      []Package         `json:"packages"`
	Flatpaks      []string          `json:"flatpaks,omitempty"`
	SystemUnits   []string          `json:"system_units,omitempty"`
	UserUnits     []string          `json:"user_units,omitempty"`
	ManualReview  []string          `json:"manual_review,omitempty"`
	Exceptions    []Exception       `json:"exceptions,omitempty"`
	Summary       Summary           `json:"summary"`
}

type Package struct {
	Name            string   `json:"name"`
	Installed       string   `json:"installed_version"`
	ArtifactVersion string   `json:"artifact_version,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
	Source          string   `json:"source"`
	Reason          string   `json:"install_reason"`
	Artifact        string   `json:"artifact,omitempty"`
	SHA256          string   `json:"sha256,omitempty"`
	Signature       string   `json:"signature,omitempty"`
	SignatureSHA256 string   `json:"signature_sha256,omitempty"`
	SignatureValid  bool     `json:"signature_valid,omitempty"`
	Depends         []string `json:"depends,omitempty"`
	Provides        []string `json:"provides,omitempty"`
	Conflicts       []string `json:"conflicts,omitempty"`
	BuildDate       string   `json:"build_date,omitempty"`
	Packager        string   `json:"packager,omitempty"`
	BuildInfo       string   `json:"buildinfo,omitempty"`
	PackageInfo     string   `json:"pkginfo,omitempty"`
	Compatible      bool     `json:"compatible"`
}

type Exception struct {
	Package string `json:"package,omitempty"`
	Code    string `json:"code"`
	Detail  string `json:"detail"`
}

type Summary struct {
	Installed int   `json:"installed"`
	Captured  int   `json:"captured"`
	Official  int   `json:"official"`
	Foreign   int   `json:"foreign"`
	Bytes     int64 `json:"artifact_bytes"`
	Missing   int   `json:"missing"`
	Rejected  int   `json:"rejected"`
}

type Progress struct {
	Phase, Package string
	Completed      int
	Total          int
	Bytes          int64
}

type Options struct {
	Context         context.Context
	MachineID       string
	StagingRoot     string
	Download        bool
	BuildMissingAUR bool
	Run             Runner
	Progress        func(Progress)
}

type Runner interface {
	Output(name string, args ...string) (string, error)
	Run(name string, args ...string) error
	RunDir(dir, name string, args ...string) error
}
