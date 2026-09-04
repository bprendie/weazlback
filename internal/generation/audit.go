package generation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/vault"
)

type Audit struct {
	SchemaVersion int       `json:"schema_version"`
	GenerationID  string    `json:"generation_id"`
	RepositoryID  string    `json:"repository_id"`
	Action        string    `json:"action"`
	Level         string    `json:"level,omitempty"`
	Result        string    `json:"result"`
	At            time.Time `json:"at"`
	Details       []string  `json:"details,omitempty"`
}

func SaveAudit(v *vault.File, audit Audit) (string, error) {
	if v == nil || !v.Unlocked() {
		return "", errors.New("vault must be unlocked to save generation audit")
	}
	audit.SchemaVersion = 1
	if audit.At.IsZero() {
		audit.At = time.Now()
	}
	plain, err := json.Marshal(audit)
	if err != nil {
		return "", err
	}
	ciphertext, err := v.Encrypt(plain)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".weazlback", "generations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := audit.GenerationID + "-" + audit.Action + "-" + audit.At.UTC().Format("20060102T150405Z") + ".wzaudit"
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, ciphertext, 0o600)
}
