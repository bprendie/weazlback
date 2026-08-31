package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/nuke"
	"github.com/bprendie/weazlback/internal/vault"
)

func nukeCommand(_ context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("internal-nuke", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "repository ID")
	mode := flags.String("mode", "full", "full or keys")
	removeDirectory := flags.Bool("remove-directory", false, "remove exact local repository directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	reader := bufio.NewReader(io.LimitReader(stdin, 128*1024))
	passLine, err := reader.ReadString('\n')
	if err != nil {
		return errors.New("nuke requires vault passphrase over stdin")
	}
	passphrase := []byte(strings.TrimSpace(passLine))
	defer zero(passphrase)
	confirmation, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(confirmation) != "NUKE "+*destinationID {
		return errors.New("break-glass confirmation did not match")
	}
	if err := stopActiveForNuke(); err != nil {
		return err
	}
	cfg, destination, v, err := loadRuntimeWithPassphrase(*destinationID, passphrase)
	if err != nil {
		return err
	}
	defer v.Lock()
	result, err := executeNuke(cfg, destination, v, *mode, *removeDirectory, passphrase)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func stopActiveForNuke() error {
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	status, err := store.Load()
	if err != nil || !status.Cancellable || status.OperationPID <= 1 {
		return err
	}
	if err := syscall.Kill(status.OperationPID, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(status.OperationPID, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("active operation did not terminate; repository was not changed")
}

type nukeResult struct {
	SchemaVersion   int       `json:"schema_version"`
	RepositoryID    string    `json:"repository_id"`
	Mode            string    `json:"mode"`
	CompletedAt     time.Time `json:"completed_at"`
	Deleted         bool      `json:"repository_deleted"`
	KeysDestroyed   bool      `json:"keys_destroyed"`
	KitsRemoved     int       `json:"matching_recovery_kits_removed"`
	KitsUnreachable int       `json:"known_recovery_kits_unreachable"`
}

func executeNuke(cfg config.Config, destination *config.Destination, v *vault.File, mode string, removeLocalDirectory bool, passphrase []byte) (nukeResult, error) {
	result := nukeResult{SchemaVersion: 1, RepositoryID: destination.ID, Mode: mode, CompletedAt: time.Now().UTC()}
	if mode != "full" && mode != "keys" {
		return result, errors.New("nuke mode must be full or keys")
	}
	if mode == "full" {
		var privateKey []byte
		var err error
		if destination.SSHKeyKey != "" {
			privateKey, err = v.Get(destination.SSHKeyKey)
			if err != nil {
				return result, err
			}
			defer zero(privateKey)
		}
		if err := nuke.DeleteRepository(*destination, privateKey, removeLocalDirectory); err != nil {
			_ = writeTombstone(result)
			return result, fmt.Errorf("repository deletion failed; keys preserved for retry: %w", err)
		}
		result.Deleted = true
	}
	keys := []string{destination.PasswordKey}
	if destination.SSHKeyKey != "" {
		keys = append(keys, destination.SSHKeyKey)
	}
	if err := v.Delete(keys...); err != nil {
		return result, err
	}
	result.KeysDestroyed = true
	result.KitsRemoved, result.KitsUnreachable = removeMatchingRecoveryKits(destination.ID, passphrase)
	cfg.Destinations = removeDestination(cfg.Destinations, destination.ID)
	if cfg.ActiveDestination == destination.ID {
		cfg.ActiveDestination = ""
		if len(cfg.Destinations) > 0 {
			cfg.ActiveDestination = cfg.Destinations[0].ID
		}
	}
	path, err := config.Path()
	if err != nil {
		return result, err
	}
	if err := config.Save(path, cfg); err != nil {
		return result, err
	}
	return result, writeTombstone(result)
}

func removeDestination(destinations []config.Destination, id string) []config.Destination {
	result := destinations[:0]
	for _, destination := range destinations {
		if destination.ID != id {
			result = append(result, destination)
		}
	}
	return result
}

func writeTombstone(result nukeResult) error {
	root, err := schedulePath("../tombstones")
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%d.json", result.RepositoryID, result.CompletedAt.Unix())
	return atomicAppWrite(filepath.Join(root, name), append(data, '\n'))
}

func atomicAppWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
