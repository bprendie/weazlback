package recoverytui

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type preparedMsg struct {
	restore *freshrestore.Restore
	err     error
}
type executedMsg struct {
	report freshrestore.Report
	err    error
}
type progressMsg struct {
	progress freshrestore.RestoreProgress
	events   <-chan tea.Msg
}
type sudoMsg struct{ err error }
type executeSudoMsg struct{ err error }
type destinationsMsg struct {
	catalog freshrestore.RecoveryCatalog
	err     error
}
type identitiesMsg struct {
	identities []restic.Identity
	err        error
}
type pointsMsg struct {
	points []restic.Snapshot
	err    error
}
type filesMsg struct {
	point restic.Snapshot
	files []restic.FileEntry
	err   error
}
type selectiveDoneMsg struct {
	result restoretxn.Result
	err    error
}
type selectiveProgressMsg struct {
	progress restoretxn.Progress
	events   <-chan tea.Msg
}
type catalogDoneMsg struct {
	path string
	err  error
}

type Model struct {
	width, height             int
	stage                     string
	input                     textinput.Model
	kit, repository, hostname string
	kits                      []string
	kitIndex                  int
	destination               string
	destinations              []config.Destination
	destinationIndex          int
	identities                []restic.Identity
	identityIndex             int
	machineID                 string
	adoptIdentity             bool
	targetIdentityMode        string
	targetMachineID           string
	persistTargetIdentity     bool
	scope                     string
	passphrase                []byte
	restore                   *freshrestore.Restore
	report                    freshrestore.Report
	appProgress               freshrestore.RestoreProgress
	filesystemProgress        freshrestore.RestoreProgress
	review                    []string
	reviewIndex               int
	started                   time.Time
	err                       string
	points                    []restic.Snapshot
	pointIndex                int
	pointIntent               string
	files, visibleFiles       []restic.FileEntry
	fileIndex                 int
	selectedPath              string
	selectiveProgress         restoretxn.Progress
	selectiveResult           restoretxn.Result
	catalogPath               string
}

var (
	cyan  = lipgloss.Color("#39ffde")
	pink  = lipgloss.Color("#ff4ecd")
	dim   = lipgloss.Color("#778899")
	panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cyan).Padding(1, 2)
)

func New() Model {
	kits := discoverKits()
	input := textinput.New()
	input.Prompt = "recovery kit > "
	input.SetValue(defaultKit(kits))
	input.Focus()
	stage := "kit"
	if len(kits) > 1 {
		stage = "kit-choice"
	}
	return Model{stage: stage, input: input, kits: kits, hostname: "original", scope: "home"}
}

func defaultKit(kits []string) string {
	if len(kits) > 0 {
		return kits[0]
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "weazlback-recovery.wzrk")
}

func discoverKits() []string {
	seen, result := map[string]bool{}, []string{}
	add := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			absolute, _ := filepath.Abs(path)
			if !seen[absolute] {
				seen[absolute], result = true, append(result, absolute)
			}
		}
	}
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	add(filepath.Join(filepath.Dir(executable), "*.wzrk"))
	add(filepath.Join(cwd, "*.wzrk"))
	add("/mnt/*.wzrk")
	add("/mnt/*/*.wzrk")
	sort.Strings(result)
	return result
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func errorString(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case preparedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			if errors.Is(msg.err, os.ErrNotExist) {
				m.input = textinput.New()
				m.input.Prompt = "recovery kit > "
				m.input.SetValue(m.kit)
				m.input.Focus()
				m.stage = "kit"
				return m, textinput.Blink
			}
			m.stage = "access"
			return m, nil
		}
		m.restore, m.stage, m.err = msg.restore, "plan", ""
		m.review = reviewLines(msg.restore.Plan)
	case destinationsMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.stage = "pass"
			m.input = textinput.New()
			m.input.Prompt = "vault passphrase > "
			m.input.EchoMode, m.input.EchoCharacter = textinput.EchoPassword, '•'
			m.input.Focus()
			return m, textinput.Blink
		}
		m.destinations = msg.catalog.Destinations
		m.destinationIndex = 0
		for i := range m.destinations {
			if m.destinations[i].ID == msg.catalog.Active {
				m.destinationIndex = i
			}
		}
		if len(m.destinations) == 1 {
			m.destination, m.stage = m.destinations[0].ID, "identity-loading"
			return m, m.loadIdentities()
		} else {
			m.stage = "destination-choice"
		}
	case identitiesMsg:
		if msg.err != nil {
			m.err, m.stage = msg.err.Error(), "destination-choice"
			return m, nil
		}
		m.identities, m.identityIndex = msg.identities, 0
		if len(msg.identities) == 0 {
			m.err, m.stage = "repository has no Weazlback machine history", "destination-choice"
		} else if len(msg.identities) == 1 {
			m.machineID, m.stage = msg.identities[0].ID, "target-identity"
		} else {
			m.stage = "identity-choice"
		}
	case pointsMsg:
		if msg.err != nil || len(msg.points) == 0 {
			m.err, m.stage = errorString(msg.err, "source identity has no Restore Points"), "action-choice"
			return m, nil
		}
		m.points, m.pointIndex = msg.points, 0
		if m.pointIntent == "selective" {
			m.stage = "selective-loading"
			return m, m.loadPointFiles()
		}
		m.stage = "point-choice"
	case filesMsg:
		if msg.err != nil {
			m.err, m.stage = msg.err.Error(), "action-choice"
			return m, nil
		}
		m.files, m.visibleFiles, m.fileIndex, m.stage, m.err = msg.files, msg.files, 0, "selective", ""
		m.input = textinput.New()
		m.input.Prompt = "/ "
		m.input.Focus()
		return m, textinput.Blink
	case selectiveProgressMsg:
		m.selectiveProgress = msg.progress
		return m, waitEvent(msg.events)
	case selectiveDoneMsg:
		m.selectiveResult, m.stage = msg.result, "selective-done"
		if msg.err != nil {
			m.err = msg.err.Error()
		}
	case catalogDoneMsg:
		m.catalogPath = msg.path
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
		}
		m.stage = "action-choice"
	case executedMsg:
		m.report, m.stage = msg.report, "done"
		if msg.err != nil {
			m.err = msg.err.Error()
		}
	case progressMsg:
		if msg.progress.Phase == "applications" {
			m.appProgress = msg.progress
		} else {
			m.filesystemProgress = msg.progress
		}
		return m, waitEvent(msg.events)
	case sudoMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.stage = "loading"
		return m, m.prepare(true)
	case executeSudoMsg:
		if msg.err != nil {
			m.stage, m.err = "plan", msg.err.Error()
			return m, nil
		}
		m.stage = "running"
		m.started = time.Now()
		return m, m.execute()
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}
