package recoverytui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/recovery"
)

func recoveryMediaVersion() string {
	executable, err := os.Executable()
	if err != nil {
		return "unrecorded build"
	}
	_ = executable
	body, err := os.ReadFile(filepath.Join(recovery.MediaRoot(), "BUNDLE.json"))
	if err != nil {
		body, err = os.ReadFile(filepath.Join(recovery.MediaRoot(), "WEAZLBACK-VERSION.json"))
	}
	if err != nil {
		return "unrecorded build"
	}
	var value struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(body, &value) != nil || value.Version == "" {
		return "invalid provenance"
	}
	return value.Version
}

func defaultKit(kits []string) string {
	if len(kits) > 0 {
		return kits[0]
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "weazlback-recovery.wzrk")
}

func discoverKits() []string {
	// A launched recovery medium owns its kit. Payload revisions identify tool
	// and credential updates, not backup generations; history is read live from
	// the repository after unlocking this kit.
	root := recovery.MediaRoot()
	if active, err := recovery.ActiveMedia(root); err == nil {
		root = active
	}
	canonical := filepath.Join(root, "weazlback-recovery.wzrk")
	if info, err := os.Stat(canonical); err == nil && info.Mode().IsRegular() {
		return []string{canonical}
	}
	// A missing kit on recognized media must not silently select another drive.
	for _, marker := range []string{"CURRENT", "BUNDLE.json", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return []string{canonical}
		}
	}
	seen, result := map[string]bool{}, []string{}
	add := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			if filepath.Base(path)[0] == '.' {
				continue
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				continue
			}
			absolute, _ := filepath.Abs(path)
			if !seen[absolute] {
				seen[absolute], result = true, append(result, absolute)
			}
		}
	}
	add(filepath.Join(recovery.MediaRoot(), "*.wzrk"))
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	add(filepath.Join(filepath.Dir(executable), "*.wzrk"))
	add(filepath.Join(cwd, "*.wzrk"))
	add("/mnt/*.wzrk")
	add("/mnt/*/*.wzrk")
	return result
}
