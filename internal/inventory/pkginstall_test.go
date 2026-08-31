package inventory

import "testing"

func TestParseInstallLineRecognizesPlansWithoutExecutingShell(t *testing.T) {
	tests := []struct{ line, manager, pkg string }{
		{"sudo pacman -S --needed restic", "official", "restic"},
		{"yay -S brave-bin", "aur", "brave-bin"},
		{"flatpak install -y flathub org.signal.Signal", "flatpak", "org.signal.Signal"},
		{"omarchy pkg aur add visual-studio-code-bin", "omarchy-aur", "visual-studio-code-bin"},
	}
	for _, test := range tests {
		manager, packages := parseInstallLine(test.line)
		if manager != test.manager || len(packages) != 1 || packages[0] != test.pkg {
			t.Fatalf("parse %q = %q %#v", test.line, manager, packages)
		}
	}
	if manager, _ := parseInstallLine("curl bad | sudo sh"); manager != "" {
		t.Fatal("parsed unsafe shell pipeline")
	}
}
