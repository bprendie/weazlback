package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bprendie/weazlback/internal/generation"
	"github.com/bprendie/weazlback/internal/restic"
	tea "github.com/charmbracelet/bubbletea"
)

type systemSnapshotListMsg struct {
	sets []generation.Generation
	err  error
}

func (m Model) runSystemSnapshotAction(action string) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	if action == "create" || action == "retry" {
		if m.vault == nil || !m.vault.Unlocked() {
			m.err = "unlock the vault before creating a Full System Snapshot"
			return m, nil
		}
		return m.authorizeSystemSnapshot(action)
	}
	if action == "list" {
		_, _, repo, err := m.activeRuntime("")
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		machineID := m.cfg.Machine.ID
		m.busy, m.operation, m.err = true, "System Snapshot list", ""
		return m, func() tea.Msg {
			points, listErr := restic.NewService(io.Discard).SnapshotsForMachine(context.Background(), repo, machineID)
			return systemSnapshotListMsg{sets: generation.Catalog(points), err: listErr}
		}
	}
	executable, err := os.Executable()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	args := []string{"system", "snapshot", action}
	command := exec.Command(executable, args...)
	var diagnostics bytes.Buffer
	command.Stdin, command.Stdout = os.Stdin, os.Stdout
	command.Stderr = io.MultiWriter(os.Stderr, &diagnostics)
	m.busy, m.operation, m.err = true, "System Snapshot "+action, ""
	return m, tea.ExecProcess(command, func(err error) tea.Msg {
		if err != nil {
			message := lastDiagnostic(diagnostics.String())
			if message != "" {
				err = fmt.Errorf("%w: %s", err, message)
			}
		}
		return operationDoneMsg{err: err}
	})
}

func lastDiagnostic(value string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(value, "\r", "\n")), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if text := strings.TrimSpace(lines[index]); text != "" {
			return text
		}
	}
	return ""
}
