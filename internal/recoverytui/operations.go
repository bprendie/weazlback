package recoverytui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/restoretxn"
	"github.com/bprendie/weazlback/internal/securelog"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) prepare(adopt bool) tea.Cmd {
	snapshot := "latest"
	if len(m.points) > 0 && m.pointIndex < len(m.points) {
		snapshot = m.points[m.pointIndex].ID
	}
	options := freshrestore.Options{RecoveryPath: m.kit, Destination: m.destination, Passphrase: m.passphrase, Snapshot: snapshot, Hostname: m.hostname,
		Scope: m.scope, Repository: m.repository, AdoptLocal: adopt, MachineID: m.machineID, AdoptSourceIdentity: m.adoptIdentity,
		TargetMachineID: m.targetMachineID, PersistTargetIdentity: m.persistTargetIdentity}
	return func() tea.Msg {
		r, err := freshrestore.Prepare(context.Background(), options)
		return preparedMsg{r, err}
	}
}

func (m Model) chooseTargetIdentity(mode string) Model {
	m.adoptIdentity, m.targetIdentityMode = false, mode
	hostname, _ := os.Hostname()
	if mode == "new" {
		m.targetMachineID, m.persistTargetIdentity = config.NewMachine(hostname).ID, true
		return m
	}
	path, _ := config.Path()
	_, statErr := os.Stat(path)
	cfg, _ := config.Load(path)
	m.targetMachineID = cfg.Machine.ID
	m.persistTargetIdentity = os.IsNotExist(statErr)
	return m
}

func (m Model) loadPoints(profile string) tea.Cmd {
	kit, destination, machine := m.kit, m.destination, m.machineID
	passphrase := append([]byte(nil), m.passphrase...)
	return func() tea.Msg {
		defer zeroBytes(passphrase)
		points, err := freshrestore.RecoveryPoints(context.Background(), kit, passphrase, destination, machine, profile)
		return pointsMsg{points: points, err: err}
	}
}

func (m Model) loadPointFiles() tea.Cmd {
	kit, destination := m.kit, m.destination
	point := m.points[m.pointIndex]
	passphrase := append([]byte(nil), m.passphrase...)
	return func() tea.Msg {
		defer zeroBytes(passphrase)
		result, err := freshrestore.RecoveryFiles(context.Background(), kit, passphrase, destination, point.ID)
		return filesMsg{point: result.Snapshot, files: result.Files, err: err}
	}
}

func (m *Model) filterFiles() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	m.visibleFiles = m.visibleFiles[:0]
	for _, file := range m.files {
		if query == "" || strings.Contains(strings.ToLower(file.Path), query) || fuzzyPath(file.Path, query) {
			m.visibleFiles = append(m.visibleFiles, file)
		}
	}
	m.fileIndex = min(m.fileIndex, max(0, len(m.visibleFiles)-1))
}

func fuzzyPath(path, query string) bool {
	index := 0
	for _, char := range strings.ToLower(path) {
		if index < len(query) && byte(char) == query[index] {
			index++
		}
	}
	return index == len(query)
}

func (m Model) executeSelective() tea.Cmd {
	options := freshrestore.SelectiveOptions{RecoveryPath: m.kit, Destination: m.destination, MachineID: m.machineID,
		Snapshot: m.points[m.pointIndex].ID, SourcePath: m.selectedPath, TargetMachineID: m.targetMachineID,
		Repository: m.repository, Passphrase: append([]byte(nil), m.passphrase...), WorkDir: recoveryWorkDir()}
	events := make(chan tea.Msg, 16)
	go func() {
		defer zeroBytes(options.Passphrase)
		options.Progress = func(value restoretxn.Progress) { events <- selectiveProgressMsg{progress: value, events: events} }
		result, err := freshrestore.RestoreRecoverySelection(context.Background(), options)
		events <- selectiveDoneMsg{result: result, err: err}
		close(events)
	}()
	return waitEvent(events)
}

func recoveryWorkDir() string {
	return filepath.Join(os.TempDir(), "weazlback-"+fmt.Sprint(os.Getuid()), "restore-mode")
}

func (m Model) rebuildCatalog() tea.Cmd {
	kit, destination, machine := m.kit, m.destination, m.machineID
	passphrase := append([]byte(nil), m.passphrase...)
	return func() tea.Msg {
		defer zeroBytes(passphrase)
		path, err := freshrestore.RebuildRecoveryCatalog(context.Background(), kit, passphrase, destination, machine)
		return catalogDoneMsg{path: path, err: err}
	}
}

func (m Model) loadIdentities() tea.Cmd {
	kit, destination := m.kit, m.destination
	passphrase := append([]byte(nil), m.passphrase...)
	return func() tea.Msg {
		defer zeroBytes(passphrase)
		identities, err := freshrestore.ReadRecoveryIdentities(context.Background(), kit, passphrase, destination)
		return identitiesMsg{identities: identities, err: err}
	}
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func (m Model) loadDestinations() tea.Cmd {
	kit := m.kit
	passphrase := append([]byte(nil), m.passphrase...)
	return func() tea.Msg {
		defer zeroBytes(passphrase)
		catalog, err := freshrestore.ReadRecoveryCatalog(kit, passphrase)
		return destinationsMsg{catalog: catalog, err: err}
	}
}

func (m Model) authorizeAdoption() (tea.Model, tea.Cmd) {
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return sudoMsg{err} })
}

func (m Model) execute() tea.Cmd {
	r := m.restore
	events := make(chan tea.Msg, 32)
	r.Options.Progress = func(value freshrestore.RestoreProgress) { events <- progressMsg{progress: value, events: events} }
	go func() {
		report, err := r.Execute(context.Background(), os.Stdout)
		payload, _ := json.Marshal(struct {
			Report freshrestore.Report `json:"report"`
			Error  string              `json:"error,omitempty"`
		}{Report: report, Error: errorString(err, "")})
		_, _ = securelog.Write(r.Session.Vault, "restore", securelog.ID(), payload)
		events <- executedMsg{report, err}
		close(events)
	}()
	return waitEvent(events)
}

func waitEvent(events <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-events } }

func (m *Model) close() {
	if m.restore != nil {
		m.restore.Close()
		m.restore = nil
	}
	zeroBytes(m.passphrase)
}

func Run() error {
	model, err := tea.NewProgram(New(), tea.WithAltScreen()).Run()
	if m, ok := model.(Model); ok {
		m.close()
	}
	return err
}
