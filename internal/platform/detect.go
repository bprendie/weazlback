package platform

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

func Current() Identity {
	facts := Facts{OSRelease: readOSRelease("/etc/os-release"), Architecture: runtime.GOARCH,
		Desktop: firstEnvironment("XDG_CURRENT_DESKTOP", "XDG_SESSION_DESKTOP"), Session: os.Getenv("XDG_SESSION_TYPE")}
	_, err := os.Stat("/usr/bin/omarchy")
	facts.Omarchy = err == nil
	_, err = os.Stat("/sys/fs/selinux")
	facts.SELinux = err == nil
	_, err = os.Stat("/sys/module/apparmor")
	facts.AppArmor = err == nil
	return Detect(facts)
}

func Detect(f Facts) Identity {
	id := strings.ToLower(f.OSRelease["ID"])
	like := strings.Fields(strings.ToLower(f.OSRelease["ID_LIKE"]))
	result := Identity{SchemaVersion: IdentitySchemaVersion, ID: id, Variant: strings.ToLower(f.OSRelease["VARIANT_ID"]),
		Version: f.OSRelease["VERSION_ID"], Architecture: f.Architecture, Desktop: desktopDomain(f.Desktop),
		Session: strings.ToLower(f.Session)}
	switch {
	case f.Omarchy || id == "omarchy":
		result.ID, result.Family, result.Variant, result.PackageFamily, result.Desktop = "omarchy", "arch", "omarchy", "pacman", "omarchy-shell"
	case id == "arch" || contains(like, "arch"):
		result.Family, result.PackageFamily = "arch", "pacman"
	case id == "ubuntu" || id == "debian" || contains(like, "debian") || contains(like, "ubuntu"):
		result.Family, result.PackageFamily = "debian", "apt"
	case id == "rhel" || contains(like, "rhel") || contains(like, "centos"):
		result.Family, result.PackageFamily = "rhel", "dnf"
	case id == "fedora" || contains(like, "fedora"):
		result.Family, result.PackageFamily = "fedora", "dnf"
	default:
		result.Family = "unknown"
	}
	if f.SELinux {
		result.Security = "selinux"
	} else if f.AppArmor {
		result.Security = "apparmor"
	}
	return result
}

func CoreCompatible(source, target Identity) bool {
	if !source.Known() || !target.Known() || source.Family != target.Family {
		return false
	}
	return source.Desktop == "" || target.Desktop == "" || source.Desktop == target.Desktop
}

func readOSRelease(path string) map[string]string {
	result := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			result[key] = strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return result
}

func firstEnvironment(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func desktopDomain(value string) string {
	value = strings.ToLower(value)
	switch {
	case strings.Contains(value, "kde") || strings.Contains(value, "plasma"):
		return "plasma"
	case strings.Contains(value, "gnome") || strings.Contains(value, "ubuntu"):
		return "gnome"
	case strings.Contains(value, "hyprland"):
		return "hyprland"
	}
	return ""
}
