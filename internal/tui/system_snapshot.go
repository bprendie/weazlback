package tui

import (
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) runSystemSnapshotAction(action string) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	executable, err := os.Executable()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	args := []string{"system", "snapshot", action}
	command := exec.Command(executable, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	m.busy, m.operation, m.err = true, "System Snapshot "+action, ""
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return operationDoneMsg{err: err} })
}
