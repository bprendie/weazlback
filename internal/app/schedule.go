package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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
)

const routineInterval = 7 * 24 * time.Hour

type scheduleState struct {
	LastKitReminder *time.Time `json:"last_kit_reminder,omitempty"`
}

func scheduleCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("schedule run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	force := flags.Bool("force", false, "run even when routine protection is current")
	if len(args) > 0 && args[0] == "run" {
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	store, err := defaultStatusStore()
	if err != nil {
		return err
	}
	status, err := store.Load()
	if err != nil {
		return err
	}
	last := status.LastRoutine
	if last == nil {
		last = status.LastHealthy
	}
	if !*force && last != nil && time.Since(*last) < routineInterval {
		return monthlyRecoveryReminder(status, stdout)
	}
	allowed, detail := batteryAllowsBackup()
	if !allowed {
		notifySchedule(status, "Backup waiting for battery", detail)
		return nil
	}
	lock, err := acquireScheduleLock()
	if err != nil {
		return err
	}
	if lock == nil {
		_, err = fmt.Fprintln(stdout, "another Weazlback operation owns the schedule lock")
		return err
	}
	defer releaseScheduleLock(lock)
	delays := []time.Duration{0, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}
	for attempt, delay := range delays {
		if delay > 0 {
			if err := waitSchedule(ctx, delay); err != nil {
				return err
			}
		}
		err = requestVaultAgent(ctx, "backup")
		if err == nil {
			_, writeErr := fmt.Fprintf(stdout, "scheduled Core + Home backup complete after %d attempt(s)\n", attempt+1)
			return writeErr
		}
		if strings.Contains(err.Error(), "Vault is locked") {
			notifySchedule(status, "Weazlback vault is locked", "Open the Weazlback widget to allow the overdue backup.")
			return nil
		}
		if !retryableBackupError(err) {
			notifySchedule(status, "Scheduled backup failed", "Open Weazlback for encrypted details.")
			return err
		}
	}
	notifySchedule(status, "Backup destination is unreachable", "Retries at 5m, 15m, 1h, and 6h all failed.")
	return err
}

func batteryAllowsBackup() (bool, string) {
	roots, _ := filepath.Glob("/sys/class/power_supply/*")
	for _, root := range roots {
		typeValue, _ := os.ReadFile(filepath.Join(root, "type"))
		if strings.TrimSpace(string(typeValue)) != "Battery" {
			continue
		}
		status, _ := os.ReadFile(filepath.Join(root, "status"))
		if strings.TrimSpace(string(status)) != "Discharging" {
			return true, "AC power"
		}
		capacityBytes, err := os.ReadFile(filepath.Join(root, "capacity"))
		capacity, parseErr := strconv.Atoi(strings.TrimSpace(string(capacityBytes)))
		if err == nil && parseErr == nil && capacity >= 59 {
			return true, fmt.Sprintf("battery %d%%", capacity)
		}
		return false, fmt.Sprintf("Battery must be at least 59%% (currently %d%%).", capacity)
	}
	return true, "no battery"
}

func acquireScheduleLock() (*os.File, error) {
	path, err := schedulePath("operation.lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, err
	}
	return file, nil
}

func releaseScheduleLock(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func waitSchedule(ctx context.Context, delay time.Duration) error {
	if value := os.Getenv("WEAZLBACK_RETRY_SCALE_MS"); value != "" {
		milliseconds, _ := strconv.Atoi(value)
		delay = time.Duration(milliseconds) * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableBackupError(err error) bool {
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"connection", "network", "timeout", "unreachable", "no route", "refused", "reset by peer"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func notifySchedule(status contracts.Status, title, body string) {
	if status.TravelUntil != nil && status.TravelUntil.After(time.Now()) {
		return
	}
	_ = exec.Command("notify-send", title, body).Run()
}

func schedulePath(name string) (string, error) {
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "schedule", name), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".weazlback", "schedule", name), err
}

func monthlyRecoveryReminder(_ contracts.Status, stdout io.Writer) error {
	path, err := schedulePath("state.json")
	if err != nil {
		return err
	}
	var state scheduleState
	data, _ := os.ReadFile(path)
	_ = json.Unmarshal(data, &state)
	if state.LastKitReminder != nil && time.Since(*state.LastKitReminder) < 30*24*time.Hour {
		return nil
	}
	now := time.Now().UTC()
	command := exec.Command("notify-send", "Weazlback recovery kit", "Verify or refresh your recovery USB this month.")
	if err := command.Run(); err != nil {
		return err
	}
	state.LastKitReminder = &now
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, _ := json.Marshal(state)
	if err := atomicAppWrite(path, encoded); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "monthly recovery-kit reminder sent")
	return err
}
