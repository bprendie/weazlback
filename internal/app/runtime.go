package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	statusstore "github.com/bprendie/weazlback/internal/status"
	"github.com/bprendie/weazlback/internal/vault"
	"golang.org/x/term"
)

func verifyRepositoryIdentity(ctx context.Context, cfg *config.Config, destination *config.Destination, repo restic.Repository, persist bool) error {
	id, err := restic.NewService(nil).RepositoryID(ctx, repo)
	if err != nil {
		return fmt.Errorf("verify repository identity: %w", err)
	}
	if destination.RepositoryID != "" && destination.RepositoryID != id {
		return fmt.Errorf("repository identity mismatch: expected %s, got %s", destination.RepositoryID, id)
	}
	if destination.RepositoryID == "" {
		destination.RepositoryID = id
		if persist {
			path, pathErr := config.Path()
			if pathErr != nil {
				return pathErr
			}
			if saveErr := config.Save(path, *cfg); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

func openVault(name string, stderr io.Writer, create bool) (*vault.File, error) {
	path, err := vault.Path(name)
	if err != nil {
		return nil, err
	}
	v := vault.New(path)
	exists, err := v.Exists()
	if err != nil {
		return nil, err
	}
	passphrase, err := readPassphrase(stderr, !exists && create)
	if err != nil {
		return nil, err
	}
	defer zero(passphrase)
	if !exists {
		if !create {
			return nil, errors.New("vault does not exist; run weazlback init")
		}
		if err := v.Create(passphrase); err != nil {
			return nil, err
		}
		return v, nil
	}
	if err := v.Unlock(passphrase); err != nil {
		return nil, err
	}
	return v, nil
}

func readPassphrase(stderr io.Writer, confirm bool) ([]byte, error) {
	if value, ok := os.LookupEnv("WEAZLBACK_TEST_PASSPHRASE"); ok {
		if value == "" {
			return nil, errors.New("passphrase must not be empty")
		}
		return []byte(value), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("vault passphrase requires an interactive terminal")
	}
	fmt.Fprint(stderr, "Vault passphrase: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil || len(first) == 0 {
		return nil, errors.New("passphrase must not be empty")
	}
	if confirm {
		fmt.Fprint(stderr, "Confirm passphrase: ")
		second, readErr := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(stderr)
		defer zero(second)
		if readErr != nil || string(first) != string(second) {
			zero(first)
			return nil, errors.New("passphrases do not match")
		}
		fmt.Fprintln(stderr, "No recovery. Lose this passphrase and the backups are gone.")
	}
	return first, nil
}

func loadRuntime(destinationID string, stderr io.Writer) (config.Config, *config.Destination, *vault.File, error) {
	path, err := config.Path()
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, nil, nil, err
	}
	var destination *config.Destination
	if destinationID == "" {
		destination = cfg.Active()
	} else {
		destination = findDestination(cfg, destinationID)
	}
	if destination == nil {
		return cfg, nil, nil, errors.New("destination not found; run weazlback init")
	}
	v, err := openVault(cfg.ActiveVault, stderr, false)
	return cfg, destination, v, err
}

func loadRuntimeWithPassphrase(destinationID string, passphrase []byte) (config.Config, *config.Destination, *vault.File, error) {
	path, err := config.Path()
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, nil, nil, err
	}
	destination := cfg.Active()
	if destinationID != "" {
		destination = findDestination(cfg, destinationID)
	}
	if destination == nil {
		return cfg, nil, nil, errors.New("destination not found; open Weazlback to configure one")
	}
	vaultPath, err := vault.Path(cfg.ActiveVault)
	if err != nil {
		return cfg, nil, nil, err
	}
	v := vault.New(vaultPath)
	if err := v.Unlock(passphrase); err != nil {
		return cfg, nil, nil, err
	}
	return cfg, destination, v, nil
}

func repositoryFrom(v *vault.File, destination config.Destination) (restic.Repository, error) {
	password, err := v.Get(destination.PasswordKey)
	if err != nil {
		return restic.Repository{}, err
	}
	repo := restic.Repository{Location: destination.Repository, Password: password, KnownHosts: destination.SSHKnownHosts,
		Elevated: destination.Privileged, Connections: destination.Connections, UploadLimitKiB: destination.UploadLimitKiB}
	if destination.SSHKeyKey != "" {
		repo.SSHKey, err = v.Get(destination.SSHKeyKey)
	}
	return repo, err
}

func findDestination(cfg config.Config, id string) *config.Destination {
	for i := range cfg.Destinations {
		if cfg.Destinations[i].ID == id {
			return &cfg.Destinations[i]
		}
	}
	return nil
}

func findProfile(cfg config.Config, name string) *config.Profile {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			return &cfg.Profiles[i]
		}
	}
	return nil
}

func destinationFlag(name string, args []string, stderr io.Writer) (string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	destination := flags.String("destination", "", "destination ID")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	return *destination, nil
}

func statusCommand(args []string, stdout io.Writer) error {
	jsonMode := len(args) == 1 && args[0] == "--json"
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	value, err := store.Load()
	if err != nil {
		return err
	}
	if value.State == "" {
		value.State = "not-configured"
	}
	if value.State == "backing-up" {
		deadOwner := false
		if value.OperationPID > 1 {
			deadOwner = errors.Is(syscall.Kill(value.OperationPID, 0), syscall.ESRCH)
		} else {
			deadOwner = !value.UpdatedAt.IsZero() && time.Since(value.UpdatedAt) > 5*time.Minute
		}
		if deadOwner {
			value.State, value.Error = "failed", "backup was interrupted; completed profile Restore Points remain healthy"
			value.Cancellable, value.OperationPID, value.VaultState = false, 0, "locked"
			_ = store.Save(value)
		}
	}
	if value.SchemaVersion == 0 {
		value.SchemaVersion = 2
	}
	if jsonMode {
		return writeJSON(stdout, value)
	}
	_, err = fmt.Fprintf(stdout, "%s\n", value.State)
	return err
}

func defaultStatusStore() (statusstore.Store, error) {
	path, err := statusstore.DefaultPath()
	return statusstore.Store{Path: path}, err
}

func randomSecret(size int) ([]byte, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(b)))
	base64.RawURLEncoding.Encode(encoded, b)
	zero(b)
	return encoded, nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && b.String()[b.Len()-1] != '-' {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func ptrTime(value time.Time) *time.Time { return &value }
func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func authorizeDestination(destination config.Destination, stdout, stderr io.Writer) error {
	if !destination.Privileged {
		return nil
	}
	command := exec.Command("sudo", "-v")
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, stdout, stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sudo authorization failed: %w", err)
	}
	return nil
}
