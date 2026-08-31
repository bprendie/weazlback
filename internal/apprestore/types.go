package apprestore

import (
	"time"

	"github.com/bprendie/weazlback/internal/inventory"
)

type Source string

const (
	Official Source = "official"
	AUR      Source = "aur"
	Flatpak  Source = "flatpak"
)

type Package struct {
	Name, WantedVersion, CurrentVersion, AvailableVersion string
	Source                                                Source
}

type Plan struct {
	MachineID, Snapshot string
	Timestamp           time.Time
	Manifest            inventory.ApplicationManifest
	Install             []Package
	Substitutions       []Package
	Unavailable         []Package
	Conflicts           []Package
	Unchanged           []Package
	InstalledLater      []Package
	SystemServices      []string
	UserServices        []string
}

type Resolver interface {
	Available(name string, source Source) (version string, ok bool)
}

type Progress struct {
	Lane, Current     string
	Completed, Failed int
	Total             int
}

type Result struct {
	Installed, Substituted []string
	Unavailable, Conflicts []string
	Failures               []string
	RemovalCommands        []string
	MissingServices        []string
}
