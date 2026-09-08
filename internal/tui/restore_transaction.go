package tui

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/browserrepair"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/bprendie/weazlback/internal/securelog"
	tea "github.com/charmbracelet/bubbletea"
)

type restoreTransactionPreflightMsg struct {
	plans   []restoretxn.Plan
	reports []restoretxn.Preflight
	deletes []string
	err     error
}

type restoreTransactionProgressMsg struct {
	progress restoretxn.Progress
	events   <-chan tea.Msg
}

type restoreTransactionDoneMsg struct {
	result restoretxn.Result
	err    error
}
type restoreRollbackDoneMsg struct{ err error }

func (m Model) startRestoreTransactionPreflight(alternate string) (tea.Model, tea.Cmd) {
	_, _, repo, err := m.activeRuntime("")
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if repo.Elevated && !m.restoreAuthorized {
		m.status = "Authorize protected repository access with your system password"
		return m, authorizeRestorePreflight(alternate)
	}
	plans, err := m.transactionPlans(repo, alternate)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.busy, m.operation, m.status, m.err = true, "restore preflight", "checking repository, destination, ownership, and space", ""
	bundleMode := m.restoreBundleMode
	return m, func() tea.Msg {
		service := restic.NewService(io.Discard)
		reports := make([]restoretxn.Preflight, 0, len(plans))
		for i := range plans {
			report, preflightErr := restoretxn.PreflightPlan(context.Background(), service, &plans[i])
			if preflightErr != nil {
				return restoreTransactionPreflightMsg{err: preflightErr}
			}
			reports = append(reports, report)
		}
		var deletes []string
		if bundleMode == "exact" {
			for _, plan := range plans {
				files, filesErr := service.Files(context.Background(), repo, plan.Snapshot)
				if filesErr != nil {
					return restoreTransactionPreflightMsg{err: filesErr}
				}
				paths := make([]string, len(files))
				for i := range files {
					paths[i] = files[i].Path
				}
				for _, item := range plan.Items {
					removed, deleteErr := restoretxn.ExactDeletions(item.TargetPath, item.SourcePath, item.TargetPath, paths)
					if deleteErr != nil && !os.IsNotExist(deleteErr) {
						return restoreTransactionPreflightMsg{err: deleteErr}
					}
					deletes = append(deletes, removed...)
				}
			}
		}
		return restoreTransactionPreflightMsg{plans: plans, reports: reports, deletes: deletes}
	}
}

func privateRestoreTarget() (string, error) {
	path, err := config.Path()
	if err != nil {
		return "", err
	}
	target := filepath.Join(filepath.Dir(path), "restores", time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	return target, nil
}

func randomOperationID() string {
	value := make([]byte, 6)
	_, _ = rand.Read(value)
	return fmt.Sprintf("restore-%d-%x", time.Now().Unix(), value)
}

func (m Model) runRestoreTransaction() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.busy, m.operation, m.status, m.err = cancel, true, "selective restore", "starting encrypted restore transaction", ""
	plans, vaultFile := append([]restoretxn.Plan(nil), m.restorePlans...), m.vault
	bundleMode, deletions := m.restoreBundleMode, append([]string(nil), m.restoreBundleDeletes...)
	bundleParts := append([]restoretxn.Component(nil), m.restoreBundleParts...)
	targetMode := m.restoreTargetMode
	bundleJournal := ""
	if len(bundleParts) > 0 && len(plans) > 0 {
		bundleJournal = filepath.Join(filepath.Dir(plans[0].JournalPath), strings.TrimSuffix(filepath.Base(plans[0].JournalPath), ".enc")+"-bundle.enc")
		m.restoreBundleJournal = bundleJournal
	}
	events := make(chan tea.Msg, 1)
	go func() {
		var combined restoretxn.Result
		engine := restoretxn.Engine{Service: restic.NewService(io.Discard), Cryptor: vaultFile}
		if bundleJournal != "" {
			if err := restoretxn.SaveBundleJournal(bundleJournal, vaultFile, restoretxn.BundleJournal{OperationID: plans[0].ID, Mode: bundleMode, State: "running", Components: bundleParts, Deletions: deletions}); err != nil {
				sendLatestOperationEvent(events, restoreTransactionDoneMsg{err: err})
				close(events)
				return
			}
		}
		for index, plan := range plans {
			result, err := engine.Run(ctx, plan, func(progress restoretxn.Progress) {
				sendLatestOperationEvent(events, restoreTransactionProgressMsg{progress: progress, events: events})
			})
			combined.JournalPath = result.JournalPath
			combined.StagedAt = result.StagedAt
			combined.Placed = append(combined.Placed, result.Placed...)
			combined.Rollback = append(combined.Rollback, result.Rollback...)
			combined.Skipped = append(combined.Skipped, result.Skipped...)
			if err != nil {
				if bundleJournal != "" {
					_ = restoretxn.SaveBundleJournal(bundleJournal, vaultFile, restoretxn.BundleJournal{OperationID: plans[0].ID, Mode: bundleMode, State: "incomplete", Completed: index, Components: bundleParts, Deletions: deletions})
				}
				sendLatestOperationEvent(events, restoreTransactionDoneMsg{result: combined, err: err})
				close(events)
				return
			}
			if bundleMode == "exact" {
				var selected []string
				for _, path := range deletions {
					for _, item := range plan.Items {
						if path == item.TargetPath || strings.HasPrefix(path, item.TargetPath+string(filepath.Separator)) {
							selected = append(selected, path)
							break
						}
					}
				}
				if err := engine.ApplyExactDeletions(plan, selected); err != nil {
					sendLatestOperationEvent(events, restoreTransactionDoneMsg{result: combined, err: err})
					close(events)
					return
				}
			}
			if bundleJournal != "" {
				_ = restoretxn.SaveBundleJournal(bundleJournal, vaultFile, restoretxn.BundleJournal{OperationID: plans[0].ID, Mode: bundleMode, State: "running", Completed: index + 1, Components: bundleParts, Deletions: deletions})
			}
		}
		if len(bundleParts) > 0 && targetMode == "original" {
			browserPlan, browserResult := repairInstalledBundleBrowsers(bundleParts)
			combined.BrowserRepair = browserResult
			payload, _ := json.Marshal(struct {
				Entries []browserrepair.Entry `json:"entries"`
				Result  browserrepair.Result  `json:"result"`
			}{browserPlan.Entries, browserResult})
			_, _ = securelog.Write(vaultFile, "browser-compatibility", plans[0].ID, payload)
			if browserResult.Live+browserResult.Boundary+browserResult.Failed > 0 {
				if bundleJournal != "" {
					_ = restoretxn.SaveBundleJournal(bundleJournal, vaultFile, restoretxn.BundleJournal{OperationID: plans[0].ID, Mode: bundleMode, State: "incomplete", Completed: len(plans), Components: bundleParts, Deletions: deletions})
				}
				sendLatestOperationEvent(events, restoreTransactionDoneMsg{result: combined, err: fmt.Errorf("browser compatibility incomplete: %d live, %d boundary, %d failed", browserResult.Live, browserResult.Boundary, browserResult.Failed)})
				close(events)
				return
			}
		}
		if bundleJournal != "" {
			_ = restoretxn.SaveBundleJournal(bundleJournal, vaultFile, restoretxn.BundleJournal{OperationID: plans[0].ID, Mode: bundleMode, State: "complete", Completed: len(plans), Components: bundleParts, Deletions: deletions})
		}
		payload, _ := json.Marshal(struct {
			Result restoretxn.Result `json:"result"`
		}{Result: combined})
		_, _ = securelog.Write(vaultFile, "restore", plans[0].ID, payload)
		sendLatestOperationEvent(events, restoreTransactionDoneMsg{result: combined})
		close(events)
	}()
	return m, waitRestoreTransaction(events)
}

func waitRestoreTransaction(events <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-events } }

func (m Model) rollbackRestoreTransaction() (tea.Model, tea.Cmd) {
	m.busy, m.status, m.err = true, "rolling back placed paths", ""
	plans, vaultFile := append([]restoretxn.Plan(nil), m.restorePlans...), m.vault
	return m, func() tea.Msg {
		engine := restoretxn.Engine{Service: restic.NewService(io.Discard), Cryptor: vaultFile}
		for i := len(plans) - 1; i >= 0; i-- {
			if err := engine.Rollback(plans[i]); err != nil {
				return restoreRollbackDoneMsg{err}
			}
		}
		return restoreRollbackDoneMsg{}
	}
}
