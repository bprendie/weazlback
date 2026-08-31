package tui

func navigationDescription(current mode) string {
	descriptions := map[mode]string{
		modeHome:         "Backup health, repository state, recent warnings, and active work.",
		modeBackup:       "Create an encrypted Core, Home, or Heavy restore point.",
		modeSnapshots:    "Immutable Restic manifests of paths, metadata, and deduplicated chunks.\nThese are not filesystem or block-device snapshots.",
		modeRestore:      "Recover selected files and folders safely, or begin full-system recovery.",
		modeProfiles:     "Review Core/Home/Heavy policy and the application restore manifest.",
		modeDestinations: "Manage encrypted SSH and local repositories and pinned host identity.",
		modeRecovery:     "Export or verify the password-locked kit needed on a fresh machine.",
		modeCheck:        "Verify encrypted repository indexes; full-data checks read every pack.",
		modeTune:         "Measure repository concurrency and SSH bandwidth, then choose both limits.",
		modeSchedule:     "Automatic backup timing, retention, power, and network policy (Phase 5).",
		modeNuke:         "Break-glass cryptographic destruction and repository deletion.",
	}
	return descriptions[current]
}
