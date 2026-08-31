package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bprendie/weazlback/internal/contracts"
	statusstore "github.com/bprendie/weazlback/internal/status"
)

func widgetCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("widget action is required")
	}
	switch args[0] {
	case "open", "restore", "check":
		mode := args[0]
		if mode == "open" {
			mode = "home"
		}
		return launchWidgetTUI(mode, stderr)
	case "backup":
		action := "backup"
		if len(args) == 2 && args[1] == "heavy" {
			action = "heavy"
		} else if len(args) != 1 {
			return errors.New("widget backup accepts only the optional heavy profile")
		}
		return requestVaultAgent(ctx, action)
	case "unlock":
		return widgetUnlock(os.Stdin, stdout)
	case "agent":
		return widgetAgent(ctx, os.Stdin, stdout)
	case "vault-status":
		if err := requestVaultAgent(ctx, "status"); err != nil {
			_, err = fmt.Fprintln(stdout, "Vault Locked")
			return err
		}
		_, err := fmt.Fprintln(stdout, "Vault Open")
		return err
	case "repository-health":
		if err := requestVaultAgent(ctx, "check"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, "repository healthy")
		return err
	case "travel":
		if len(args) != 2 {
			return errors.New("travel requires days")
		}
		if args[1] == "off" {
			return clearTravel(stdout)
		}
		days, err := strconv.Atoi(args[1])
		if err != nil || days < 1 || days > 365 {
			return errors.New("travel days must be between 1 and 365")
		}
		return setTravel(days, stdout)
	case "cancel":
		return cancelWidgetBackup(stdout)
	case "remind":
		return widgetReminder(stdout)
	default:
		return fmt.Errorf("unknown widget action %q", args[0])
	}
}

func clearTravel(stdout io.Writer) error {
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	value, err := store.Load()
	if err != nil {
		return err
	}
	value.TravelUntil = nil
	if err := store.Save(value); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "travel notifications enabled")
	return err
}

func widgetReminder(stdout io.Writer) error {
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	value, err := store.Load()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if value.TravelUntil != nil && value.TravelUntil.After(now) {
		return nil
	}
	if value.LastHealthy != nil && now.Sub(*value.LastHealthy) < 7*24*time.Hour {
		return nil
	}
	if value.LastReminder != nil && now.Sub(*value.LastReminder) < 24*time.Hour {
		return nil
	}
	message := "Your system has never had a successful Core + Home backup."
	if value.LastHealthy != nil {
		message = fmt.Sprintf("Your last successful Core + Home backup was %d days ago.", int(now.Sub(*value.LastHealthy).Hours()/24))
	}
	command := exec.Command("notify-send", "Weazlback: time to back up", message)
	if err := command.Run(); err != nil {
		return err
	}
	value.LastReminder = &now
	if err := store.Save(value); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "backup reminder sent")
	return err
}

func cancelWidgetBackup(stdout io.Writer) error {
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	value, err := store.Load()
	if err != nil {
		return err
	}
	if !value.Cancellable || value.OperationPID <= 1 {
		return errors.New("no cancellable Weazlback operation")
	}
	executable, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", value.OperationPID))
	if err != nil || !strings.Contains(filepath.Base(executable), "weazlback") {
		return errors.New("operation identity is stale; refusing to signal it")
	}
	if err := syscall.Kill(value.OperationPID, syscall.SIGINT); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "cancellation requested")
	return err
}

func launchWidgetTUI(mode string, stderr io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	launcher := home + "/.weazlback/bin/weazlback-launch"
	command := exec.Command(launcher)
	command.Env = append(os.Environ(), "WEAZLBACK_START_MODE="+mode)
	command.Stdout, command.Stderr = io.Discard, stderr
	return command.Start()
}

func setTravel(days int, stdout io.Writer) error {
	path, err := statusstore.DefaultPath()
	if err != nil {
		return err
	}
	store := statusstore.Store{Path: path}
	value, err := store.Load()
	if err != nil {
		return err
	}
	until := time.Now().Add(time.Duration(days) * 24 * time.Hour).UTC()
	value.TravelUntil = &until
	if value.State == "" {
		value = contracts.Status{State: "not-configured", TravelUntil: &until}
	}
	if err := store.Save(value); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "travel notifications muted until %s\n", until.Format(time.RFC3339))
	return err
}
