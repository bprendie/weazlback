package freshrestore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (r *Restore) commitCore() ([]string, error) {
	var restored []string
	for _, profile := range r.Session.Config.Profiles {
		if profile.Name != "core" {
			continue
		}
		for _, original := range profile.Includes {
			target, err := mapHomePath(original, r.Plan.OriginalHome, r.Plan.TargetHome)
			if err != nil {
				return restored, err
			}
			if contains(r.Journal.CommittedPaths, target) {
				restored = append(restored, target)
				continue
			}
			if err := r.placeJournaled(stagedPath(r.StageDir, original), target); err != nil {
				return restored, err
			}
			restored = append(restored, target)
		}
	}
	return restored, nil
}

func (r *Restore) placeJournaled(source, target string) error {
	backup := target + ".weazlback-before-" + time.Now().Format("20060102-150405")
	if r.Journal.PendingTarget == target {
		source, backup = r.Journal.PendingSource, r.Journal.PendingBackup
	} else {
		r.Journal.PendingSource, r.Journal.PendingTarget, r.Journal.PendingBackup = source, target, backup
		if err := SaveJournal(r.JournalPath, r.Journal); err != nil {
			return err
		}
	}
	_, sourceErr := os.Lstat(source)
	_, targetErr := os.Lstat(target)
	if sourceErr != nil && targetErr == nil {
		return r.recordPlaced(target)
	}
	if _, err := os.Lstat(target); err == nil {
		if _, collision := os.Lstat(backup); collision == nil {
			return fmt.Errorf("recoverable conflict path already exists: %s", backup)
		}
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(source); err == nil {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := moveOrCopy(source, target); err != nil {
			return err
		}
	} else if _, err := os.Lstat(target); err != nil {
		return fmt.Errorf("pending restore has neither staged nor placed path: %s", target)
	}
	return r.recordPlaced(target)
}

func moveOrCopy(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		if !isCrossDevice(err) {
			return err
		}
		return copyAcrossFilesystems(source, target)
	}
	return nil
}

func isCrossDevice(err error) bool {
	linkErr, ok := err.(*os.LinkError)
	return ok && linkErr.Err == syscall.EXDEV
}

func copyAcrossFilesystems(source, target string) error {
	command := exec.Command("cp", "-a", "--reflink=auto", "--", source, target)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("archive-preserving cross-filesystem copy: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Lstat(target); err != nil {
		return fmt.Errorf("cross-filesystem placement validation: %w", err)
	}
	return nil
}

func (r *Restore) recordPlaced(target string) error {
	r.Journal.CommittedPaths = append(r.Journal.CommittedPaths, target)
	r.Journal.PendingSource, r.Journal.PendingTarget, r.Journal.PendingBackup = "", "", ""
	return SaveJournal(r.JournalPath, r.Journal)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func mapHomePath(path, oldHome, newHome string) (string, error) {
	relative, err := filepath.Rel(oldHome, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Core path escapes original home: %s", path)
	}
	return filepath.Join(newHome, relative), nil
}

func placePreserving(source, target string) error {
	if _, err := os.Lstat(source); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		backup := target + ".weazlback-before-" + time.Now().Format("20060102-150405")
		if _, collision := os.Lstat(backup); collision == nil {
			return fmt.Errorf("recoverable conflict path already exists: %s", backup)
		}
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	return os.Rename(source, target)
}
