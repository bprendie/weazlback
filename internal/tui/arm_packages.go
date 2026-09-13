package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/architecture"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/targetpackages"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type armSudoDoneMsg struct{ err error }
type armProgressMsg struct {
	lane     string
	progress systemSnapshotLane
	events   <-chan tea.Msg
}
type armDoneMsg struct {
	manifest packagecapsule.Manifest
	err      error
}

func (m Model) architecturePackageUI() (tea.Model, tea.Cmd) {
	if m.busy {
		m.status = "ARM64 packages unavailable while another operation is running; wait for it to finish"
		return m, nil
	}
	m.err = ""
	if m.cfg.ARMTargetEnvironment == "" {
		return m.setupARMTarget()
	}
	if err := validateARMTarget(m.cfg.ARMTargetEnvironment); err != nil {
		next, cmd := m.setupARMTarget()
		updated := next.(Model)
		updated.err = err.Error()
		return updated, cmd
	}
	return m.authorizeARMCapture()
}

func validateARMTarget(root string) error {
	env, err := targetpackages.LoadEnvironment(root)
	if err != nil {
		return err
	}
	if architecture.Go(env.Architecture) != "arm64" {
		return fmt.Errorf("these settings are for %s; export them on the Snapdragon PC", env.Architecture)
	}
	for _, name := range []string{"keyring/pubring.gpg", "keyring/trustdb.gpg"} {
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target verification file must be regular: %s", name)
		}
	}
	return nil
}

func (m Model) setupARMTarget() (tea.Model, tea.Cmd) {
	m.armStage = "setup"
	m.armInput = textinput.New()
	m.armInput.Prompt = "Snapdragon settings folder > "
	m.armInput.Placeholder = "/mnt/snapdragon-target"
	m.armInput.Focus()
	return m, textinput.Blink
}

// Keep a private copy, so preparation does not depend on a mounted transfer drive.
func rememberARMTarget(source string, cfg *config.Config) error {
	source = strings.TrimSpace(source)
	if source == "~" || strings.HasPrefix(source, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		source = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(source, "~"), "/"))
	}
	if err := validateARMTarget(source); err != nil {
		return err
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	parent := filepath.Join(filepath.Dir(path), "architecture-targets")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	root, err := os.MkdirTemp(parent, "arm64-")
	if err != nil {
		return err
	}
	saved := false
	defer func() {
		if !saved {
			_ = os.RemoveAll(root)
		}
	}()
	if err := os.Mkdir(filepath.Join(root, "keyring"), 0700); err != nil {
		return err
	}
	for _, name := range []string{"target.json", "keyring/pubring.gpg", "keyring/trustdb.gpg"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			return err
		}
	}
	if err := validateARMTarget(root); err != nil {
		return err
	}
	next := *cfg
	next.ARMTargetEnvironment = root
	if err := config.Save(path, next); err != nil {
		return err
	}
	*cfg = next
	saved = true
	return nil
}

func (m Model) updateARMKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	if msg.String() == "esc" {
		m.armStage, m.err = "", ""
		return m, nil
	}
	if m.armStage == "setup" {
		if msg.String() == "enter" {
			if err := rememberARMTarget(m.armInput.Value(), &m.cfg); err != nil {
				m.err = err.Error()
				return m, nil
			}
			return m.authorizeARMCapture()
		}
		var cmd tea.Cmd
		m.armInput, cmd = m.armInput.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "enter", "X":
		return m.architecturePackageUI()
	case "T":
		return m.setupARMTarget()
	}
	return m, nil
}

func (m Model) authorizeARMCapture() (tea.Model, tea.Cmd) {
	if _, _, _, err := m.activeRuntime(""); err != nil {
		m.err = err.Error()
		m.armStage = "done"
		return m, nil
	}
	m.busy, m.armStage, m.operation = true, "authorizing", "ARM64 packages"
	m.armManifest = nil
	m.armStart = time.Now()
	m.armLanes = map[string]systemSnapshotLane{
		"packages":   {State: "waiting", Current: "authorize target package downloads"},
		"aur":        {State: "review", Current: "unavailable applications require native ARM builds or replacements"},
		"repository": {State: "waiting", Current: "packages-arm64 in default repository"},
	}
	m.status, m.err = "authorize ARM64 package preparation", ""
	return m, tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg { return armSudoDoneMsg{err} })
}

func (m Model) beginARMCapture() (tea.Model, tea.Cmd) {
	destination, _, repo, err := m.activeRuntime("")
	if err != nil {
		return m.finishARMCapture(armDoneMsg{err: err})
	}
	// The visible sudo handshake also covers repositories written by System Set.
	repo.Elevated = true
	stage, err := packageTUIStagingRoot()
	if err != nil {
		return m.finishARMCapture(armDoneMsg{err: err})
	}
	m.armStage, m.busy, m.operation = "running", true, "ARM64 packages"
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	events := make(chan tea.Msg, 32)
	cfg := m.cfg
	go func() {
		defer close(events)
		emit := func(lane string, p systemSnapshotLane) {
			select {
			case events <- armProgressMsg{lane, p, events}:
			case <-ctx.Done():
			}
		}
		manifest, err := runARMCapture(ctx, cfg, stage, repo, func() error { return verifyTUIRepositoryIdentity(ctx, &cfg, destination, repo) }, emit)
		if err == nil {
			_ = catalog.Refresh(ctx, m.vault, destination.ID, repo, cfg.Machine.ID, "packages-arm64")
		}
		events <- armDoneMsg{manifest, err}
	}()
	return m, waitARMEvent(events)
}

type armCaptureOperations struct {
	inventory func(context.Context) (inventory.ApplicationManifest, error)
	prepare   func(context.Context, string, string, string, inventory.ApplicationManifest, bool, bool, func(packagecapsule.Progress)) (packagecapsule.Manifest, string, func(), error)
	archive   func(context.Context, restic.Repository, string, string, []string, func(restic.BackupProgress)) error
}

func runARMCapture(ctx context.Context, cfg config.Config, stage string, repo restic.Repository, verify func() error, emit func(string, systemSnapshotLane)) (packagecapsule.Manifest, error) {
	return captureARMWith(ctx, cfg, stage, repo, verify, emit, armCaptureOperations{
		inventory: inventory.CaptureApplications,
		prepare:   targetpackages.Prepare,
		archive: func(ctx context.Context, repo restic.Repository, profile, machine string, paths []string, progress func(restic.BackupProgress)) error {
			return restic.NewService(io.Discard).BackupMachineWithProgress(ctx, repo, profile, machine, paths, nil, false, false, progress)
		},
	})
}

func captureARMWith(ctx context.Context, cfg config.Config, stage string, repo restic.Repository, verify func() error, emit func(string, systemSnapshotLane), ops armCaptureOperations) (packagecapsule.Manifest, error) {
	if err := validateARMTarget(cfg.ARMTargetEnvironment); err != nil {
		return packagecapsule.Manifest{}, err
	}
	if err := verify(); err != nil {
		return packagecapsule.Manifest{}, err
	}
	emit("packages", systemSnapshotLane{State: "inventory", Current: "reading this machine's installed applications"})
	source, err := ops.inventory(ctx)
	if err != nil {
		return packagecapsule.Manifest{}, err
	}
	manifest, root, cleanup, err := ops.prepare(ctx, cfg.ARMTargetEnvironment, stage, cfg.Machine.ID, source, true, false, func(p packagecapsule.Progress) {
		lane := "packages"
		if p.Source == "foreign" {
			lane = "aur"
		}
		emit(lane, systemSnapshotLane{State: p.Phase, Current: p.Package, Completed: p.Completed, Total: p.Total, Percent: lanePercent(p.Completed, p.Total), BytesDone: nonnegativeARMBytes(p.Bytes)})
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return manifest, err
	}
	emit("packages", systemSnapshotLane{State: "prepared", Current: fmt.Sprintf("%d artifacts / %d exceptions", manifest.Summary.Captured, len(manifest.Exceptions)), Percent: 1})
	emit("aur", systemSnapshotLane{State: "review", Current: fmt.Sprintf("%d review notes; native source builds remain manual", len(manifest.ManualReview))})
	emit("repository", systemSnapshotLane{State: "archiving", Current: "saving packages-arm64"})
	archiveStart := time.Now()
	err = ops.archive(ctx, repo, "packages-arm64", cfg.Machine.ID, []string{root}, func(p restic.BackupProgress) {
		emit("repository", systemSnapshotLane{State: "archiving", Current: "saving encrypted ARM64 capsule", Completed: int(p.FilesDone), Total: int(p.TotalFiles), Percent: p.PercentDone, BytesDone: p.BytesDone, Rate: float64(p.BytesDone) / time.Since(archiveStart).Seconds()})
	})
	return manifest, err
}

func waitARMEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return armDoneMsg{err: fmt.Errorf("ARM64 package operation ended without a result")}
		}
		return msg
	}
}

func (m Model) finishARMCapture(msg armDoneMsg) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.busy, m.cancel, m.armStage = false, nil, "done"
	if m.armLanes == nil {
		m.armLanes = map[string]systemSnapshotLane{}
	}
	if msg.err != nil {
		for key, lane := range m.armLanes {
			if key != "aur" && lane.State != "prepared" && lane.State != "complete" {
				lane.State = "failed"
				m.armLanes[key] = lane
			}
		}
		m.armManifest = nil
		m.err, m.status = msg.err.Error(), "ARM64 package preparation failed; Enter retries"
		m.armLanes["repository"] = systemSnapshotLane{State: "failed", Current: "capsule was not saved"}
	} else {
		m.armManifest, m.err = &msg.manifest, ""
		m.status = fmt.Sprintf("ARM64 capsule saved — %d artifacts / %d exceptions", msg.manifest.Summary.Captured, len(msg.manifest.Exceptions))
		m.armLanes["repository"] = systemSnapshotLane{State: "complete", Current: "packages-arm64 saved in default repository", Percent: 1}
	}
	return m, nil
}

func (m Model) armPackagesScreen() string {
	body := m.styles.header.Render("APPS / ARM64") + "\n\n"
	if m.err != "" {
		body += m.styles.status.Render(m.err) + "\n\n"
	}
	if d := m.cfg.Active(); d != nil {
		body += "Default repository  " + d.Name + "\nARM package target  packages-arm64\n\n"
	}
	if m.armStage == "setup" {
		return body + "ONE-TIME SNAPDRAGON SETUP\n\nUse repository settings exported from your Snapdragon PC.\nWeazlback remembers them and saves packages directly to the default repository.\n\n" + m.armInput.View() + "\n\nenter remember + prepare  •  esc back"
	}
	for _, lane := range []struct{ key, title string }{{"packages", "PACKAGES"}, {"aur", "AUR"}, {"repository", "REPO"}} {
		body += snapshotLaneView(lane.title, m.armLanes[lane.key], m.height > 0 && m.height < 30) + "\n\n"
	}
	if m.armManifest != nil {
		body += fmt.Sprintf("Saved %s  •  %d artifacts  •  %s  •  %d exceptions\n", m.armManifest.CapturedAt.Local().Format("2006-01-02 15:04"), m.armManifest.Summary.Captured, bytesText(uint64(m.armManifest.Summary.Bytes)), len(m.armManifest.Exceptions))
		for i, note := range m.armManifest.ManualReview {
			if i == 4 {
				body += "Further review notes are stored in the capsule.\n"
				break
			}
			body += "• " + note + "\n"
		}
	}
	if m.busy {
		body += "Elapsed " + time.Since(m.armStart).Round(time.Second).String() + "\nCtrl+C cancels  •  ARM preparation runs independently of System Set"
	} else {
		body += "enter refresh ARM packages  •  T change Snapdragon settings  •  esc back"
	}
	return body
}

func nonnegativeARMBytes(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
