package freshrestore

import (
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
)

func coreHome(cfg config.Config) string {
	for _, profile := range cfg.Profiles {
		if profile.Name != "core" {
			continue
		}
		for _, path := range profile.Includes {
			if filepath.Base(filepath.Clean(path)) == ".config" {
				return filepath.Dir(filepath.Clean(path))
			}
		}
	}
	return ""
}

func stagedPath(root, absolute string) string {
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(absolute), string(filepath.Separator)))
}
