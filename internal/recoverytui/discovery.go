package recoverytui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

func recoveryMediaVersion() string {
	executable, err := os.Executable()
	if err != nil {
		return "unrecorded build"
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "WEAZLBACK-VERSION.json"))
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
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	add(filepath.Join(filepath.Dir(executable), "*.wzrk"))
	add(filepath.Join(cwd, "*.wzrk"))
	add("/mnt/*.wzrk")
	add("/mnt/*/*.wzrk")
	sort.Strings(result)
	return result
}
