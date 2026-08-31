package apprestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Cryptor interface{ Encrypt([]byte) ([]byte, error) }

func SaveResult(path string, cryptor Cryptor, plan Plan, result Result) error {
	payload, err := json.Marshal(struct {
		Version   int
		UpdatedAt time.Time
		Plan      Plan
		Result    Result
	}{Version: 1, UpdatedAt: time.Now().UTC(), Plan: plan, Result: result})
	if err != nil {
		return err
	}
	sealed, err := cryptor.Encrypt(payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".applications-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(sealed)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
