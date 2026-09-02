package tui

import (
	"os"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/restoretxn"
)

func repairInstalledBundleBrowsers(parts []restoretxn.Component) (browserrepair.Plan, browserrepair.Result) {
	target, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	return repairInstalledBundleBrowsersWithOptions(parts, target, browserrepair.Options{Home: home, UID: os.Getuid(), Processes: browserrepair.ProcFS{}})
}

func repairInstalledBundleBrowsersWithOptions(parts []restoretxn.Component, target string, options browserrepair.Options) (browserrepair.Plan, browserrepair.Result) {
	eligible := false
	for _, part := range parts {
		if (part.Bundle == restoretxn.SystemConfig || part.Bundle == restoretxn.PersonalFiles) && part.Snapshot.Hostname != "" && part.Snapshot.Hostname != target {
			eligible = true
		}
	}
	if !eligible {
		return browserrepair.Plan{}, browserrepair.Result{}
	}
	plan := browserrepair.Detect(options)
	return plan, browserrepair.Apply(options, plan)
}
