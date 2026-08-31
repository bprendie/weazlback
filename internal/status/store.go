package status

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/contracts"
)

type Store struct{ Path string }

func DefaultPath() (string, error) {
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "status.json"), nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "weazlback", "status.json"), nil
}

func (s Store) Load() (contracts.Status, error) {
	var value contracts.Status
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(b, &value)
	return value, err
}

func (s Store) Save(value contracts.Status) error {
	value.SchemaVersion = 2
	value.UpdatedAt = time.Now().UTC()
	// Widget-visible state is intentionally filename-free. Detailed manifests
	// and diagnostics belong only in vault-encrypted operation logs.
	value.Manifest = ""
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".status-*")
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
	return os.Rename(name, s.Path)
}
