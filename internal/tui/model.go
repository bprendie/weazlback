package tui

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/apprestore"
	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/bprendie/weazlback/internal/vault"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeHome mode = iota
	modeBackup
	modeSnapshots
	modeRestore
	modeProfiles
	modeDestinations
	modeRecovery
	modeCheck
	modeTune
	modeSchedule
	modeNuke
	modeSystemSnapshot
)

type navEntry struct {
	key, label string
	mode       mode
}

type restoreBasketItem struct {
	Path, Snapshot, MachineID, Profile string
	Time                               time.Time
}

var navigation = []navEntry{
	{"h", "Home", modeHome}, {"b", "Backup now", modeBackup},
	{"s", "Restore Points", modeSnapshots}, {"r", "Restore", modeRestore},
	{"p", "Profiles", modeProfiles}, {"d", "Destinations", modeDestinations},
	{"k", "Recovery kit", modeRecovery}, {"c", "Check repository", modeCheck},
	{"u", "Tune", modeTune},
	{"t", "Schedule", modeSchedule},
	{"x", "Nuke repository", modeNuke},
	{"y", "System Snapshot", modeSystemSnapshot},
}

type Model struct {
	styles               styles
	mode                 mode
	workspace            string
	backupMode           mode
	backupIndex          int
	backupRailFocused    bool
	width                int
	height               int
	index                int
	railFocused          bool
	status               string
	err                  string
	cfg                  config.Config
	vault                *vault.File
	vaultStage           string
	vaultInput           textinput.Model
	pendingPass          string
	busy                 bool
	operation            string
	progress             restic.BackupProgress
	snapshots            []restic.Snapshot
	cancel               context.CancelFunc
	destinationStage     string
	destinationInputs    []textinput.Model
	destinationIndex     int
	destinationSelection int
	sshFingerprint       string
	sshHostLine          string
	selectedProfile      string
	recoveryStage        string
	recoveryPrepare      bool
	recoveryInputs       []textinput.Model
	recoveryIndex        int
	sudoPending          bool
	incomplete           bool
	skippedPaths         []string
	skippedManifest      string
	restoreStage         string
	restoreInput         textinput.Model
	restoreEntries       []restic.FileEntry
	restoreVisible       []restic.FileEntry
	restoreIndex         int
	restoreSnapshot      int
	restoreSearching     bool
	restorePathMode      bool
	restoreTreePath      string
	restoreBasket        map[string]restoreBasketItem
	restorePlans         []restoretxn.Plan
	restorePreflights    []restoretxn.Preflight
	restoreTransaction   restoretxn.Progress
	restoreTargetMode    string
	restoreConflict      restoretxn.ConflictPolicy
	restoreResult        restoretxn.Result
	restoreIntent        string
	restoreBundleChoices map[restoretxn.Bundle]bool
	restoreBundleMode    string
	restoreBundleTime    time.Time
	restoreBundleParts   []restoretxn.Component
	restoreBundleDeletes []string
	restoreBundleJournal string
	restoreScopeDecision freshrestore.ScopeDecision
	restoreSafetyBackup  bool
	restoreAppPlan       apprestore.Plan
	restoreAppResult     apprestore.Result
	restoreAppProgress   apprestore.Progress
	restoreAppJournal    string
	restoreIdentities    []restic.Identity
	restoreIdentity      int
	restoreCatalog       catalog.Catalog
	restoreCatalogState  string
	restoreResults       []catalog.PathRecord
	restoreVersionPath   string
	restoreDeletedOnly   bool
	restoreAllMachines   bool
	restorePathHistory   bool
	restoreContentOpen   bool
	restoreContentQuery  string
	restoreLiveHints     []string
	applications         *inventory.ApplicationManifest
	packageManifest      *packagecapsule.Manifest
	packageProgress      packagecapsule.Progress
	packageStage         string
	packageBuildAUR      bool
	helpVisible          bool
	heavyReport          heavy.Report
	nukeStage            string
	nukeMode             string
	nukeInput            textinput.Model
	nukePass             string
	nukeRemoveDirectory  bool
	tuneStage            string
	tuneTrials           []restic.ConnectionTrial
	tuneConnection       int
	tuneActiveConnection int
	tuneFrame            int
	tuneProbe            restic.UploadProbe
	tuneProbeWritten     int64
	tuneProbeElapsed     time.Duration
	tuneInput            textinput.Model
}

func New() Model {
	input := textinput.New()
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '•'
	input.Focus()
	m := Model{styles: newStyles(), mode: modeHome, workspace: "backup", status: "hardened backup",
		backupMode: modeHome, vaultInput: input, selectedProfile: "core", nukeMode: "full", restoreBasket: map[string]restoreBasketItem{},
		restoreConflict: restoretxn.ReplacePreserving, restoreTargetMode: "original"}
	switch os.Getenv("WEAZLBACK_START_MODE") {
	case "backup":
		m.mode, m.index = modeBackup, 1
	case "restore":
		m.mode, m.index, m.workspace, m.restoreStage = modeRestore, 3, "restore", "dashboard"
	case "check":
		m.mode, m.index = modeCheck, 7
	}
	path, err := config.Path()
	if err != nil {
		m.err = err.Error()
		return m
	}
	m.cfg, err = config.Load(path)
	if err != nil {
		m.err = err.Error()
		return m
	}
	vaultPath, err := vault.Path(m.cfg.ActiveVault)
	if err != nil {
		m.err = err.Error()
		return m
	}
	m.vault = vault.New(vaultPath)
	exists, err := m.vault.Exists()
	if err != nil {
		m.err = err.Error()
		return m
	}
	if exists {
		m.vaultStage = "unlock"
		m.vaultInput.Prompt = "vault passphrase > "
		m.status = "unlock private vault"
	} else {
		m.vaultStage = "create"
		m.vaultInput.Prompt = "new vault passphrase > "
		m.status = "create private vault — no recovery"
	}
	return m
}

func (m Model) Init() tea.Cmd { return tea.Batch(textinput.Blink, themeTick(), waitRestoreSignal()) }

func (m Model) View() string {
	width := max(36, m.width-8)
	height := max(12, m.height-2)
	header := m.header(width)
	bodyHeight := max(3, height-lineHeight(header)-6)
	body := m.content(width, bodyHeight)
	help := "tab focus • ↑/↓ move • enter select • D next target • ? help • q detach • ^c cancel"
	if m.workspace == "restore" {
		help = "/ search • /p path • arrows navigate • space select • B backup mode"
	}
	if m.width < 90 {
		help = "tab focus • arrows move • enter select • q detach • ^c cancel"
	}
	if m.width < 64 {
		help = "tab • arrows • enter • ? • q"
	}
	footer := m.styles.status.Render(m.status) + "\n" + m.styles.help.Render(help)
	return m.styles.frame.Render(strings.Join([]string{header, body, footer}, "\n"))
}
