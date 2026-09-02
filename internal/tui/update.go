package tui

import (
	"fmt"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if model, cmd, handled := m.updateRestoreMessage(message); handled {
		return model, cmd
	}
	switch msg := message.(type) {
	case restoreModeMsg:
		if m.busy {
			m.status = "Restore Mode unavailable while backup work is active"
		} else if m.vaultStage != "" {
			m.status = "unlock the vault to enter Restore Mode"
		} else {
			m.backupMode, m.backupIndex, m.backupRailFocused = m.mode, m.index, m.railFocused
			m.workspace, m.mode, m.index, m.restoreStage, m.railFocused = "restore", modeRestore, 3, "dashboard", false
			m.status = "Restore Mode"
		}
		return m, waitRestoreSignal()
	case detachDoneMsg:
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "detach failed; operation continues"
		}
		return m, nil
	case nukeDoneMsg:
		m.busy, m.nukeStage, m.nukePass = false, "", ""
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "repository nuke incomplete"
		} else {
			m.err, m.status = "", "repository cryptographically destroyed"
			if path, err := config.Path(); err == nil {
				m.cfg, _ = config.Load(path)
			}
		}
		return m, nil
	case themeTickMsg:
		m.styles = newStyles()
		return m, themeTick()
	case tuneTickMsg:
		if m.busy && (m.tuneStage == "connections" || m.tuneStage == "bandwidth") {
			m.tuneFrame++
			return m, tuneTick()
		}
		return m, nil
	case tuneConnectionMsg:
		m.tuneActiveConnection = msg.connection
		if !msg.active {
			m.tuneActiveConnection = 0
		}
		return m, waitTuneEvent(msg.events)
	case tuneConnectionsDoneMsg:
		return m.tuneConnectionsFinished(msg)
	case tuneBandwidthMsg:
		m.tuneProbeWritten, m.tuneProbeElapsed = msg.written, msg.elapsed
		return m, waitTuneEvent(msg.events)
	case tuneBandwidthDoneMsg:
		return m.tuneBandwidthFinished(msg)
	case operationProgressMsg:
		m.progress, m.status = msg.progress, progressStatus(msg.progress)
		m.publishOperationStatus("backing-up", "")
		return m, waitOperation(msg.events)
	case operationDoneMsg:
		return m.operationFinished(msg)
	case packageSudoDoneMsg:
		if msg.err != nil {
			m.packageStage, m.err, m.status = "", msg.err.Error(), "package authorization failed"
			return m, nil
		}
		return m.beginPackageCapture()
	case packageProgressMsg:
		m.packageProgress = msg.progress
		m.status = packageProgressStatus(msg.progress)
		return m, waitPackageEvent(msg.events)
	case packageDoneMsg:
		m.busy, m.cancel, m.packageStage = false, nil, ""
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "Package Capsule failed"
		} else {
			m.packageManifest, m.err = &msg.manifest, ""
			now := msg.manifest.CapturedAt
			if path, err := config.Path(); err == nil {
				latest, loadErr := config.Load(path)
				if loadErr == nil {
					latest.PackagePolicy.LastCaptured = &now
					if config.Save(path, latest) == nil {
						m.cfg = latest
					}
				}
			}
			m.status = fmt.Sprintf("Package Capsule saved — %d artifacts / %d exceptions", msg.manifest.Summary.Captured, len(msg.manifest.Exceptions))
		}
		return m, nil
	case sshProbeMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.sshFingerprint, m.sshHostLine = msg.fingerprint, msg.hostLine
		m.destinationStage, m.status = "confirm-host", "confirm pinned SSH host identity"
		return m, nil
	case sshSetupMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "SSH setup failed"
			return m, nil
		}
		m.cfg, m.destinationStage, m.destinationInputs = msg.cfg, "", nil
		m.destinationSelection = len(m.cfg.Destinations) - 1
		m.status, m.err = "SSH destination encrypted and ready", ""
		return m, nil
	case restoreBrowseMsg:
		return m.restoreBrowseFinished(msg)
	case fullRestoreDoneMsg:
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "guided recovery exited with an error"
		} else {
			m.err, m.status = "", "guided recovery closed"
		}
		return m, nil
	case applicationsMsg:
		m.busy = false
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "application inventory failed"
		} else {
			m.applications, m.err, m.status = &msg.manifest, "", "application restore plan ready"
		}
		return m, nil
	case recoveryDoneMsg:
		m.busy, m.recoveryStage, m.recoveryInputs = false, "", nil
		if msg.err != nil {
			m.err, m.status = msg.err.Error(), "recovery export failed"
		} else if msg.prepared {
			m.err, m.status = "", "recovery media prepared and verified at "+msg.path
		} else {
			m.err, m.status = "", "encrypted recovery kit written to "+msg.path
		}
		return m, nil
	case preflightDoneMsg:
		return m.preflightFinished(msg)
	case heavyInspectMsg:
		m.heavyReport = msg.report
		if msg.report.Safe {
			m.status = fmt.Sprintf("Heavy ready — %d disk images are idle", len(msg.report.Images))
		} else {
			m.status = fmt.Sprintf("Heavy blocked — %d live writers", len(msg.report.Writers))
		}
	case sudoDoneMsg:
		return m.sudoFinished(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m Model) operationFinished(msg operationDoneMsg) (tea.Model, tea.Cmd) {
	m.busy, m.cancel, m.skippedManifest = false, nil, msg.manifest
	if msg.err != nil {
		m.err, m.status = msg.err.Error(), m.operation+" failed"
		m.publishOperationStatus("failed", "operation failed; reopen Weazlback for details")
	} else if m.incomplete {
		m.err, m.status = "", fmt.Sprintf("backup incomplete — skipped %d paths", len(m.skippedPaths))
		m.publishOperationStatus("incomplete", "")
	} else {
		m.err, m.status = "", m.operation+" complete"
		m.publishOperationStatus("healthy", "")
	}
	m.snapshots = msg.snapshots
	return m, nil
}

func (m Model) restoreBrowseFinished(msg restoreBrowseMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.err, m.status = msg.err.Error(), "restore-point index failed"
		return m, nil
	}
	m.snapshots, m.restoreEntries, m.restoreSnapshot = msg.snapshots, msg.files, msg.index
	if len(msg.identities) > 0 {
		m.restoreIdentities = msg.identities
		m.restoreIdentity = identityIndex(msg.identities, restic.IdentityID(msg.snapshots[msg.index]))
	}
	m.restoreInput, m.restoreStage, m.restoreIndex = newRestoreInput(), "browse", 0
	if m.restoreTreePath == "" {
		m.restoreTreePath = restoreRoot(msg.files)
	}
	m.filterRestore()
	if m.restoreSearching {
		m.restoreInput.Focus()
	}
	if m.restoreDeletedOnly {
		m.restoreResults = deletedCatalogResults(m.restoreCatalog, m.restoreIdentities, m.restoreIdentity, msg.snapshots[msg.index].Time)
		m.restoreVisible = nil
		m.restoreSearching = true
		m.restoreInput.Prompt = "deleted since then > "
	}
	m.status, m.err = fmt.Sprintf("browsing %d paths from %s", len(msg.files), msg.snapshots[msg.index].ShortID), ""
	intent := m.restoreIntent
	m.restoreIntent = ""
	if intent == "bundle" {
		m.restoreBundleChoices = map[restoretxn.Bundle]bool{restoretxn.SystemConfig: true, restoretxn.PersonalFiles: true}
		m.restoreBundleTime, m.restoreBundleMode, m.restoreStage = msg.snapshots[msg.index].Time, "overlay", "bundle-components"
		m.status = "choose point-in-time file bundles"
	}
	if len(msg.identities) == 0 {
		return m, textinput.Blink
	}
	m.restoreCatalogState = "loading encrypted history catalog in background"
	commands := []tea.Cmd{textinput.Blink, m.loadRestoreCatalogCmd(msg.snapshots)}
	if intent == "applications" {
		m.restoreStage, m.status = "applications-loading", "loading timestamp-scoped application manifest"
		commands = append(commands, m.loadApplicationPlanCmd())
	}
	return m, tea.Batch(commands...)
}

func identityIndex(identities []restic.Identity, id string) int {
	for i := range identities {
		if identities[i].ID == id {
			return i
		}
	}
	return 0
}

func (m Model) preflightFinished(msg preflightDoneMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if m.selectedProfile == "heavy" && !msg.heavy.Safe {
		m.status = "Heavy backup refused — live writable data"
		var writers []string
		for _, writer := range msg.heavy.Writers {
			writers = append(writers, fmt.Sprintf("%s — %s pid %d", writer.Path, writer.Process, writer.PID))
			if len(writers) == 5 {
				break
			}
		}
		m.err = strings.Join(writers, "\n") + "\n\nStop the VM/container and retry."
		return m, nil
	}
	m.skippedPaths = append([]string(nil), msg.report.Paths...)
	if active := m.cfg.Active(); active != nil && active.Privileged {
		m.status = "authorizing sudo in visible terminal"
		return m, authorizeSudoCmd()
	}
	if msg.report.Unreadable == 0 {
		return m.beginEngineBackup()
	}
	m.sudoPending = true
	m.status = fmt.Sprintf("%d unreadable paths need sudo — enter authorize / s or esc skip", msg.report.Unreadable)
	m.err = strings.Join(msg.report.Samples, "\n")
	return m, nil
}

func (m Model) sudoFinished(msg sudoDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.sudoPending = false
		m.err, m.status = msg.err.Error(), "sudo authorization failed"
		return m, nil
	}
	if active := m.cfg.Active(); active != nil {
		active.Privileged = true
		if path, err := config.Path(); err == nil {
			_ = config.Save(path, m.cfg)
		}
	}
	m.sudoPending = false
	m.incomplete, m.skippedPaths = false, nil
	return m.beginEngineBackup()
}
