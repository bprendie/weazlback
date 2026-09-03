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
	"time"

	"github.com/bprendie/weazlback/internal/benchmark"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/heavy"
	"github.com/bprendie/weazlback/internal/inventory"
	"github.com/bprendie/weazlback/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

const Version = "1.1.0-dev"

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		program := tea.NewProgram(tui.New(), tea.WithAltScreen())
		final, err := program.Run()
		if model, ok := final.(tui.Model); ok {
			model.Close()
		}
		return err
	}
	switch args[0] {
	case "version", "--version":
		_, err := fmt.Fprintln(stdout, Version)
		return err
	case "help", "--help", "-h":
		_, err := fmt.Fprint(stdout, usage)
		return err
	case "doctor":
		return doctor(stdout)
	case "inventory":
		return inventoryCommand(ctx, args[1:], stdout, stderr)
	case "heavy":
		return heavyCommand(args[1:], stdout, stderr)
	case "applications":
		return applicationsCommand(ctx, args[1:], stdout, stderr)
	case "packages":
		return packagesCommand(ctx, args[1:], stdout, stderr)
	case "benchmark":
		return benchmarkCommand(ctx, args[1:], stdout, stderr)
	case "browser":
		return browserCommand(args[1:], stdout, stderr)
	case "status":
		return statusCommand(args[1:], stdout)
	case "widget":
		return widgetCommand(ctx, args[1:], stdout, stderr)
	case "schedule":
		return scheduleCommand(ctx, args[1:], stdout, stderr)
	case "internal-nuke":
		return nukeCommand(ctx, args[1:], os.Stdin, stdout, stderr)
	case "rotate":
		return rotateCommand(ctx, args[1:], stdout, stderr)
	case "init":
		return initCommand(ctx, args[1:], stdout, stderr)
	case "backup":
		return backupCommand(ctx, args[1:], stdout, stderr)
	case "tune":
		return tuneCommand(ctx, args[1:], stdout, stderr)
	case "list", "snapshots":
		return snapshotsCommand(ctx, args[1:], stdout, stderr)
	case "identity":
		return identityCommand(ctx, args[1:], stdout, stderr)
	case "check":
		return checkCommand(ctx, args[1:], stdout, stderr)
	case "prune":
		return pruneCommand(ctx, args[1:], stdout, stderr)
	case "recovery":
		return recoveryCommand(args[1:], stdout, stderr)
	case "restore":
		return restoreCommand(ctx, args[1:], stdout, stderr)
	case "files":
		return filesCommand(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func heavyCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("heavy inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Heavy root (defaults to configured Heavy profile)")
	if len(args) > 0 && args[0] == "inspect" {
		args = args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	roots := []string{*root}
	if *root == "" {
		path, err := config.Path()
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		profile := findProfile(cfg, "heavy")
		if profile == nil {
			return errors.New("Heavy profile is not configured")
		}
		roots = profile.Includes
	}
	return writeJSON(stdout, heavy.Inspect(roots))
}

func inventoryCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("inventory", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "", "write metadata-only JSON report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	report, err := inventory.Capture(ctx)
	if err != nil {
		return err
	}
	if *output != "" {
		if err := inventory.Write(*output, report); err != nil {
			return err
		}
	}
	return writeJSON(stdout, report)
}

func benchmarkCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	flags.SetOutput(stderr)
	engine := flags.String("engine", "", "borg, restic, or turbo")
	fixture := flags.String("fixture", "all", "all, tiny, mixed, raw, qcow2, or metadata")
	workDir := flags.String("work-dir", ".weazlback-bench", "writable benchmark filesystem")
	output := flags.String("output", "", "write JSON report")
	repository := flags.String("repository", "", "optional remote repository base URL")
	sshKey := flags.String("ssh-key", "", "SSH private key for remote benchmark")
	knownHosts := flags.String("known-hosts", "", "pinned OpenSSH known_hosts file")
	trials := flags.Int("trials", 1, "restore trials; use 3 or more for median dyno results")
	connections := flags.Int("connections", 4, "repository connections for Turbo trials")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *engine == "" {
		return errors.New("--engine is required")
	}
	report, err := benchmark.Run(ctx, benchmark.Options{
		Engine: *engine, Fixture: *fixture, WorkDir: *workDir, Output: *output,
		Repository: *repository, SSHKey: *sshKey, KnownHosts: *knownHosts, Trials: *trials, Connections: *connections,
	}, func(message string) { fmt.Fprintln(stderr, message) })
	if err != nil {
		return err
	}
	return writeJSON(stdout, report)
}

func doctor(stdout io.Writer) error {
	type check struct {
		Name, Value string
		Ready       bool
	}
	checks := []check{{Name: "go", Value: toolVersion("go", "version")},
		{Name: "borg", Value: toolVersion("borg", "--version")},
		{Name: "restic", Value: toolVersion("restic", "version")},
		{Name: "tmux", Value: toolVersion("tmux", "-V")},
		{Name: "jq", Value: toolVersion("jq", "--version")},
		{Name: "qemu-img", Value: toolVersion("qemu-img", "--version")},
		{Name: "omarchy", Value: toolVersion("omarchy", "version")}}
	for i := range checks {
		checks[i].Ready = checks[i].Value != "missing"
	}
	for _, path := range []string{"/mnt/weazlback"} {
		checks = append(checks, check{Name: path, Value: writable(path), Ready: writable(path) == "writable"})
	}
	return writeJSON(stdout, checks)
}

func toolVersion(name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "missing"
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		return "error: " + err.Error()
	}
	for i, b := range out {
		if b == '\n' {
			out = out[:i]
			break
		}
	}
	return string(out)
}

func writable(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "unavailable"
	}
	test := filepath.Join(path, fmt.Sprintf(".weazlback-write-test-%d", time.Now().UnixNano()))
	file, err := os.OpenFile(test, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "not writable"
	}
	file.Close()
	os.Remove(test)
	return "writable"
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

const usage = `Weazlback — sovereign Omarchy backup

Usage:
  weazlback                         open Bubble Tea TUI
  weazlback init [options]          create vault and encrypted repository
  weazlback backup [options]        create a deduplicated snapshot
  weazlback tune [options]          tune or override connections and SSH upload ceiling
  weazlback list [options]          list snapshots
  weazlback identity [list|adopt]   inspect or adopt machine histories
  weazlback check [options]         verify repository indexes or data
  weazlback prune [options]         preview configured retention (--apply confirms)
weazlback recovery export|verify|prepare|refresh manage recovery media
weazlback rotate repository-key                 rotate repository encryption key
  weazlback restore [options]       restore a snapshot into a staging path
  weazlback files [--snapshot ID --query TEXT] browse paths in a restore point
  weazlback status [--json]         read local operation state
  weazlback doctor                  inspect dependencies
  weazlback inventory [--output P]  capture metadata-only inventory
  weazlback heavy inspect [--root P] inspect disks and refuse live writable data
  weazlback applications [--output P] capture application restore manifest
  weazlback packages refresh [options] capture encrypted package artifacts
  weazlback packages schedule --days N configure independent refresh reminders
  weazlback browser repair [--apply] inspect or clear validated stale browser locks
  weazlback benchmark --engine E [--fixture F] [--work-dir D] [--output P]
	[--repository URL --ssh-key KEY --known-hosts FILE]
  weazlback status [--json]
  weazlback version
`
