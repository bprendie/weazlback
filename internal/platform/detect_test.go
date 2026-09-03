package platform

import "testing"

func TestDetectSupportedPlatformAndDesktopFixtures(t *testing.T) {
	tests := []struct {
		name                      string
		facts                     Facts
		family, packages, desktop string
	}{
		{"omarchy", Facts{OSRelease: map[string]string{"ID": "arch"}, Omarchy: true, Architecture: "amd64"}, "arch", "pacman", "omarchy-shell"},
		{"ubuntu", Facts{OSRelease: map[string]string{"ID": "ubuntu", "ID_LIKE": "debian"}, Desktop: "ubuntu:GNOME"}, "debian", "apt", "gnome"},
		{"kubuntu", Facts{OSRelease: map[string]string{"ID": "ubuntu", "VARIANT_ID": "kubuntu"}, Desktop: "KDE"}, "debian", "apt", "plasma"},
		{"fedora-gnome", Facts{OSRelease: map[string]string{"ID": "fedora"}, Desktop: "GNOME", SELinux: true}, "fedora", "dnf", "gnome"},
		{"fedora-kde", Facts{OSRelease: map[string]string{"ID": "fedora"}, Desktop: "KDE"}, "fedora", "dnf", "plasma"},
		{"rhel", Facts{OSRelease: map[string]string{"ID": "rhel", "ID_LIKE": "fedora"}}, "rhel", "dnf", ""},
		{"unknown", Facts{OSRelease: map[string]string{"ID": "weird"}}, "unknown", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Detect(test.facts)
			if got.Family != test.family || got.PackageFamily != test.packages || got.Desktop != test.desktop {
				t.Fatalf("identity=%+v", got)
			}
		})
	}
}

func TestCoreCompatibilityRequiresPlatformAndDesktop(t *testing.T) {
	ubuntu := Identity{Family: "debian", Desktop: "gnome"}
	if !CoreCompatible(ubuntu, ubuntu) {
		t.Fatal("same platform rejected")
	}
	if CoreCompatible(ubuntu, Identity{Family: "fedora", Desktop: "gnome"}) {
		t.Fatal("cross-platform Core accepted")
	}
	if CoreCompatible(ubuntu, Identity{Family: "debian", Desktop: "plasma"}) {
		t.Fatal("cross-desktop Core accepted")
	}
}
