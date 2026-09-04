package freshrestore

import "fmt"

func PlanText(plan Plan) string {
	generationText := ""
	if plan.GenerationID != "" {
		generationText = "\nSystem set     " + plan.GenerationID + "  COMPLETE"
	}
	points := fmt.Sprintf("Core           %s", plan.Snapshot.Time.Local().Format("2006-01-02 15:04"))
	if plan.HomeSnapshot != nil {
		points += fmt.Sprintf("\nHome           %s", plan.HomeSnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	if plan.HeavySnapshot != nil {
		points += fmt.Sprintf("\nHeavy          %s", plan.HeavySnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	if plan.PackageSnapshot != nil {
		points += fmt.Sprintf("\nPackages       %s", plan.PackageSnapshot.Time.Local().Format("2006-01-02 15:04"))
	}
	identity := "preserve target identity"
	if plan.AdoptSourceIdentity {
		identity = "ADOPT source identity " + plan.SourceMachineID
	}
	warning := ""
	if plan.ScopeDecision.Warning != "" {
		warning = "\n\nWARNING\n" + plan.ScopeDecision.Warning
	}
	return fmt.Sprintf("Recovery scope %s\nIncludes       %s%s\n%s\nHostname       %s\nMachine        %s\nHome mapping   %s -> %s\nOwnership      %d:%d -> %d:%d\nPackage delta  %d local / %d official online / %d foreign online / %d kept\nFlatpaks       %d\nServices       %d system / %d user%s",
		plan.Scope, recoveryScopeContents(plan.Scope), generationText, points, plan.Hostname, identity, plan.OriginalHome, plan.TargetHome, plan.SourceUID, plan.SourceGID, plan.TargetUID, plan.TargetGID,
		len(plan.PackageDelta.Local), len(plan.Official), len(plan.AUR), len(plan.PackageDelta.Kept), len(plan.Flatpak), len(plan.SystemServices), len(plan.UserServices), warning)
}

func recoveryScopeContents(scope string) string {
	switch scope {
	case "everything":
		return "Applications (parallel) + Core + Home + Heavy"
	case "home", "core-home":
		return "Applications (parallel) + Core + Home"
	case "applications":
		return "Applications only"
	default:
		return "Applications (parallel) + Core"
	}
}
