package preflight

import (
	"io/fs"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/sys/unix"
)

type Report struct {
	Files      uint64   `json:"files"`
	Unreadable uint64   `json:"unreadable"`
	Samples    []string `json:"samples,omitempty"`
	Paths      []string `json:"paths,omitempty"`
}

func Scan(paths, excludes []string) Report {
	var report Report
	for _, root := range paths {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if excluded(path, excludes) {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if walkErr != nil {
				report.mark(path)
				return nil
			}
			if entry.IsDir() {
				if unix.Access(path, unix.R_OK|unix.X_OK) != nil {
					report.mark(path)
					return filepath.SkipDir
				}
				return nil
			}
			report.Files++
			if entry.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			if unix.Access(path, unix.R_OK) != nil {
				report.mark(path)
			}
			return nil
		})
	}
	return report
}

func (r *Report) mark(path string) {
	r.Unreadable++
	r.Paths = append(r.Paths, path)
	if len(r.Samples) < 8 {
		r.Samples = append(r.Samples, path)
	}
}

func excluded(path string, patterns []string) bool {
	path = filepath.ToSlash(path)
	for _, pattern := range patterns {
		matched, _ := doublestar.PathMatch(filepath.ToSlash(pattern), path)
		if matched {
			return true
		}
	}
	return false
}
