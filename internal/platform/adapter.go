package platform

import "path/filepath"

type Adapter interface {
	Identity() Identity
	CoreClaims(home string) []Claim
}

type staticAdapter struct{ identity Identity }

func For(identity Identity) Adapter        { return staticAdapter{identity: identity} }
func (a staticAdapter) Identity() Identity { return a.identity }

func (a staticAdapter) CoreClaims(home string) []Claim {
	paths := []string{".config/systemd/user", ".local/share/systemd/user"}
	switch a.identity.Desktop {
	case "omarchy-shell", "hyprland":
		paths = append(paths, ".config/omarchy", ".config/hypr", ".config/quickshell", ".config/uwsm", ".local/state/omarchy")
	case "gnome":
		paths = append(paths, ".config/dconf", ".config/gnome-session", ".config/gtk-3.0", ".config/gtk-4.0",
			".config/monitors.xml", ".local/share/gnome-shell", ".local/share/gvfs-metadata")
	case "plasma":
		paths = append(paths, ".config/kdeglobals", ".config/kglobalshortcutsrc", ".config/kwinrc",
			".config/kwinoutputconfig.json", ".config/kscreenlockerrc", ".config/plasmarc",
			".config/plasma-localerc", ".config/plasma-org.kde.plasma.desktop-appletsrc",
			".config/powermanagementprofilesrc", ".local/share/kscreen", ".local/share/plasma")
	}
	if a.identity.ID == "omarchy" {
		paths = append(paths, ".local/share/omarchy", ".config/omarchy/plugins")
	}
	if a.identity.ID == "ubuntu" {
		paths = append(paths, ".config/ubuntu-report", ".local/share/ubuntu-report")
	}
	if a.identity.PackageFamily == "pacman" {
		paths = append(paths, ".cache/paru", ".cache/yay")
	}
	claims := make([]Claim, 0, len(paths)+1)
	for _, path := range paths {
		claims = append(claims, Claim{Path: filepath.Join(home, path), Owner: "desktop", Domain: a.identity.Desktop})
	}
	if a.identity.Desktop == "gnome" {
		claims = append(claims, Claim{Resource: "dconf:/org/gnome/", Owner: "desktop", Domain: "gnome"})
	}
	return claims
}
