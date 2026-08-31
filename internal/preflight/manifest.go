package preflight

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Profile       string    `json:"profile"`
	Destination   string    `json:"destination"`
	Reason        string    `json:"reason"`
	Skipped       []string  `json:"skipped"`
}

func ManifestPath() (string, error) {
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "last-skipped.json"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "weazlback", "last-skipped.json"), nil
}

func WriteManifest(path string, manifest Manifest) error {
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".skipped-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
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
	return os.Rename(name, path)
}
