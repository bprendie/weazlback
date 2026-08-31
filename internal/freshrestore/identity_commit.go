package freshrestore

import (
	"fmt"
	"os"

	"github.com/bprendie/weazlback/internal/config"
)

func (r *Restore) adoptSourceIdentity() error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	hostname := r.Plan.Snapshot.Hostname
	cfg.Machine = config.Machine{Version: config.MachineSchemaVersion, ID: r.Plan.SourceMachineID,
		Name: hostname, Hostname: hostname, Hostnames: []string{hostname}}
	return config.Save(path, cfg)
}

func (r *Restore) persistTargetIdentity() error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	cfg.Machine = config.Machine{Version: config.MachineSchemaVersion, ID: r.Plan.TargetMachineID,
		Name: hostname, Hostname: hostname, Hostnames: []string{hostname}}
	return config.Save(path, cfg)
}

func (r *Restore) repairInconsistentJournal() error {
	if !stageAtLeast(r.Journal.Stage, "core_committed") || r.journalCoversCore() {
		return nil
	}
	if err := r.validateStage(); err != nil {
		return fmt.Errorf("journal claims Core placement but has no committed paths and staged recovery is unavailable: %w", err)
	}
	r.Journal.Stage = "packages_reconciled"
	return SaveJournal(r.JournalPath, r.Journal)
}

func (r *Restore) journalCoversCore() bool {
	for _, target := range r.expectedTargets() {
		if !contains(r.Journal.CommittedPaths, target) {
			return false
		}
	}
	return true
}

func (r *Restore) validateCommit() error {
	for _, target := range r.expectedTargets() {
		if !contains(r.Journal.CommittedPaths, target) {
			return fmt.Errorf("Core placement invariant failed: %s was not committed", target)
		}
		if _, err := os.Lstat(target); err != nil {
			return fmt.Errorf("Core placement invariant failed: %s is absent: %w", target, err)
		}
	}
	return nil
}
