package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/bprendie/weazlback/internal/restoretxn"
	tea "github.com/charmbracelet/bubbletea"
)

type restorePreflightAuthMsg struct {
	alternate string
	err       error
}

type restoreLoadAuthMsg struct {
	err   error
	index int
	point bool
}

func authorizeRestoreLoad(index int, point bool) tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg { return restoreLoadAuthMsg{err: err, index: index, point: point} })
}

func authorizeRestorePreflight(alternate string) tea.Cmd {
	return tea.ExecProcess(exec.Command("sudo", "-v"), func(err error) tea.Msg { return restorePreflightAuthMsg{alternate: alternate, err: err} })
}

func validateRestoreTargets(groups map[string][]restoretxn.Item) error {
	var items []restoretxn.Item
	for _, group := range groups {
		items = append(items, group...)
	}
	for i, a := range items {
		for _, b := range items[i+1:] {
			if a.TargetPath == b.TargetPath || strings.HasPrefix(a.TargetPath, b.TargetPath+"/") || strings.HasPrefix(b.TargetPath, a.TargetPath+"/") {
				return fmt.Errorf("restore targets overlap: %s and %s; adjust the basket or destination", a.TargetPath, b.TargetPath)
			}
		}
	}
	return nil
}
