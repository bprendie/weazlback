package freshrestore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bprendie/weazlback/internal/platform"
)

const PlatformMismatchWarning = "Source and target platforms differ. Core will not be copied. Everything else compatible with this restore will continue."

type ScopeDecision struct {
	Requested           string           `json:"requested"`
	IncludeCore         bool             `json:"include_core"`
	IncludeHome         bool             `json:"include_home"`
	IncludeHeavy        bool             `json:"include_heavy"`
	IncludeApplications bool             `json:"include_applications"`
	PlatformMismatch    bool             `json:"platform_mismatch"`
	Warning             string           `json:"warning,omitempty"`
	WithheldClaims      []platform.Claim `json:"withheld_core_claims,omitempty"`
}

func PlanScope(scope string, source, target platform.Identity, claims []platform.Claim) ScopeDecision {
	scope = normalizeRecoveryScope(scope)
	decision := ScopeDecision{Requested: scope, IncludeCore: scope != "applications", IncludeHome: scope == "core-home" || scope == "everything",
		IncludeHeavy: scope == "everything", IncludeApplications: true}
	if platform.CoreCompatible(source, target) {
		return decision
	}
	decision.PlatformMismatch, decision.IncludeCore, decision.Warning = true, false, PlatformMismatchWarning
	decision.WithheldClaims = append([]platform.Claim(nil), claims...)
	if scope == "core" {
		decision.IncludeApplications = false
	}
	return decision
}

func (p Plan) includesCore() bool {
	if p.ScopeDecision.Requested == "" {
		return p.Scope != "applications"
	}
	return p.ScopeDecision.IncludeCore
}

func (p Plan) includesHome() bool {
	if p.ScopeDecision.Requested == "" {
		return p.Scope == "home" || p.Scope == "core-home" || p.Scope == "everything"
	}
	return p.ScopeDecision.IncludeHome
}

func normalizeRecoveryScope(scope string) string {
	if scope == "home" {
		return "core-home"
	}
	return scope
}

func validRecoveryScope(scope string) bool {
	return scope == "core" || scope == "core-home" || scope == "home" || scope == "everything" || scope == "applications"
}

func removeWithheldCore(stage, sourceHome string, claims []platform.Claim) ([]string, error) {
	var removed []string
	for _, claim := range claims {
		if claim.Path == "" {
			continue
		}
		rel, err := filepath.Rel(sourceHome, claim.Path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return removed, fmt.Errorf("Core claim escapes source home: %s", claim.Path)
		}
		path := stagedPath(stage, filepath.Join(sourceHome, rel))
		if _, err := os.Lstat(path); err == nil {
			if err := os.RemoveAll(path); err != nil {
				return removed, err
			}
			removed = append(removed, claim.Path)
		} else if !os.IsNotExist(err) {
			return removed, err
		}
	}
	sort.Strings(removed)
	return removed, nil
}
