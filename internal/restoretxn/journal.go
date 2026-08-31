package restoretxn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const journalVersion = 1

type PathState struct {
	Source, Target, Rollback, State string
}

type Journal struct {
	Version     int
	OperationID string
	Snapshot    string
	Stage       string
	UpdatedAt   time.Time
	Paths       map[string]PathState
}

type BundleJournal struct {
	Version, Completed       int
	OperationID, Mode, State string
	Components               []Component
	Deletions                []string
	UpdatedAt                time.Time
}

func SaveBundleJournal(path string, cryptor Cryptor, journal BundleJournal) error {
	journal.Version, journal.UpdatedAt = journalVersion, time.Now().UTC()
	plain, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	encoded, err := cryptor.Encrypt(plain)
	if err != nil {
		return err
	}
	return writeJournalFile(path, encoded)
}

func newJournal(plan Plan) Journal {
	return Journal{Version: journalVersion, OperationID: plan.ID, Snapshot: plan.Snapshot, Stage: "planned", Paths: map[string]PathState{}}
}

func loadJournal(path string, cryptor Cryptor, plan Plan) (Journal, error) {
	encoded, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newJournal(plan), nil
	}
	if err != nil {
		return Journal{}, err
	}
	plain, err := cryptor.Decrypt(encoded)
	if err != nil {
		return Journal{}, err
	}
	var journal Journal
	if json.Unmarshal(plain, &journal) != nil || journal.Version != journalVersion || journal.OperationID != plan.ID || journal.Snapshot != plan.Snapshot {
		return Journal{}, errors.New("restore transaction journal does not match this operation")
	}
	if journal.Paths == nil {
		journal.Paths = map[string]PathState{}
	}
	return journal, nil
}

func saveJournal(path string, cryptor Cryptor, journal Journal) error {
	journal.Version, journal.UpdatedAt = journalVersion, time.Now().UTC()
	plain, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	encoded, err := cryptor.Encrypt(plain)
	if err != nil {
		return err
	}
	return writeJournalFile(path, encoded)
}

func writeJournalFile(path string, encoded []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".restore-journal-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(encoded)
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
