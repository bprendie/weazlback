package packagecapsule

import (
	"bufio"
	"fmt"
	"runtime"
	"strings"
)

type artifactMetadata struct {
	Name, Version, Architecture, BuildDate, Packager string
	Depends, Provides, Conflicts                     []string
	Raw                                              string
}

func inspectArtifact(run Runner, path string) (artifactMetadata, string, error) {
	pkginfo, err := run.Output("bsdtar", "-xOf", path, ".PKGINFO")
	if err != nil {
		return artifactMetadata{}, "", fmt.Errorf("read .PKGINFO: %w", err)
	}
	buildinfo, _ := run.Output("bsdtar", "-xOf", path, ".BUILDINFO")
	meta := parsePackageInfo(pkginfo)
	meta.Raw = pkginfo
	if meta.Name == "" || meta.Version == "" || meta.Architecture == "" {
		return meta, buildinfo, fmt.Errorf("incomplete .PKGINFO")
	}
	return meta, buildinfo, nil
}

func parsePackageInfo(value string) artifactMetadata {
	var result artifactMetadata
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		key, item, ok := strings.Cut(scanner.Text(), " = ")
		if !ok {
			continue
		}
		switch key {
		case "pkgname":
			result.Name = item
		case "pkgver":
			result.Version = item
		case "arch":
			result.Architecture = item
		case "builddate":
			result.BuildDate = item
		case "packager":
			result.Packager = item
		case "depend":
			result.Depends = append(result.Depends, item)
		case "provides":
			result.Provides = append(result.Provides, item)
		case "conflict":
			result.Conflicts = append(result.Conflicts, item)
		}
	}
	return result
}

func compatible(meta artifactMetadata, buildinfo string) (bool, string) {
	wanted := runtime.GOARCH
	if wanted == "amd64" {
		wanted = "x86_64"
	}
	if meta.Architecture != "any" && meta.Architecture != wanted {
		return false, "artifact architecture " + meta.Architecture + " does not match " + wanted
	}
	if strings.Contains(strings.ToLower(buildinfo), "-march=native") {
		return false, "artifact BUILDINFO contains -march=native"
	}
	return true, ""
}
