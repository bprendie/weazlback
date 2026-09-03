package platform

const IdentitySchemaVersion = 1

type Identity struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Family        string `json:"family"`
	Variant       string `json:"variant,omitempty"`
	Version       string `json:"version,omitempty"`
	Architecture  string `json:"architecture"`
	PackageFamily string `json:"package_family,omitempty"`
	Desktop       string `json:"desktop,omitempty"`
	Session       string `json:"session,omitempty"`
	Security      string `json:"security,omitempty"`
}

func (i Identity) Known() bool { return i.Family != "" && i.Family != "unknown" }

type Claim struct {
	Path     string `json:"path,omitempty"`
	Resource string `json:"resource,omitempty"`
	Owner    string `json:"owner"`
	Domain   string `json:"domain"`
}

type Facts struct {
	OSRelease    map[string]string
	Architecture string
	Desktop      string
	Session      string
	Omarchy      bool
	SELinux      bool
	AppArmor     bool
}
