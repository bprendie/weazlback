package targetpackages

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
)

type Plan struct {
	Versions        map[string]string `json:"versions"`
	Architecture    string            `json:"architecture"`
	Fingerprint     string            `json:"target_fingerprint"`
	InventoryDigest string            `json:"inventory_digest"`
	Official        []string          `json:"official"`
	Flatpak         []string          `json:"flatpak"`
	Build           []string          `json:"build_required"`
	Kept            []string          `json:"kept"`
	Review          []string          `json:"review"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9@._+:-]*$`)

func SafeName(name string) bool { return safeName.MatchString(name) }
func Hardware(name string) bool {
	for _, prefix := range []string{"linux", "firmware", "nvidia", "intel-ucode", "amd-ucode", "grub", "systemd", "uboot", "u-boot", "qcom", "snapdragon", "mesa", "vulkan", "xf86-video", "mkinitcpio", "dracut", "pacman", "archlinux", "omarchy", "oma-snap", "archlinuxarm"} {
		if name == prefix || strings.HasPrefix(name, prefix+"-") {
			return true
		}
	}
	return false
}
func Digest(m inventory.ApplicationManifest) string {
	p := m.PackagePlan
	p.Official = append([]string{}, p.Official...)
	p.AUR = append([]string{}, p.AUR...)
	p.Flatpak = append([]string{}, p.Flatpak...)
	sort.Strings(p.Official)
	sort.Strings(p.AUR)
	sort.Strings(p.Flatpak)
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Resolve(m inventory.ApplicationManifest, arch, fingerprint string, run packagecapsule.Runner) Plan {
	return resolveWithProgress(m, arch, fingerprint, run, nil, nil)
}

func resolveWithProgress(m inventory.ApplicationManifest, arch, fingerprint string, run packagecapsule.Runner, progress func(packagecapsule.Progress), held []string) Plan {
	p := Plan{Versions: map[string]string{}, Architecture: architecture.Package(arch), Fingerprint: fingerprint, InventoryDigest: Digest(m)}
	installed, _ := run.Output("pacman", "-Qq")
	have := map[string]bool{}
	for _, name := range strings.Fields(installed) {
		have[name] = true
	}
	seen := map[string]bool{}
	names := append(append([]string{}, m.PackagePlan.Official...), m.PackagePlan.AUR...)
	for index, name := range names {
		if progress != nil {
			progress(packagecapsule.Progress{Phase: "resolving", Package: name, Source: "official", Completed: index, Total: len(names)})
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if !SafeName(name) {
			p.Review = append(p.Review, "invalid package name withheld")
			continue
		}
		if matchesHeld(name, held) {
			p.Kept = append(p.Kept, name+" (held by target policy)")
			continue
		}
		if have[name] {
			p.Kept = append(p.Kept, name)
			continue
		}
		if Hardware(name) {
			p.Kept = append(p.Kept, name+" (target hardware/base equivalent)")
			continue
		}
		if _, err := run.Output("pacman", "-T", "--", name); err == nil {
			p.Kept = append(p.Kept, name+" (target provider)")
			continue
		}
		out, err := run.Output("pacman", "-Si", "--", name)
		if err != nil {
			p.Build = append(p.Build, name)
			p.Review = append(p.Review, name+": unavailable in target repositories; native source build or replacement requires review")
			continue
		}
		meta := Fields(out)
		if !architecture.Compatible(meta["Architecture"], arch) {
			p.Review = append(p.Review, name+": incompatible repository architecture")
			continue
		}
		conflict := false
		for _, item := range strings.Fields(meta["Conflicts With"] + " " + meta["Replaces"]) {
			item = strings.FieldsFunc(item, func(r rune) bool { return strings.ContainsRune("<>=", r) })[0]
			if have[item] {
				conflict = true
			}
		}
		if conflict {
			p.Review = append(p.Review, name+": would replace/conflict with target package")
			continue
		}
		p.Official = append(p.Official, name)
		p.Versions[name] = meta["Version"]
	}
	for _, app := range m.PackagePlan.Flatpak {
		if !SafeName(app) {
			continue
		}
		if _, err := run.Output("flatpak", "info", "--arch="+p.Architecture, app); err == nil {
			continue
		}
		if _, err := run.Output("flatpak", "remote-info", "--arch="+p.Architecture, "flathub", app); err != nil {
			p.Review = append(p.Review, app+": no target Flatpak available")
			continue
		}
		p.Flatpak = append(p.Flatpak, app)
	}
	sort.Strings(p.Official)
	sort.Strings(p.Kept)
	sort.Strings(p.Build)
	return p
}
func Fields(output string) map[string]string {
	result := map[string]string{}
	last := ""
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			last = strings.TrimSpace(key)
			result[last] = strings.TrimSpace(value)
		} else if strings.HasPrefix(line, " ") {
			result[last] += " " + strings.TrimSpace(line)
		}
	}
	return result
}

// Freeze the installed baseline during migration. If new apps require changing it,
// report the transaction for review instead of upgrading the custom image blindly.
func CheckTransaction(run packagecapsule.Runner, args []string) error {
	have, err := run.Output("pacman", "-Qq")
	if err != nil {
		return err
	}
	installed := map[string]bool{}
	for _, n := range strings.Fields(have) {
		installed[n] = true
	}
	output, err := run.Output("pacman", append([]string{"-Sp", "--needed", "--print-format", "%n", "--"}, args...)...)
	if err != nil {
		return err
	}
	for _, n := range strings.Fields(output) {
		info, err := run.Output("pacman", "-Si", "--", n)
		if err != nil {
			return err
		}
		if err := rejectConflicts(run, Fields(info)); err != nil {
			return err
		}
		if installed[n] {
			return fmt.Errorf("application transaction requires changing target package %s; update the target image explicitly, then re-plan", n)
		}
		if Hardware(n) {
			return fmt.Errorf("application transaction introduces hardware/base package %s; review on target", n)
		}
	}
	return nil
}

func CheckLocalTransaction(run packagecapsule.Runner, files map[string]string) error {
	have, err := run.Output("pacman", "-Qq")
	if err != nil {
		return err
	}
	installed := map[string]bool{}
	for _, n := range strings.Fields(have) {
		installed[n] = true
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	args := []string{"-Up", "--needed", "--print-format", "%n", "--"}
	for _, name := range names {
		info, err := run.Output("pacman", "-Qip", "--", files[name])
		if err != nil {
			return err
		}
		if err := rejectConflicts(run, Fields(info)); err != nil {
			return err
		}
		args = append(args, files[name])
	}
	output, err := run.Output("pacman", args...)
	if err != nil {
		return err
	}
	for _, n := range strings.Fields(output) {
		if files[n] == "" {
			info, err := run.Output("pacman", "-Si", "--", n)
			if err != nil {
				return err
			}
			if err := rejectConflicts(run, Fields(info)); err != nil {
				return err
			}
		}
		if installed[n] || Hardware(n) {
			return fmt.Errorf("local transaction changes target baseline package %s", n)
		}
	}
	return nil
}

func rejectConflicts(run packagecapsule.Runner, meta map[string]string) error {
	for _, dep := range strings.Fields(meta["Conflicts With"] + " " + meta["Replaces"]) {
		if dep == "None" {
			continue
		}
		if _, err := run.Output("pacman", "-T", "--", dep); err == nil {
			return fmt.Errorf("package transaction conflicts with installed provider of %s", dep)
		}
	}
	return nil
}

// ResolveEnvironment applies the target's package holds during both preparation and restore.
func ResolveEnvironment(m inventory.ApplicationManifest, env Environment, run packagecapsule.Runner) Plan {
	return resolveWithProgress(m, env.Architecture, env.Fingerprint, run, nil, heldPackages(env.Config))
}

func heldPackages(config string) []string {
	var held []string
	for _, line := range strings.Split(config, "\n") {
		line, _, _ = strings.Cut(line, "#")
		key, value, ok := strings.Cut(line, "=")
		if ok && (strings.TrimSpace(key) == "IgnorePkg" || strings.TrimSpace(key) == "HoldPkg") {
			held = append(held, strings.Fields(value)...)
		}
	}
	return held
}
func matchesHeld(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
