package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/recovery"
)

func removeMatchingRecoveryKits(repositoryID string, passphrase []byte) (int, int) {
	removed, unreachable := 0, 0
	for _, path := range mountedRecoveryKits() {
		bundle, err := recovery.Open(path, passphrase)
		if err != nil {
			unreachable++
			continue
		}
		var cfg config.Config
		err = json.Unmarshal(bundle.Config, &cfg)
		bundle.Close()
		if err != nil || findDestination(cfg, repositoryID) == nil {
			continue
		}
		if err := os.Remove(path); err != nil {
			unreachable++
		} else {
			removed++
		}
	}
	return removed, unreachable
}

func mountedRecoveryKits() []string {
	home, _ := os.UserHomeDir()
	user := filepath.Base(home)
	patterns := []string{
		"/mnt/*.wzrk", "/mnt/*/*.wzrk",
		filepath.Join("/media", user, "*", "*.wzrk"),
		filepath.Join("/run/media", user, "*", "*.wzrk"),
	}
	seen := map[string]bool{}
	var result []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				result = append(result, match)
			}
		}
	}
	return result
}
