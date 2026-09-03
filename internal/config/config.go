package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = 1

type Config struct {
	SchemaVersion     int           `json:"schema_version"`
	ActiveVault       string        `json:"active_vault"`
	Machine           Machine       `json:"machine"`
	ActiveDestination string        `json:"active_destination,omitempty"`
	Destinations      []Destination `json:"destinations"`
	Profiles          []Profile     `json:"profiles"`
	Retention         Retention     `json:"retention"`
	HeavyRetention    Retention     `json:"heavy_retention"`
	PackagePolicy     PackagePolicy `json:"package_policy"`
}

type Machine struct {
	Version   int      `json:"version"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Hostname  string   `json:"hostname"`
	Hostnames []string `json:"hostname_history,omitempty"`
}

const MachineSchemaVersion = 1

func (c *Config) Active() *Destination {
	for i := range c.Destinations {
		if c.Destinations[i].ID == c.ActiveDestination {
			return &c.Destinations[i]
		}
	}
	if len(c.Destinations) > 0 {
		return &c.Destinations[0]
	}
	return nil
}

type Destination struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Repository     string `json:"repository"`
	RepositoryID   string `json:"repository_id,omitempty"`
	PasswordKey    string `json:"password_key"`
	SSHKeyKey      string `json:"ssh_key_key,omitempty"`
	SSHKnownHosts  string `json:"ssh_known_hosts,omitempty"`
	Privileged     bool   `json:"privileged,omitempty"`
	Connections    int    `json:"connections,omitempty"`
	UploadLimitKiB int    `json:"upload_limit_kib,omitempty"`
}

type Profile struct {
	Name     string   `json:"name"`
	Includes []string `json:"includes"`
	Excludes []string `json:"excludes"`
}

type Retention struct {
	Hourly  int `json:"hourly"`
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

type PackagePolicy struct {
	Scheduled        bool       `json:"scheduled"`
	IntervalDays     int        `json:"interval_days"`
	DownloadOfficial bool       `json:"download_official"`
	LastCaptured     *time.Time `json:"last_captured,omitempty"`
	LastReminder     *time.Time `json:"last_reminder,omitempty"`
}

func (p PackagePolicy) Due(now time.Time) bool {
	if !p.Scheduled {
		return false
	}
	if p.LastCaptured == nil {
		return true
	}
	return now.Sub(*p.LastCaptured) >= time.Duration(p.IntervalDays)*24*time.Hour
}

func Default() Config {
	home, _ := os.UserHomeDir()
	hostname, _ := os.Hostname()
	core := existingPaths([]string{filepath.Join(home, ".config"), filepath.Join(home, ".local", "share"),
		filepath.Join(home, ".local", "bin"), filepath.Join(home, ".ssh"), filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".bashrc"), filepath.Join(home, ".bash_profile"), filepath.Join(home, ".profile"),
		filepath.Join(home, ".zshrc"), filepath.Join(home, ".gitconfig")})
	core = appendUnique(core, weazlRoots(home)...)
	return Config{
		SchemaVersion:  SchemaVersion,
		ActiveVault:    "default",
		Machine:        newMachine(hostname),
		Retention:      Retention{Hourly: 24, Daily: 14, Weekly: 8, Monthly: 12},
		HeavyRetention: Retention{Daily: 7, Weekly: 4, Monthly: 3},
		PackagePolicy:  PackagePolicy{IntervalDays: 30, DownloadOfficial: true},
		Profiles: []Profile{
			{Name: "core", Includes: core,
				Excludes: []string{"**/Cache/**", "**/cache/**", "**/.cache/**"}},
			{Name: "home", Includes: []string{home}, Excludes: []string{filepath.Join(home, "containers", "**")}},
			{Name: "heavy", Includes: []string{filepath.Join(home, "containers")}},
		},
	}
}

func newMachine(hostname string) Machine {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("generate machine identity: %v", err))
	}
	name := strings.TrimSpace(hostname)
	if name == "" {
		name = "machine"
	}
	return Machine{Version: MachineSchemaVersion, ID: fmt.Sprintf("%x", raw), Name: name, Hostname: hostname, Hostnames: nonEmpty(hostname)}
}

// NewMachine creates a stable identity for a genuinely new installation.
func NewMachine(hostname string) Machine { return newMachine(hostname) }

func nonEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func normalizeMachine(machine *Machine, hostname string) bool {
	changed := false
	if machine.Version != MachineSchemaVersion {
		machine.Version, changed = MachineSchemaVersion, true
	}
	if machine.Name == "" {
		machine.Name, changed = hostname, true
	}
	if hostname != "" && machine.Hostname != hostname {
		machine.Hostname, changed = hostname, true
	}
	for _, known := range machine.Hostnames {
		if known == hostname || hostname == "" {
			return changed
		}
	}
	if hostname != "" {
		machine.Hostnames, changed = append(machine.Hostnames, hostname), true
	}
	return changed
}

func ValidMachineID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func weazlRoots(home string) []string {
	matches, _ := filepath.Glob(filepath.Join(home, ".*weazl*"))
	return existingPaths(matches)
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func existingPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			result = append(result, path)
		}
	}
	return result
}

func Path() (string, error) {
	if path := os.Getenv("WEAZLBACK_CONFIG"); path != "" {
		return path, nil
	}
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "config.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "weazlback", "config.json"), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.SchemaVersion != SchemaVersion {
		return cfg, errors.New("unsupported configuration schema")
	}
	changed := false
	if !ValidMachineID(cfg.Machine.ID) {
		hostname, _ := os.Hostname()
		cfg.Machine = newMachine(hostname)
		changed = true
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		changed = normalizeMachine(&cfg.Machine, hostname) || changed
	}
	if cfg.HeavyRetention == (Retention{}) {
		cfg.HeavyRetention = Retention{Daily: 7, Weekly: 4, Monthly: 3}
	}
	if cfg.PackagePolicy.IntervalDays <= 0 {
		cfg.PackagePolicy.IntervalDays = 30
		cfg.PackagePolicy.DownloadOfficial = true
		changed = true
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == "core" {
			home, _ := os.UserHomeDir()
			cfg.Profiles[i].Includes = appendUnique(cfg.Profiles[i].Includes, weazlRoots(home)...)
		}
		if cfg.Profiles[i].Name == "home" {
			changed = migrateHomeProfile(&cfg.Profiles[i]) || changed
		}
	}
	if changed {
		if err := Save(path, cfg); err != nil {
			return cfg, fmt.Errorf("persist migrated machine identity: %w", err)
		}
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
