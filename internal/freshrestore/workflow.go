package freshrestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/restic"
)

type Restore struct {
	Options         Options
	Session         *Session
	Plan            Plan
	Journal         Journal
	JournalPath     string
	StageDir        string
	PackageStageDir string
	service         restic.Service
}

func Prepare(ctx context.Context, options Options) (*Restore, error) {
	if options.RecoveryPath == "" {
		return nil, errors.New("recovery kit path is required")
	}
	if options.TargetHome == "" {
		options.TargetHome, _ = os.UserHomeDir()
	}
	if options.Scope == "" {
		options.Scope = "core"
	}
	if options.Engine == "" {
		options.Engine = EngineStandard
	}
	if options.Engine != EngineStandard && options.Engine != EngineTurbo {
		return nil, fmt.Errorf("invalid recovery engine %q", options.Engine)
	}
	if !validRecoveryScope(options.Scope) {
		return nil, fmt.Errorf("invalid recovery scope %q", options.Scope)
	}
	if options.WorkDir == "" {
		options.WorkDir = filepath.Join("/var/tmp", fmt.Sprintf("weazlback-%d", os.Getuid()), "restore")
	}
	if err := os.MkdirAll(options.WorkDir, 0o700); err != nil {
		return nil, err
	}
	session, err := OpenSessionDestinationAt(options.RecoveryPath, options.Passphrase, options.Destination, options.Repository)
	if err != nil {
		return nil, err
	}
	r := &Restore{Options: options, Session: session, service: restic.NewService(os.Stderr)}
	if options.AdoptLocal {
		if err := AdoptLocalRepository(ctx, session.Destination, session.Repository.Location); err != nil {
			r.Close()
			return nil, err
		}
	}
	if options.Connections > 0 {
		session.Repository.Connections = options.Connections
	}
	if err := r.service.Check(ctx, session.Repository, false); err != nil {
		r.Close()
		return nil, ClassifyRepositoryError(session.Destination, session.Repository.Location, err)
	}
	if repositoryID, identityErr := r.service.RepositoryID(ctx, session.Repository); identityErr != nil {
		r.Close()
		return nil, fmt.Errorf("verify repository identity: %w", identityErr)
	} else if session.Destination.RepositoryID != "" && session.Destination.RepositoryID != repositoryID {
		r.Close()
		return nil, fmt.Errorf("repository identity mismatch: expected %s, got %s", session.Destination.RepositoryID, repositoryID)
	}
	snapshots, err := r.service.Snapshots(ctx, session.Repository)
	if err != nil {
		r.Close()
		return nil, err
	}
	machineID := options.MachineID
	if machineID == "" {
		machineID = session.Config.Machine.ID
	}
	filtered := restic.FilterIdentity(snapshots, machineID)
	if len(filtered) == 0 {
		identities := restic.GroupIdentities(snapshots, session.Config.Machine.ID, session.Config.Machine.Name)
		if len(identities) == 1 {
			machineID, filtered = identities[0].ID, restic.FilterIdentity(snapshots, identities[0].ID)
		} else {
			r.Close()
			return nil, errors.New("source machine identity must be selected")
		}
	}
	if options.AdoptSourceIdentity && !config.ValidMachineID(machineID) {
		r.Close()
		return nil, errors.New("legacy history must be adopted into a stable machine identity before replacement adoption")
	}
	snapshots = filtered
	snapshot, err := selectCoreSnapshot(snapshots, options.Snapshot)
	if err != nil {
		r.Close()
		return nil, err
	}
	originalHome := coreHome(session.Config)
	targetMachineID := options.TargetMachineID
	if targetMachineID == "" {
		targetMachineID = session.Config.Machine.ID
	}
	if !config.ValidMachineID(targetMachineID) {
		r.Close()
		return nil, errors.New("target machine identity is invalid")
	}
	r.Plan = Plan{Snapshot: snapshot, OriginalHome: originalHome, TargetHome: options.TargetHome, Scope: options.Scope,
		SourceMachineID: machineID, TargetMachineID: targetMachineID, AdoptSourceIdentity: options.AdoptSourceIdentity,
		PersistTargetIdentity: options.PersistTargetIdentity}
	r.Plan.PackageSnapshot = selectLatestPackageSnapshot(snapshots)
	if options.Scope == "home" || options.Scope == "everything" {
		home, selectErr := selectProfileSnapshotAt(snapshots, "home", snapshot.Time)
		if selectErr != nil {
			r.Close()
			return nil, selectErr
		}
		r.Plan.HomeSnapshot = &home
	}
	if options.Scope == "everything" {
		heavy, selectErr := selectProfileSnapshotAt(snapshots, "heavy", snapshot.Time)
		if selectErr != nil {
			r.Close()
			return nil, selectErr
		}
		r.Plan.HeavySnapshot = &heavy
	}
	files, err := r.service.Files(ctx, session.Repository, snapshot.ID)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.Plan.SourceUID, r.Plan.SourceGID = snapshotOwner(files, filepath.Join(originalHome, ".config"))
	requiredBytes := fileBytes(files)
	r.Plan.TargetUID, r.Plan.TargetGID = pathOwner(options.TargetHome)
	if r.Plan.HomeSnapshot != nil {
		homeFiles, listErr := r.service.Files(ctx, session.Repository, r.Plan.HomeSnapshot.ID)
		if listErr != nil {
			r.Close()
			return nil, listErr
		}
		r.Plan.PlacementPaths = topLevelTargets(homeFiles, originalHome, options.TargetHome)
		if homeBytes := fileBytes(homeFiles); homeBytes > requiredBytes {
			requiredBytes = homeBytes
		}
	}
	if r.Plan.HeavySnapshot != nil {
		heavyFiles, listErr := r.service.Files(ctx, session.Repository, r.Plan.HeavySnapshot.ID)
		if listErr != nil {
			r.Close()
			return nil, listErr
		}
		r.Plan.HeavyPlacementPaths = topLevelTargets(heavyFiles, originalHome, options.TargetHome)
		requiredBytes += fileBytes(heavyFiles)
	}
	manifest, err := r.restoreApplicationManifest(ctx)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.Plan.Applications = &manifest
	r.Plan.SourceHostname = manifest.Hostname
	r.Plan.Hostname, err = ResolveHostname(options.Hostname, manifest.Hostname)
	if err != nil {
		r.Close()
		return nil, err
	}
	r.Plan.Official, r.Plan.AUR, r.Plan.Flatpak = MissingApplications(ctx, manifest)
	r.Plan.SystemServices, r.Plan.UserServices = MissingServices(ctx, manifest)
	if r.Plan.PackageSnapshot != nil {
		capsule, manifestPath, capsuleErr := r.loadPackageCapsule(ctx)
		if capsuleErr != nil {
			r.Close()
			return nil, fmt.Errorf("load Package Capsule: %w", capsuleErr)
		}
		delta, deltaErr := resolvePackageDelta(ctx, capsule)
		if deltaErr != nil {
			r.Close()
			return nil, fmt.Errorf("resolve Package Capsule delta: %w", deltaErr)
		}
		r.Plan.PackageCapsule, r.Plan.PackageManifestPath, r.Plan.PackageDelta = &capsule, manifestPath, delta
		// A selected capsule supersedes legacy AUR files embedded in old Core
		// manifests. Mixing generations would violate the planned ledger.
		r.Plan.ArtifactFiles = nil
		r.Plan.Official, r.Plan.AUR = append([]string(nil), delta.OfficialOnline...), append([]string(nil), delta.ForeignOnline...)
		r.Plan.Flatpak = missing(capsule.Flatpaks, commandSet(ctx, "flatpak", "list", "--app", "--columns=application"))
		capsuleApps := inventory.ApplicationManifest{Services: inventory.ServiceInventory{SystemEnabled: capsule.SystemUnits, UserEnabled: capsule.UserUnits}}
		r.Plan.SystemServices, r.Plan.UserServices = MissingServices(ctx, capsuleApps)
	}
	r.Plan.LocalApps = append([]string(nil), manifest.WeazlApps...)
	r.JournalPath = filepath.Join(options.WorkDir, session.Destination.ID+"-"+options.Scope+"-journal.json")
	r.StageDir = filepath.Join(options.WorkDir, session.Destination.ID+"-"+options.Scope+"-stage")
	r.PackageStageDir = r.StageDir + "-packages"
	r.Journal, err = LoadJournal(r.JournalPath)
	if err != nil {
		r.Close()
		return nil, err
	}
	if r.Journal.SnapshotID != "" && r.Journal.SnapshotID != snapshot.ID {
		r.Close()
		return nil, errors.New("existing restore journal belongs to another Restore Point")
	}
	if r.Plan.HomeSnapshot != nil && r.Journal.HomeSnapshotID != "" && r.Journal.HomeSnapshotID != r.Plan.HomeSnapshot.ID {
		r.Close()
		return nil, errors.New("existing restore journal belongs to another Home Restore Point")
	}
	if r.Plan.HeavySnapshot != nil && r.Journal.HeavySnapshotID != "" && r.Journal.HeavySnapshotID != r.Plan.HeavySnapshot.ID {
		r.Close()
		return nil, errors.New("existing restore journal belongs to another Heavy Restore Point")
	}
	if r.Plan.PackageSnapshot != nil && r.Journal.PackageSnapshotID != "" && r.Journal.PackageSnapshotID != r.Plan.PackageSnapshot.ID {
		r.Close()
		return nil, errors.New("existing restore journal belongs to another Package Capsule Restore Point")
	}
	if r.Journal.Scope != "" && r.Journal.Scope != options.Scope {
		r.Close()
		return nil, errors.New("existing restore journal belongs to another recovery scope")
	}
	if options.Connections == 0 {
		if r.Journal.Connections > 0 {
			r.Session.Repository.Connections = r.Journal.Connections
		} else {
			tuning := r.service.TuneRestoreConnections(ctx, session.Repository, snapshot.ID, files, options.WorkDir)
			r.Session.Repository.Connections = tuning.Selected
			r.Journal.Connections = tuning.Selected
		}
	}
	if r.Journal.Stage == "" {
		r.Journal = Journal{RepositoryID: session.Destination.ID, SnapshotID: snapshot.ID,
			Stage: "snapshot_selected", Hostname: r.Plan.Hostname, TargetHome: options.TargetHome,
			Connections: r.Session.Repository.Connections, Scope: options.Scope, Engine: EngineStandard, RequestedEngine: EngineStandard}
		if r.Plan.HomeSnapshot != nil {
			r.Journal.HomeSnapshotID = r.Plan.HomeSnapshot.ID
		}
		if r.Plan.HeavySnapshot != nil {
			r.Journal.HeavySnapshotID = r.Plan.HeavySnapshot.ID
		}
		if r.Plan.PackageSnapshot != nil {
			r.Journal.PackageSnapshotID = r.Plan.PackageSnapshot.ID
		}
	}
	if r.Journal.Stage == "snapshot_selected" {
		r.configureEngine(requiredBytes)
	}
	err = SaveJournal(r.JournalPath, r.Journal)
	return r, err
}

func (r *Restore) configureEngine(requiredBytes uint64) {
	r.Journal.Engine, r.Journal.RequestedEngine = EngineStandard, EngineStandard
	r.Journal.FallbackEngine, r.Journal.FallbackPhase, r.Journal.FallbackReason = "", "", ""
	requestedTurbo := r.Options.Engine == EngineTurbo || (r.Options.RestoreEngine != nil && r.Options.RestoreEngine.Name() != EngineStandard)
	if !requestedTurbo {
		return
	}
	transport := "local"
	if r.Session.Destination.Kind == "ssh" {
		transport = "ssh"
	}
	sourcePath := ""
	if transport == "local" {
		sourcePath = r.Session.Repository.Location
	}
	r.Journal.Qualification = QualifyTurboSource(r.Options.TargetHome, sourcePath, transport, r.Options.TurboPolicy)
	requireRestoreSpace(&r.Journal.Qualification, requiredBytes)
	r.Journal.RequestedEngine = EngineTurbo
	if r.Journal.Qualification.Eligible {
		r.Journal.Engine = EngineTurbo
		return
	}
	r.Journal.FallbackEngine, r.Journal.FallbackPhase = EngineStandard, "qualification"
	r.Journal.FallbackReason = strings.Join(append(r.Journal.Qualification.HardFailures, r.Journal.Qualification.SoftFindings...), "; ")
}

func validRecoveryScope(scope string) bool {
	return scope == "core" || scope == "home" || scope == "everything" || scope == "applications"
}
