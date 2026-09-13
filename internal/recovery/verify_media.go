package recovery

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func VerifyMediaDirectory(directory string) error {
	active, err := ActiveMedia(directory)
	if err != nil {
		return err
	}
	directory = active
	checksumPath := filepath.Join(directory, "SHA256SUMS")
	file, err := os.Open(checksumPath)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	verified := 0
	seen := map[string]bool{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 || !filepath.IsLocal(fields[1]) {
			return fmt.Errorf("invalid recovery-media checksum entry")
		}
		if seen[fields[1]] {
			return fmt.Errorf("duplicate checksum entry")
		}
		seen[fields[1]] = true
		if err := regularWithin(directory, fields[1]); err != nil {
			return err
		}
		body, err := os.ReadFile(filepath.Join(directory, fields[1]))
		if err != nil {
			return fmt.Errorf("verify %s: %w", fields[1], err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != fields[0] {
			return fmt.Errorf("recovery-media checksum failed: %s", fields[1])
		}
		verified++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(directory, BundleManifestName)); err == nil {
		bundle, err := ValidateBundle(directory, "")
		if err != nil {
			return err
		}
		for name := range bundle.Files {
			if !seen[name] {
				return fmt.Errorf("missing asset checksum: %s", name)
			}
		}
		for _, name := range []string{BundleManifestName, "weazlback-recovery.wzrk", "RESTORE.txt", "THIRD_PARTY_NOTICES.txt"} {
			if !seen[name] {
				return fmt.Errorf("missing checksum: %s", name)
			}
		}
		for _, arch := range BundleArchitectures {
			for _, tool := range BundleTools {
				if !seen["linux-"+arch+"/"+tool] {
					return fmt.Errorf("missing binary checksum")
				}
			}
		}
	}
	if verified < 4 {
		return fmt.Errorf("recovery media checksum set is incomplete")
	}
	return nil
}

func regularWithin(root, name string) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("unsafe media path")
	}
	current := root
	for _, part := range strings.Split(filepath.Clean(name), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("media symlink refused: %s", name)
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("media entry is not a file: %s", name)
	}
	return nil
}
