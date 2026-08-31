package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

const SchemaVersion = 1

type Catalog struct {
	SchemaVersion int                    `json:"schema_version"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Chains        map[string]Chain       `json:"chains"`
	Paths         map[string]*PathRecord `json:"paths"`
}

type Chain struct {
	MachineID string `json:"machine_id"`
	Profile   string `json:"profile"`
	Latest    string `json:"latest"`
	Health    string `json:"health"`
}

type PathRecord struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Versions []Version `json:"versions"`
}

type Version struct {
	SnapshotID string    `json:"snapshot_id"`
	MachineID  string    `json:"machine_id"`
	Profile    string    `json:"profile"`
	Time       time.Time `json:"time"`
	Change     string    `json:"change"`
	Health     string    `json:"health"`
	Type       string    `json:"type,omitempty"`
	Size       uint64    `json:"size,omitempty"`
	Mode       uint32    `json:"mode,omitempty"`
	UID        uint32    `json:"uid,omitempty"`
	GID        uint32    `json:"gid,omitempty"`
}

func New() Catalog {
	return Catalog{SchemaVersion: SchemaVersion, Chains: map[string]Chain{}, Paths: map[string]*PathRecord{}}
}

func (c *Catalog) normalize() {
	if c.Chains == nil {
		c.Chains = map[string]Chain{}
	}
	if c.Paths == nil {
		c.Paths = map[string]*PathRecord{}
	}
}

func ChainKey(machineID, profile string) string { return machineID + "/" + profile }

func (c *Catalog) Baseline(snapshot restic.Snapshot, files []restic.FileEntry) {
	c.normalize()
	machineID, profile := restic.MachineID(snapshot.Tags), restic.Profile(snapshot.Tags)
	for _, file := range files {
		c.add(file.Path, Version{SnapshotID: snapshot.ID, MachineID: machineID, Profile: profile,
			Time: snapshot.Time, Change: "+", Health: restic.SnapshotHealth(snapshot.Tags), Type: file.Type, Size: file.Size, Mode: file.Mode, UID: file.UID, GID: file.GID})
	}
	c.Chains[ChainKey(machineID, profile)] = Chain{MachineID: machineID, Profile: profile, Latest: snapshot.ID, Health: restic.SnapshotHealth(snapshot.Tags)}
}

func (c *Catalog) Apply(snapshot restic.Snapshot, changes []restic.DiffChange) {
	c.normalize()
	machineID, profile := restic.MachineID(snapshot.Tags), restic.Profile(snapshot.Tags)
	for _, change := range changes {
		c.add(change.Path, Version{SnapshotID: snapshot.ID, MachineID: machineID, Profile: profile,
			Time: snapshot.Time, Change: change.Modifier, Health: restic.SnapshotHealth(snapshot.Tags)})
	}
	c.Chains[ChainKey(machineID, profile)] = Chain{MachineID: machineID, Profile: profile, Latest: snapshot.ID, Health: restic.SnapshotHealth(snapshot.Tags)}
}

func (c *Catalog) add(path string, version Version) {
	path = filepath.Clean(path)
	record := c.Paths[path]
	if record == nil {
		record = &PathRecord{Path: path, Name: filepath.Base(path)}
		c.Paths[path] = record
	}
	for _, existing := range record.Versions {
		if existing.SnapshotID == version.SnapshotID && existing.MachineID == version.MachineID && existing.Profile == version.Profile {
			return
		}
	}
	record.Versions = append(record.Versions, version)
	sort.Slice(record.Versions, func(i, j int) bool { return record.Versions[i].Time.After(record.Versions[j].Time) })
}

func (c *Catalog) Enrich(snapshotID string, files []restic.FileEntry) {
	for _, file := range files {
		record := c.Paths[filepath.Clean(file.Path)]
		if record == nil {
			continue
		}
		for i := range record.Versions {
			if record.Versions[i].SnapshotID == snapshotID {
				record.Versions[i].Type, record.Versions[i].Size = file.Type, file.Size
				record.Versions[i].Mode, record.Versions[i].UID, record.Versions[i].GID = file.Mode, file.UID, file.GID
			}
		}
	}
}

func (c Catalog) Search(query, machineID string, limit int) []PathRecord {
	query = strings.ToLower(strings.TrimSpace(query))
	var result []PathRecord
	for _, record := range c.Paths {
		if query != "" && !fuzzy(strings.ToLower(record.Path), query) {
			continue
		}
		copyRecord := PathRecord{Path: record.Path, Name: record.Name}
		for _, version := range record.Versions {
			if machineID == "" || version.MachineID == machineID {
				copyRecord.Versions = append(copyRecord.Versions, version)
			}
		}
		if len(copyRecord.Versions) > 0 {
			result = append(result, copyRecord)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := score(result[i].Path, query), score(result[j].Path, query)
		if left == right {
			return result[i].Path < result[j].Path
		}
		return left < right
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func fuzzy(value, query string) bool {
	if strings.Contains(value, query) {
		return true
	}
	index := 0
	for _, r := range value {
		if index < len(query) && byte(r) == query[index] {
			index++
		}
	}
	return index == len(query)
}

func score(value, query string) int {
	if strings.Contains(strings.ToLower(filepath.Base(value)), query) {
		return 0
	}
	if strings.Contains(strings.ToLower(value), query) {
		return 1
	}
	return 2
}

func Save(path string, c Catalog, v *vault.File) error {
	c.SchemaVersion = SchemaVersion
	c.UpdatedAt = time.Now().UTC()
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := v.Encrypt(plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".catalog-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(sealed); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func Load(path string, v *vault.File) (Catalog, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return Catalog{}, err
	}
	plain, err := v.Decrypt(b)
	if err != nil {
		return Catalog{}, err
	}
	var c Catalog
	if err := json.Unmarshal(plain, &c); err != nil || c.SchemaVersion != SchemaVersion {
		return Catalog{}, errors.New("invalid or unsupported history catalog")
	}
	c.normalize()
	return c, nil
}

func Path(destinationID string) (string, error) {
	root := os.Getenv("WEAZLBACK_HOME")
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(dir, "weazlback")
	}
	return filepath.Join(root, "catalogs", destinationID+".catalog"), nil
}
