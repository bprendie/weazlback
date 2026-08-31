package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bprendie/weazlback/internal/app"
)

func main() {
	ignoreTerminalHangup()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "weazlback: %v\n", err)
		os.Exit(1)
	}
}

// The visible terminal is only a tmux client. Closing it must never terminate
// the vault-owning backend or an active repository operation.
func ignoreTerminalHangup() { signal.Ignore(syscall.SIGHUP) }
