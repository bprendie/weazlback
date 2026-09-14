package freshrestore

import (
	"fmt"
	"time"
)

// Completed journals from a broader preservation policy must not cause a new
// Core recovery to skip placement. Keep the previous journal and rollback paths.
func (r *Restore) resetChangedMigrationPolicy() error {
	if !r.Plan.Migration.Active || r.Journal.Stage == "" || r.Journal.Migration.PolicyVersion == r.Plan.Migration.PolicyVersion {
		return nil
	}
	if r.Journal.PendingTarget != "" {
		return fmt.Errorf("finish or resolve the pending placement before changing migration policy: %s", r.Journal.PendingTarget)
	}
	backup := r.JournalPath + ".before-policy-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := SaveJournal(backup, r.Journal); err != nil {
		return err
	}
	r.Journal = Journal{}
	return nil
}
