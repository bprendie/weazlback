package tui

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

func (m Model) transactionPlans(repo restic.Repository, alternate string) ([]restoretxn.Plan, error) {
	if len(m.restoreBasket) == 0 {
		return nil, fmt.Errorf("restore basket is empty")
	}
	root, err := privateRestoreTarget()
	if err != nil {
		return nil, err
	}
	groups := map[string][]restoretxn.Item{}
	metadata := map[string]restoreBasketItem{}
	for path, basket := range normalizedBasket(m.restoreBasket) {
		target, targetErr := m.restoreTarget(path, alternate)
		if targetErr != nil {
			return nil, targetErr
		}
		groups[basket.Snapshot] = append(groups[basket.Snapshot], restoretxn.Item{SourcePath: path, TargetPath: target})
		metadata[basket.Snapshot] = basket
	}
	operationID := randomOperationID()
	journalRoot := filepath.Join(filepath.Dir(root), "journals")
	var plans []restoretxn.Plan
	for snapshot, items := range groups {
		sort.Slice(items, func(i, j int) bool { return items[i].SourcePath < items[j].SourcePath })
		meta := metadata[snapshot]
		plans = append(plans, restoretxn.Plan{ID: operationID + "-" + shortID(snapshot), Snapshot: snapshot,
			SourceMachineID: meta.MachineID, TargetMachineID: m.cfg.Machine.ID, Repository: repo, Items: items,
			StageRoot: filepath.Join(root, shortID(snapshot)), JournalPath: filepath.Join(journalRoot, operationID+"-"+shortID(snapshot)+".enc"),
			Conflict: m.restoreConflict, StagingOnly: m.restoreTargetMode == "staging", TargetUID: uint32(os.Getuid()), TargetGID: uint32(os.Getgid())})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Snapshot < plans[j].Snapshot })
	return plans, nil
}

func normalizedBasket(input map[string]restoreBasketItem) map[string]restoreBasketItem {
	cleaned := make(map[string]restoreBasketItem, len(input))
	paths := make([]string, 0, len(input))
	for path, item := range input {
		path = filepath.Clean(path)
		item.Path = path
		cleaned[path] = item
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) < len(paths[j]) })
	result := map[string]restoreBasketItem{}
	for _, path := range paths {
		item := cleaned[path]
		covered := false
		for parent, selected := range result {
			if selected.Snapshot == item.Snapshot && strings.HasPrefix(path, parent+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			result[path] = item
		}
	}
	return result
}

func (m Model) restoreTarget(source, alternate string) (string, error) {
	if m.restoreTargetMode == "staging" {
		return source, nil
	}
	if m.restoreTargetMode == "alternate" {
		if strings.TrimSpace(alternate) == "" {
			return "", fmt.Errorf("alternate destination is required")
		}
		return filepath.Join(alternate, filepath.Base(source)), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(source), "/"), "/")
	if len(parts) >= 3 && parts[0] == "home" {
		return filepath.Join(home, filepath.Join(parts[2:]...)), nil
	}
	return source, nil
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
