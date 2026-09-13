package recoverytui

import (
	"encoding/json"
	"github.com/bprendie/weazlback/internal/recovery"
	"os"
	"path/filepath"
	"sort"
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
	seen, result := map[string]bool{}, []string{}
	add := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
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
	sort.Strings(result)
	return result
}
