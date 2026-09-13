package targetpackages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/platform"
)

// ImportOmaSnap derives an ARM target descriptor from the online source profile
// and public keyring of a locally built installer. It never runs build scripts or
// copies the installer's offline-only pacman configuration or private keys.
func ImportOmaSnap(project, output string, run packagecapsule.Runner) error {
	project, err := filepath.Abs(project)
	if err != nil {
		return err
	}
	profile := filepath.Join(project, "profiles", "t14s-lcd")
	config, err := os.ReadFile(filepath.Join(profile, "pacman.conf"))
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(profile, "mirrorlist")); err != nil {
		return err
	}
	root, err := omaSnapBuildRoot(project)
	if err != nil {
		return err
	}
	release, err := os.ReadFile(filepath.Join(root, "etc", "os-release"))
	if err != nil {
		return err
	}
	facts := map[string]string{}
	for _, line := range strings.Split(string(release), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			facts[key] = strings.Trim(value, "\"'")
		}
	}
	if facts["ID"] != "omarchy" || facts["VERSION_ID"] == "" {
		return fmt.Errorf("oma_snap build must identify its Omarchy version")
	}
	identity := platform.Detect(platform.Facts{OSRelease: facts, Architecture: "arm64", Omarchy: true})
	// Let pacman-conf normalize exactly as it does for a physical target export.
	// Only redirect the profile's mirror include; other unresolved host includes
	// would accidentally mix the x64 host configuration into this ARM target.
	var lines []string
	for _, line := range strings.Split(string(config), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "Architecture" {
			line = "Architecture = aarch64"
		}
		if ok && strings.TrimSpace(key) == "Include" {
			if strings.TrimSpace(value) != "/etc/pacman.d/mirrorlist" {
				return fmt.Errorf("unsupported oma_snap profile include: %s", strings.TrimSpace(value))
			}
			line = "Include = " + filepath.Join(profile, "mirrorlist")
		}
		lines = append(lines, line)
	}
	temporary, err := os.CreateTemp("", "weazlback-oma-snap-*.conf")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.WriteString(strings.Join(lines, "\n")); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	expanded, err := run.Output("pacman-conf", "--config", temporary.Name())
	if err != nil {
		return err
	}
	if !strings.Contains(expanded, "Architecture = aarch64") {
		return fmt.Errorf("oma_snap profile must declare Architecture = auto or aarch64")
	}
	if strings.Contains(expanded, "file://") {
		return fmt.Errorf("oma_snap application preparation needs online repositories, not the installer-only offline mirror")
	}
	env := environmentFromConfig(expanded, identity, "aarch64")
	return writeEnvironment(output, filepath.Join(root, "etc", "pacman.d", "gnupg"), env)
}

func omaSnapBuildRoot(project string) (string, error) {
	roots, err := filepath.Glob(filepath.Join(project, "build", "snapdragon-v*-installer-root"))
	if err != nil {
		return "", err
	}
	// Prefer the most recently built image; the source profile remains the authority
	// for repository URLs and holds because installer roots use offline repositories.
	sort.SliceStable(roots, func(i, j int) bool {
		a, ea := os.Stat(filepath.Join(roots[i], "etc", "os-release"))
		b, eb := os.Stat(filepath.Join(roots[j], "etc", "os-release"))
		if ea != nil {
			return false
		}
		if eb != nil {
			return true
		}
		if a.ModTime().Equal(b.ModTime()) {
			return roots[i] > roots[j]
		}
		return a.ModTime().After(b.ModTime())
	})
	for _, root := range roots {
		ready := true
		for _, name := range []string{"etc/os-release", "etc/pacman.d/gnupg/pubring.gpg", "etc/pacman.d/gnupg/trustdb.gpg"} {
			info, err := os.Lstat(filepath.Join(root, name))
			if err != nil || !info.Mode().IsRegular() {
				ready = false
				break
			}
		}
		if ready {
			return root, nil
		}
	}
	return "", fmt.Errorf("oma_snap has no built Snapdragon installer with public package keys; build it or import settings exported from the Snapdragon PC")
}
