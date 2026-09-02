package packagecapsule

import "sort"

type Install struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Source   string `json:"source"`
	Artifact string `json:"artifact,omitempty"`
}

type Decision struct {
	Name, TargetVersion, DesiredVersion, Action, Reason string
}

type Delta struct {
	Local          []Install
	OfficialOnline []string
	ForeignOnline  []string
	Kept           []Decision
	Fallback       []Decision
}

type CompareVersion func(left, right string) int

func ResolveDelta(manifest Manifest, installed map[string]string, compare CompareVersion) Delta {
	var result Delta
	for _, pkg := range manifest.Packages {
		desired := pkg.Installed
		if pkg.ArtifactVersion != "" && compare(pkg.ArtifactVersion, desired) > 0 {
			desired = pkg.ArtifactVersion
		}
		target := installed[pkg.Name]
		if target != "" && compare(target, desired) >= 0 {
			result.Kept = append(result.Kept, Decision{Name: pkg.Name, TargetVersion: target, DesiredVersion: desired,
				Action: "keep", Reason: "target version is equal or newer"})
			continue
		}
		if pkg.Compatible && pkg.Artifact != "" && pkg.ArtifactVersion != "" && compare(pkg.ArtifactVersion, desired) >= 0 {
			result.Local = append(result.Local, Install{Name: pkg.Name, Version: pkg.ArtifactVersion, Source: pkg.Source, Artifact: pkg.Artifact})
			continue
		}
		decision := Decision{Name: pkg.Name, TargetVersion: target, DesiredVersion: desired, Action: "online",
			Reason: "compatible selected artifact is unavailable"}
		result.Fallback = append(result.Fallback, decision)
		if pkg.Source == "official" {
			result.OfficialOnline = append(result.OfficialOnline, pkg.Name)
		} else {
			result.ForeignOnline = append(result.ForeignOnline, pkg.Name)
		}
	}
	sort.Slice(result.Local, func(i, j int) bool { return result.Local[i].Name < result.Local[j].Name })
	sort.Strings(result.OfficialOnline)
	sort.Strings(result.ForeignOnline)
	sort.Slice(result.Kept, func(i, j int) bool { return result.Kept[i].Name < result.Kept[j].Name })
	sort.Slice(result.Fallback, func(i, j int) bool { return result.Fallback[i].Name < result.Fallback[j].Name })
	return result
}
