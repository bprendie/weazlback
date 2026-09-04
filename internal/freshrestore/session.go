package freshrestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

func OpenSession(kit string, passphrase []byte) (*Session, error) {
	return OpenSessionDestinationAt(kit, passphrase, "", "")
}

func OpenSessionAt(kit string, passphrase []byte, repository string) (*Session, error) {
	return OpenSessionDestinationAt(kit, passphrase, "", repository)
}

type RecoveryCatalog struct {
	Active       string
	Destinations []config.Destination
	Summaries    map[string]DestinationRecoverySummary
}

type DestinationRecoverySummary struct {
	LatestCore     time.Time
	LatestHome     time.Time
	LatestComplete time.Time
	CompleteID     string
	Error          string
}

func ReadRecoveryIdentities(ctx context.Context, kit string, passphrase []byte, destinationID string) ([]restic.Identity, error) {
	session, err := OpenSessionDestinationAt(kit, passphrase, destinationID, "")
	if err != nil {
		return nil, err
	}
	defer session.Close()
	snapshots, err := restic.NewService(nil).Snapshots(ctx, session.Repository)
	if err != nil {
		return nil, err
	}
	return restic.GroupIdentities(snapshots, session.Config.Machine.ID, session.Config.Machine.Name), nil
}

func ReadRecoveryCatalog(kit string, passphrase []byte) (RecoveryCatalog, error) {
	bundle, err := recovery.Open(kit, passphrase)
	if err != nil {
		return RecoveryCatalog{}, err
	}
	defer bundle.Close()
	var cfg config.Config
	if err := json.Unmarshal(bundle.Config, &cfg); err != nil || cfg.SchemaVersion != config.SchemaVersion {
		return RecoveryCatalog{}, errors.New("recovery kit configuration is invalid or unsupported")
	}
	if len(cfg.Destinations) == 0 {
		return RecoveryCatalog{}, errors.New("recovery kit has no repository destination")
	}
	catalog := RecoveryCatalog{Active: cfg.ActiveDestination, Destinations: append([]config.Destination(nil), cfg.Destinations...), Summaries: map[string]DestinationRecoverySummary{}}
	for _, destination := range cfg.Destinations {
		session, openErr := OpenSessionDestinationAt(kit, passphrase, destination.ID, "")
		if openErr != nil {
			catalog.Summaries[destination.ID] = DestinationRecoverySummary{Error: openErr.Error()}
			continue
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		points, listErr := restic.NewService(io.Discard).Snapshots(probeCtx, session.Repository)
		cancel()
		session.Close()
		if listErr != nil {
			catalog.Summaries[destination.ID] = DestinationRecoverySummary{Error: listErr.Error()}
			continue
		}
		summary := summarizeDestination(points, cfg.Machine.ID)
		catalog.Summaries[destination.ID] = summary
	}
	return catalog, nil
}

func summarizeDestination(points []restic.Snapshot, machineID string) DestinationRecoverySummary {
	var result DestinationRecoverySummary
	for _, point := range restic.FilterIdentity(points, machineID) {
		if restic.SnapshotHealth(point.Tags) != "healthy" {
			continue
		}
		switch restic.Profile(point.Tags) {
		case "core":
			if point.Time.After(result.LatestCore) {
				result.LatestCore = point.Time
			}
		case "home":
			if point.Time.After(result.LatestHome) {
				result.LatestHome = point.Time
			}
		}
	}
	if complete, ok := generation.LatestComplete(generation.Catalog(points), machineID); ok {
		result.LatestComplete, result.CompleteID = complete.EndedAt, complete.ID
	}
	return result
}

func OpenSessionDestinationAt(kit string, passphrase []byte, destinationID, repository string) (*Session, error) {
	bundle, err := recovery.Open(kit, passphrase)
	if err != nil {
		return nil, err
	}
	defer bundle.Close()
	var cfg config.Config
	if err := json.Unmarshal(bundle.Config, &cfg); err != nil || cfg.SchemaVersion != config.SchemaVersion {
		return nil, errors.New("recovery kit configuration is invalid or unsupported")
	}
	if len(cfg.Destinations) == 0 {
		return nil, errors.New("recovery kit has no repository destination")
	}
	privateDir, err := os.MkdirTemp("", "weazlback-restore-*")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(privateDir, 0o700); err != nil {
		os.RemoveAll(privateDir)
		return nil, err
	}
	vaultPath := filepath.Join(privateDir, "vault.enc")
	if err := os.WriteFile(vaultPath, bundle.Vault, 0o600); err != nil {
		os.RemoveAll(privateDir)
		return nil, err
	}
	v := vault.New(vaultPath)
	if err := v.Unlock(passphrase); err != nil {
		os.RemoveAll(privateDir)
		return nil, err
	}
	destination := cfg.Active()
	if destinationID != "" {
		destination = nil
		for i := range cfg.Destinations {
			if cfg.Destinations[i].ID == destinationID {
				destination = &cfg.Destinations[i]
				break
			}
		}
	}
	if destination == nil {
		v.Lock()
		os.RemoveAll(privateDir)
		return nil, fmt.Errorf("recovery destination %q was not found in this kit", destinationID)
	}
	selected := *destination
	password, err := v.Get(selected.PasswordKey)
	if err != nil {
		v.Lock()
		os.RemoveAll(privateDir)
		return nil, err
	}
	repo := restic.Repository{Location: selected.Repository, Password: password, Connections: selected.Connections, UploadLimitKiB: selected.UploadLimitKiB}
	if repository != "" {
		if selected.Kind != "local" {
			zero(repo.Password)
			v.Lock()
			os.RemoveAll(privateDir)
			return nil, errors.New("repository path override is only valid for a local destination")
		}
		absolute, pathErr := filepath.Abs(repository)
		if pathErr != nil {
			return nil, pathErr
		}
		selected.Repository, repo.Location = absolute, absolute
	}
	if selected.SSHKeyKey != "" {
		repo.SSHKey, err = v.Get(selected.SSHKeyKey)
	}
	if err == nil && len(bundle.KnownHosts) > 0 {
		repo.KnownHosts = filepath.Join(privateDir, "known_hosts")
		err = os.WriteFile(repo.KnownHosts, bundle.KnownHosts, 0o600)
	}
	if err != nil {
		zero(repo.Password)
		zero(repo.SSHKey)
		v.Lock()
		os.RemoveAll(privateDir)
		return nil, err
	}
	return &Session{Config: cfg, Destination: selected, Repository: repo, Vault: v, PrivateDir: privateDir}, nil
}

func (s *Session) Close() {
	if s.Vault != nil {
		s.Vault.Lock()
	}
	zero(s.Repository.Password)
	zero(s.Repository.SSHKey)
	_ = os.RemoveAll(s.PrivateDir)
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
