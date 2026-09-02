package freshrestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/browserrepair"
)

type browserDiagnostic struct {
	SourceHostname string                `json:"source_hostname"`
	TargetHostname string                `json:"target_hostname"`
	Entries        []browserrepair.Entry `json:"entries"`
	Result         browserrepair.Result  `json:"result"`
}

func (r *Restore) repairBrowserCompatibility() (browserrepair.Result, []string) {
	if !r.browserRepairEligible() {
		return browserrepair.Result{}, nil
	}
	emitProgress(r.Options.Progress, RestoreProgress{Phase: "browser compatibility", Lane: "Browser compatibility", Current: "validating transient locks"})
	processes := r.Options.BrowserProcesses
	if processes == nil {
		processes = browserrepair.ProcFS{}
	}
	options := browserrepair.Options{Home: r.Plan.TargetHome, UID: os.Getuid(), Processes: processes}
	plan := browserrepair.Detect(options)
	result := browserrepair.Apply(options, plan)
	issues := browserIssues(result)
	if err := r.saveBrowserDiagnostic(plan, result); err != nil {
		result.Failed++
		issues = append(issues, "browser compatibility: encrypted diagnostic could not be saved")
	}
	emitProgress(r.Options.Progress, RestoreProgress{Phase: "browser compatibility", Lane: "Browser compatibility", Current: "complete", Completed: result.Removed, Total: result.Removed + result.Live + result.Failed})
	return result, issues
}

func (r *Restore) browserRepairEligible() bool {
	eligibleScope := r.Plan.Scope == "core" || r.Plan.Scope == "home" || r.Plan.Scope == "everything"
	return eligibleScope && r.Plan.SourceHostname != "" && r.Plan.SourceHostname != r.Plan.Hostname
}

func browserIssues(result browserrepair.Result) []string {
	var issues []string
	if result.Live > 0 {
		issues = append(issues, fmt.Sprintf("browser compatibility: %d transient locks skipped because a browser is running", result.Live))
	}
	if result.Boundary > 0 {
		issues = append(issues, fmt.Sprintf("browser compatibility: %d unsafe profile roots skipped", result.Boundary))
	}
	if result.Failed > 0 {
		issues = append(issues, fmt.Sprintf("browser compatibility: %d transient locks could not be removed", result.Failed))
	}
	return issues
}

func (r *Restore) saveBrowserDiagnostic(plan browserrepair.Plan, result browserrepair.Result) error {
	if r.Session == nil || r.Session.Vault == nil {
		return nil
	}
	plain, err := json.Marshal(browserDiagnostic{SourceHostname: r.Plan.SourceHostname, TargetHostname: r.Plan.Hostname, Entries: plan.Entries, Result: result})
	if err != nil {
		return err
	}
	ciphertext, err := r.Session.Vault.Encrypt(plain)
	if err != nil {
		return err
	}
	path := r.JournalPath + "-browser.enc"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
		return err
	}
	r.Journal.BrowserJournal = path
	return nil
}
