package tui

import (
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

type restoreModeMsg struct{}

func waitRestoreSignal() tea.Cmd {
	return func() tea.Msg {
		channel := make(chan os.Signal, 1)
		signal.Notify(channel, syscall.SIGUSR1)
		defer signal.Stop(channel)
		<-channel
		return restoreModeMsg{}
	}
}
