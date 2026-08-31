package inventory

import "time"

type Report struct {
	SchemaVersion int              `json:"schema_version"`
	CapturedAt    time.Time        `json:"captured_at"`
	Hostname      string           `json:"hostname"`
	Architecture  string           `json:"architecture"`
	Omarchy       string           `json:"omarchy_version,omitempty"`
	Home          string           `json:"home"`
	HomeEntries   []PathEntry      `json:"home_entries"`
	ConfigEntries []PathEntry      `json:"config_entries"`
	LargeFiles    []LargeFile      `json:"large_files"`
	Packages      PackageInventory `json:"packages"`
	Services      ServiceInventory `json:"services"`
	PkgInstall    []string         `json:"pkginstall_files,omitempty"`
}

type PathEntry struct {
	Path           string `json:"path"`
	Bytes          int64  `json:"bytes"`
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
}

type LargeFile struct {
	Path        string `json:"path"`
	Format      string `json:"format"`
	Logical     int64  `json:"logical_bytes"`
	Allocated   int64  `json:"allocated_bytes"`
	SparseRatio string `json:"sparse_ratio"`
}

type PackageInventory struct {
	OfficialExplicit  []string           `json:"official_explicit"`
	ForeignExplicit   []string           `json:"foreign_explicit"`
	OfficialInstalled []InstalledPackage `json:"official_installed,omitempty"`
	ForeignInstalled  []InstalledPackage `json:"foreign_installed,omitempty"`
	FlatpakApps       []string           `json:"flatpak_apps,omitempty"`
}

type InstalledPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServiceInventory struct {
	SystemEnabled []string `json:"system_enabled"`
	UserEnabled   []string `json:"user_enabled"`
}
