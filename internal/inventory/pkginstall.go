package inventory

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func parsePkgInstall(root string, files []string) []InstallIntent {
	var result []InstallIntent
	for _, relative := range files {
		file, err := os.Open(filepath.Join(root, relative))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for line := 1; scanner.Scan(); line++ {
			manager, packages := parseInstallLine(scanner.Text())
			if manager != "" && len(packages) > 0 {
				result = append(result, InstallIntent{Source: relative, Line: line, Manager: manager, Packages: packages})
			}
		}
		file.Close()
	}
	return result
}

func parseInstallLine(line string) (string, []string) {
	line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
	if line == "" || strings.ContainsAny(line, "|;&`$()") {
		return "", nil
	}
	fields := strings.Fields(line)
	if len(fields) > 0 && fields[0] == "sudo" {
		fields = fields[1:]
	}
	manager, start := "", -1
	switch {
	case len(fields) >= 2 && fields[0] == "pacman" && fields[1] == "-S":
		manager, start = "official", 2
	case len(fields) >= 2 && (fields[0] == "yay" || fields[0] == "paru") && fields[1] == "-S":
		manager, start = "aur", 2
	case len(fields) >= 3 && fields[0] == "flatpak" && fields[1] == "install":
		manager, start = "flatpak", 2
	case len(fields) >= 3 && fields[0] == "omarchy" && fields[1] == "pkg" && fields[2] == "add":
		manager, start = "omarchy", 3
	case len(fields) >= 4 && fields[0] == "omarchy" && fields[1] == "pkg" && fields[2] == "aur" && fields[3] == "add":
		manager, start = "omarchy-aur", 4
	}
	if start < 0 {
		return "", nil
	}
	var packages []string
	for _, value := range fields[start:] {
		if !strings.HasPrefix(value, "-") && value != "flathub" && value != "--" {
			packages = append(packages, value)
		}
	}
	return manager, packages
}
