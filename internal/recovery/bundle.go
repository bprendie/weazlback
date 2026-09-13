package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/architecture"
)

const BundleManifestName = "BUNDLE.json"

type BundleManifest struct {
	Version       string            `json:"version"`
	ResticVersion string            `json:"restic_version"`
	Files         map[string]string `json:"files"`
}

var BundleArchitectures = []string{"amd64", "arm64"}
var BundleRuntime = []string{"scripts/install.sh", "scripts/launch.sh", "scripts/uninstall.sh", "scripts/weazlback-session", "systemd/weazlback-backup.service", "systemd/weazlback-backup.timer", "manifest.json", "BarWidget.qml", "widget/weazlback/Model.js", "widget/weazlback/BarWidget.qml", "widget/weazlback/Panel.qml", "widget/weazlback/Lane.qml"}
var BundleTools = []string{"weazlback", "weazlback-restore", "restic"}

// InstalledBundle locates and validates the universal payload used by USB preparation.
func InstalledBundle(version string) (string, error) {
	if root := os.Getenv("WEAZLBACK_BUNDLE_DIR"); root != "" {
		_, err := ValidateBundle(root, version)
		return root, err
	}
	var candidates []string
	if executable, err := os.Executable(); err == nil {
		parent := filepath.Dir(filepath.Dir(executable))
		candidates = append(candidates, filepath.Join(parent, "recovery-bundle"), parent)
	}
	root := os.Getenv("WEAZLBACK_HOME")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".weazlback")
		}
	}
	if root != "" {
		candidates = append(candidates, filepath.Join(root, "recovery-bundle"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, BundleManifestName)); err != nil {
			continue
		}
		if _, err := ValidateBundle(candidate, version); err != nil {
			return "", fmt.Errorf("invalid universal recovery bundle %s: %w", candidate, err)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("universal recovery bundle for %s is missing; install the universal release or set WEAZLBACK_BUNDLE_DIR", version)
}

func ValidateBundle(root, version string) (BundleManifest, error) {
	var m BundleManifest
	data, err := os.ReadFile(filepath.Join(root, BundleManifestName))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.Version == "" || (version != "" && version != m.Version) || m.ResticVersion == "" {
		return m, fmt.Errorf("bundle version does not match %s", version)
	}
	for _, arch := range BundleArchitectures {
		for _, tool := range BundleTools {
			name := "linux-" + arch + "/" + tool
			path := filepath.Join(root, name)
			if err := regularWithin(root, name); err != nil {
				return m, err
			}
			actual, err := fileSHA256(path)
			if err != nil || m.Files[name] != actual {
				return m, fmt.Errorf("bundle checksum mismatch: %s", name)
			}
			if err := architecture.ValidateELF(path, arch); err != nil {
				return m, err
			}
		}
	}
	for _, name := range BundleRuntime {
		if m.Files[name] == "" {
			return m, fmt.Errorf("missing runtime asset: %s", name)
		}
	}
	for name, wanted := range m.Files {
		if err := regularWithin(root, name); err != nil {
			return m, err
		}
		actual, err := fileSHA256(filepath.Join(root, name))
		if err != nil || wanted != actual {
			return m, fmt.Errorf("bundle checksum mismatch: %s", name)
		}
	}
	return m, nil
}

// MediaRoot works for native binaries in either legacy or universal media.
func MediaRoot() string {
	if root := os.Getenv("WEAZLBACK_MEDIA_ROOT"); root != "" {
		return root
	}
	executable, err := os.Executable()
	if err != nil {
		return filepath.Dir(os.Args[0])
	}
	root := filepath.Dir(executable)
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), BundleManifestName)); err == nil {
		return filepath.Dir(root)
	}
	return root
}
