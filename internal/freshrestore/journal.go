package freshrestore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var stages = []string{"start", "preflight", "repository_verified", "snapshot_selected", "plan_confirmed", "hostname_applied", "core_staged", "packages_reconciled", "core_committed", "browser_compatibility", "user_state_reconciled", "heavy_committed", "system_validated", "complete"}

func LoadJournal(path string) (Journal, error) {
	var journal Journal
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return journal, nil
	}
	if err != nil {
		return journal, err
	}
	if err := json.Unmarshal(b, &journal); err != nil || (journal.SchemaVersion != 1 && journal.SchemaVersion != JournalSchemaVersion) {
		return Journal{}, errors.New("restore journal is invalid or unsupported")
	}
	if journal.SchemaVersion == 1 {
		journal.SchemaVersion = JournalSchemaVersion
		if journal.Engine == "" {
			journal.Engine = "standard"
		}
	}
	return journal, nil
}

func SaveJournal(path string, journal Journal) error {
	journal.SchemaVersion, journal.UpdatedAt = JournalSchemaVersion, time.Now().UTC()
	b, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".journal-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(append(b, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func stageAtLeast(current, expected string) bool {
	index := func(value string) int {
		for i, stage := range stages {
			if stage == value {
				return i
			}
		}
		return -1
	}
	return index(current) >= index(expected)
}
