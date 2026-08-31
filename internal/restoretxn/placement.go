package restoretxn

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func placeAtomic(source, target, rollback string) error {
	if _, err := os.Lstat(source); err != nil {
		return fmt.Errorf("staged path missing: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(target), ".weazlback-place-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	candidate := filepath.Join(temporary, filepath.Base(target))
	if err := copyPreserving(source, candidate); err != nil {
		return err
	}
	if err := validatePair(source, candidate); err != nil {
		return fmt.Errorf("validate placement candidate: %w", err)
	}
	if _, err := os.Lstat(target); err == nil {
		if _, collision := os.Lstat(rollback); collision == nil {
			return fmt.Errorf("rollback path already exists: %s", rollback)
		}
		if err := os.Rename(target, rollback); err != nil {
			return fmt.Errorf("preserve live object: %w", err)
		}
	}
	if err := os.Rename(candidate, target); err != nil {
		_ = rollbackOne(target, rollback)
		return fmt.Errorf("commit placement: %w", err)
	}
	return nil
}

func plannedRollback(target string, now time.Time) string {
	if _, err := os.Lstat(target); err != nil {
		return ""
	}
	return target + ".weazlback-before-" + now.Format("20060102-150405.000000000")
}

func copyPreserving(source, target string) error {
	command := exec.Command("cp", "-a", "--sparse=always", "--reflink=auto", "--", source, target)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("archive-preserving copy: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func validatePair(source, target string) error {
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeType != targetInfo.Mode()&os.ModeType || sourceInfo.Mode().Perm() != targetInfo.Mode().Perm() {
		return fmt.Errorf("type or mode mismatch for %s", target)
	}
	if sourceInfo.Mode().IsRegular() {
		if sourceInfo.Size() != targetInfo.Size() {
			return fmt.Errorf("size mismatch for %s", target)
		}
		if sourceInfo.Size() <= 16<<20 {
			left, leftErr := digest(source)
			right, rightErr := digest(target)
			if leftErr != nil || rightErr != nil || left != right {
				return fmt.Errorf("checksum mismatch for %s", target)
			}
		}
	}
	if sourceInfo.IsDir() {
		sourceFiles, sourceBytes := pathTotals(source)
		targetFiles, targetBytes := pathTotals(target)
		if sourceFiles != targetFiles || sourceBytes != targetBytes {
			return fmt.Errorf("tree validation mismatch for %s", target)
		}
	}
	return nil
}

func validateStaged(plan Plan) error {
	for _, item := range plan.Items {
		path := stagedPath(plan.StageRoot, item.SourcePath)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("validate staged %s: %w", item.SourcePath, err)
		}
		if item.Entry.Type == "file" && (!info.Mode().IsRegular() || uint64(info.Size()) != item.Entry.Size) {
			return fmt.Errorf("staged file metadata mismatch: %s", item.SourcePath)
		}
		if item.Entry.Type == "dir" && !info.IsDir() {
			return fmt.Errorf("staged directory type mismatch: %s", item.SourcePath)
		}
		if item.Entry.Type == "symlink" && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("staged symlink type mismatch: %s", item.SourcePath)
		}
	}
	return nil
}

func digest(path string) ([32]byte, error) {
	var result [32]byte
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return result, err
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func pathTotals(path string) (uint64, uint64) {
	var files, bytes uint64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil {
			files++
			if info.Mode().IsRegular() {
				bytes += uint64(info.Size())
			}
		}
		return nil
	})
	return files, bytes
}

func stagedTotals(plan Plan) (uint64, uint64) {
	var files, bytes uint64
	for _, item := range plan.Items {
		itemFiles, itemBytes := pathTotals(stagedPath(plan.StageRoot, item.SourcePath))
		files, bytes = files+itemFiles, bytes+itemBytes
	}
	return files, bytes
}

func mapOwnership(path string, plan Plan) error {
	return filepath.Walk(path, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || uint32(stat.Uid) != plan.SourceUID || uint32(stat.Gid) != plan.SourceGID {
			return nil
		}
		if uint32(stat.Uid) == plan.TargetUID && uint32(stat.Gid) == plan.TargetGID {
			return nil
		}
		if err := os.Lchown(current, int(plan.TargetUID), int(plan.TargetGID)); err != nil {
			return fmt.Errorf("map ownership for %s: %w", current, err)
		}
		return nil
	})
}

func rollbackOne(target, rollback string) error {
	if rollback == "" {
		return os.RemoveAll(target)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(rollback, target)
}
