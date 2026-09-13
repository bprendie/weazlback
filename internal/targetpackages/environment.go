package targetpackages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/platform"
)

type Environment struct {
	Architecture string            `json:"architecture"`
	Platform     platform.Identity `json:"platform"`
	Fingerprint  string            `json:"fingerprint"`
	Config       string            `json:"config"`
}

func CurrentEnvironment(run packagecapsule.Runner) (Environment, error) {
	config, err := run.Output("pacman-conf")
	if err != nil {
		return Environment{}, err
	}
	// pacman-conf expands Include entries. Keep target repository and signature policy,
	// discard machine-local state paths so an exported descriptor remains portable.
	var lines []string
	for _, line := range strings.Split(config, "\n") {
		key, _, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		switch key {
		case "RootDir", "DBPath", "CacheDir", "LogFile", "GPGDir", "HookDir", "Architecture", "DownloadUser":
			continue
		}
		lines = append(lines, line)
	}
	config = strings.Join(lines, "\n")
	identity := platform.Current()
	arch := architecture.Native()
	hash := sha256.Sum256([]byte(config + "\n" + identity.ID + "\n" + identity.Variant + "\n" + identity.Version + "\n" + arch))
	return Environment{Architecture: arch, Platform: identity, Fingerprint: hex.EncodeToString(hash[:]), Config: config}, nil
}
func ExportEnvironment(root string, run packagecapsule.Runner) error {
	e, err := CurrentEnvironment(run)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	// Only public verification material, never the host secret keyring.
	if err := os.MkdirAll(filepath.Join(root, "keyring"), 0700); err != nil {
		return err
	}
	for _, name := range []string{"pubring.gpg", "trustdb.gpg"} {
		data, err := os.ReadFile(filepath.Join("/etc/pacman.d/gnupg", name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "keyring", name), data, 0600); err != nil {
			return err
		}
	}
	b, _ := json.MarshalIndent(e, "", "  ")
	return os.WriteFile(filepath.Join(root, "target.json"), b, 0600)
}
func LoadEnvironment(root string) (Environment, error) {
	var e Environment
	b, err := os.ReadFile(filepath.Join(root, "target.json"))
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(b, &e)
	if err == nil && e.Fingerprint != environmentFingerprint(e) {
		err = fmt.Errorf("target environment changed; re-export it from the target PC")
	}
	if err == nil && (e.Fingerprint == "" || (architecture.Go(e.Architecture) != "arm64" && architecture.Go(e.Architecture) != "amd64")) {
		err = fmt.Errorf("invalid target environment")
	}
	return e, err
}

type IsolatedRunner struct {
	Base        packagecapsule.Runner
	Root        string
	Environment Environment
}

func (r IsolatedRunner) args(name string, args []string) ([]string, error) {
	root, err := filepath.Abs(r.Root)
	if err != nil {
		return nil, err
	}
	if name == "pacman" {
		if len(args) > 0 && args[0] == "-Qq" {
			return nil, nil
		}
		args = append([]string{"--config", filepath.Join(root, "pacman.conf"), "--dbpath", filepath.Join(root, "db"), "--cachedir", filepath.Join(root, "cache"), "--logfile", filepath.Join(root, "pacman.log"), "--gpgdir", filepath.Join(root, "keyring"), "--arch", r.Environment.Architecture}, args...)
	}
	if name == "pacman-key" {
		args = append([]string{"--gpgdir", filepath.Join(root, "keyring")}, args...)
	}
	return args, nil
}
func (r IsolatedRunner) Output(name string, args ...string) (string, error) {
	a, err := r.args(name, args)
	if err != nil {
		return "", err
	}
	if name == "pacman" && a == nil {
		return "", nil
	}
	return r.Base.Output(name, a...)
}
func (r IsolatedRunner) Run(name string, args ...string) error {
	a, err := r.args(name, args)
	if err != nil {
		return err
	}
	if name == "pacman" {
		return r.Base.Run("sudo", append([]string{"pacman"}, a...)...)
	}
	return r.Base.Run(name, a...)
}
func (r IsolatedRunner) RunDir(dir, name string, args ...string) error {
	return r.Base.RunDir(dir, name, args...)
}
func (r IsolatedRunner) Initialize() error {
	for _, d := range []string{"db", "cache"} {
		if err := os.MkdirAll(filepath.Join(r.Root, d), 0700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(r.Root, "pacman.conf"), []byte(r.Environment.Config), 0600); err != nil {
		return err
	}
	return r.Run("pacman", "-Sy", "--noconfirm")
}

func environmentFingerprint(e Environment) string {
	hash := sha256.Sum256([]byte(e.Config + "\n" + e.Platform.ID + "\n" + e.Platform.Variant + "\n" + e.Platform.Version + "\n" + e.Architecture))
	return hex.EncodeToString(hash[:])
}
