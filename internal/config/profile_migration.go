package config

import (
	"os"
	"path/filepath"
)

func migrateHomeProfile(profile *Profile) bool {
	home, _ := os.UserHomeDir()
	legacy := filepath.Join(home, ".cache", "**")
	filtered := make([]string, 0, len(profile.Excludes))
	for _, value := range profile.Excludes {
		if value != legacy {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == len(profile.Excludes) {
		return false
	}
	profile.Excludes = filtered
	return true
}
