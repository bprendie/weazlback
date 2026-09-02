package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/freshrestore"
	"github.com/bprendie/weazlback/internal/recovery"
	"github.com/bprendie/weazlback/internal/recoverytui"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--internal-set-hostname" {
		if err := freshrestore.SetHostnamePrivileged(os.Args[2]); err != nil {
			fatal(err)
		}
		return
	}
	if len(os.Args) == 1 {
		kit := filepath.Join(filepath.Dir(os.Args[0]), "weazlback-recovery.wzrk")
		if _, err := os.Stat(filepath.Join(filepath.Dir(os.Args[0]), "SHA256SUMS")); err == nil {
			if err := recovery.VerifyMediaDirectory(filepath.Dir(os.Args[0])); err != nil {
				fatal(err)
			}
		}
		if err := ensureRestic(kit); err != nil {
			fatal(err)
		}
		if err := recoverytui.Run(); err != nil {
			fatal(err)
		}
		return
	}
	recovery := flag.String("recovery", "", "portable .wzrk recovery vault")
	destination := flag.String("destination", "", "repository destination ID embedded in the recovery kit")
	hostname := flag.String("hostname", "original", "original, current, or a custom hostname")
	snapshot := flag.String("snapshot", "latest", "Core Restore Point ID or latest")
	scope := flag.String("scope", "core", "recovery scope: core, home, everything, or applications")
	targetHome := flag.String("target-home", "", "fresh-system home directory")
	workDir := flag.String("work-dir", "", "private resumable restore workspace")
	planOnly := flag.Bool("plan-only", false, "verify and print the plan without changing the system")
	stageOnly := flag.Bool("stage-only", false, "extract and validate privately without changing the system")
	yes := flag.Bool("yes", false, "accept the displayed restore plan")
	jsonOutput := flag.Bool("json", false, "write the final report as JSON")
	connections := flag.Int("connections", 0, "parallel repository connections (0 auto-starts at 4; try 2/4/10)")
	repository := flag.String("repository", "", "override the local repository mount path")
	adoptLocal := flag.Bool("adopt-local-repository", false, "use sudo to adopt the exact local repository for this user")
	adoptIdentity := flag.Bool("adopt-source-identity", false, "make this replacement system continue the selected source machine identity")
	nukeRepository := flag.Bool("nuke-repository", false, "break glass: destroy this kit's repository and keys")
	nukeKeysOnly := flag.Bool("nuke-keys-only", false, "with --nuke-repository, destroy keys but preserve repository ciphertext")
	nukeRemoveDirectory := flag.Bool("nuke-remove-local-directory", false, "with --nuke-repository, remove the exact local repository directory")
	flag.Parse()
	if *recovery == "" {
		fmt.Fprintln(os.Stderr, "weazlback-restore: --recovery is required")
		flag.PrintDefaults()
		os.Exit(2)
	}
	passphrase, err := readPassphrase()
	if err != nil {
		fatal(err)
	}
	defer zero(passphrase)
	if *nukeRepository {
		if err := recoveryNuke(*recovery, passphrase, *nukeKeysOnly, *nukeRemoveDirectory); err != nil {
			fatal(err)
		}
		return
	}
	if err := ensureRestic(*recovery); err != nil {
		fatal(err)
	}
	restore, err := freshrestore.Prepare(context.Background(), freshrestore.Options{
		RecoveryPath: *recovery, Destination: *destination, Passphrase: passphrase, Snapshot: *snapshot, Hostname: *hostname, Scope: *scope,
		TargetHome: *targetHome, WorkDir: *workDir, Yes: *yes, DryRun: *planOnly, Connections: *connections,
		Repository: *repository, AdoptLocal: *adoptLocal, AdoptSourceIdentity: *adoptIdentity,
	})
	if err != nil {
		fatal(err)
	}
	defer restore.Close()
	fmt.Println("\n" + freshrestore.PlanText(restore.Plan))
	if len(restore.Plan.Applications.ManualReview) > 0 {
		fmt.Printf("Manual review  %d items\n", len(restore.Plan.Applications.ManualReview))
	}
	if *planOnly {
		fmt.Println("\nPlan verified. No system changes were made.")
		return
	}
	if *stageOnly {
		path, err := restore.StagePreview(context.Background())
		if err != nil {
			fatal(err)
		}
		fmt.Printf("\nRestore Point extracted and validated at %s. No system changes were made.\n", path)
		return
	}
	if !*yes && !confirm() {
		fatal(fmt.Errorf("restore cancelled; no system changes were made"))
	}
	report, err := restore.Execute(context.Background(), os.Stdout)
	if err != nil {
		fatal(err)
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fatal(err)
		}
	} else {
		status := "complete"
		if !report.Complete {
			status = "incomplete"
		}
		fmt.Printf("\nRestore %s: %d paths placed; %d browser locks removed; %d exceptions.\n", status, len(report.RestoredPaths), report.BrowserRepair.Removed, len(report.PackageErrors)+len(report.BrowserIssues))
		fmt.Printf("Journal: %s\n", report.JournalPath)
	}
}

func recoveryNuke(kitPath string, passphrase []byte, keysOnly, removeDirectory bool) error {
	bundle, err := recovery.Open(kitPath, passphrase)
	if err != nil {
		return err
	}
	defer bundle.Close()
	var cfg config.Config
	if err := json.Unmarshal(bundle.Config, &cfg); err != nil {
		return fmt.Errorf("recovery configuration: %w", err)
	}
	destination := cfg.Active()
	if destination == nil {
		return fmt.Errorf("recovery kit contains no repository")
	}
	fmt.Printf("\nBREAK GLASS — cryptographic destruction and repository deletion\nRepository  %s\nLocation    %s\n", destination.ID, destination.Repository)
	mode := "full"
	if keysOnly {
		mode = "keys"
		fmt.Println("Mode        cryptographic key destruction; ciphertext retained")
	} else {
		fmt.Println("Mode        repository deletion plus cryptographic key destruction")
	}
	fmt.Printf("\nType NUKE %s: ", destination.ID)
	confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(confirmation) != "NUKE "+destination.ID {
		return fmt.Errorf("break-glass confirmation did not match; nothing changed")
	}
	temporary, err := os.MkdirTemp("", "weazlback-recovery-nuke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := os.MkdirAll(filepath.Join(temporary, "vaults"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temporary, "vaults", cfg.ActiveVault+".vault"), bundle.Vault, 0o600); err != nil {
		return err
	}
	if len(bundle.KnownHosts) > 0 {
		knownHosts := filepath.Join(temporary, "known_hosts")
		if err := os.WriteFile(knownHosts, bundle.KnownHosts, 0o600); err != nil {
			return err
		}
		for i := range cfg.Destinations {
			if cfg.Destinations[i].SSHKnownHosts != "" {
				cfg.Destinations[i].SSHKnownHosts = knownHosts
			}
		}
	}
	if err := config.Save(filepath.Join(temporary, "config.json"), cfg); err != nil {
		return err
	}
	binary := filepath.Join(filepath.Dir(os.Args[0]), "weazlback")
	args := []string{"internal-nuke", "--destination", destination.ID, "--mode", mode}
	if removeDirectory {
		args = append(args, "--remove-directory")
	}
	command := exec.Command(binary, args...)
	command.Env = append(os.Environ(), "WEAZLBACK_HOME="+temporary)
	secretInput := append(append([]byte{}, passphrase...), '\n')
	secretInput = append(secretInput, []byte("NUKE "+destination.ID+"\n")...)
	defer zero(secretInput)
	command.Stdin = bytes.NewReader(secretInput)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func ensureRestic(recoveryPath string) error {
	if _, err := exec.LookPath("restic"); err == nil {
		return nil
	}
	if bundled, ok := bundledRestic(recoveryPath); ok {
		return os.Setenv("PATH", filepath.Dir(bundled)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	fmt.Fprintln(os.Stderr, "Restic is required but not installed.")
	fmt.Fprint(os.Stderr, "Install it from the Omarchy/Arch repositories now? [y/N] ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		return fmt.Errorf("restic is required; installation declined")
	}
	cmd := exec.Command("sudo", "pacman", "-S", "--needed", "--noconfirm", "restic")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func bundledRestic(recoveryPath string) (string, bool) {
	bundled := filepath.Join(filepath.Dir(recoveryPath), "restic")
	info, err := os.Stat(bundled)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", false
	}
	return bundled, exec.Command(bundled, "version").Run() == nil
}

func readPassphrase() ([]byte, error) {
	if value := os.Getenv("WEAZLBACK_TEST_PASSPHRASE"); value != "" {
		return []byte(value), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("vault passphrase requires an interactive terminal")
	}
	fmt.Fprint(os.Stderr, "Vault passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil || len(passphrase) == 0 {
		return nil, fmt.Errorf("passphrase must not be empty")
	}
	return passphrase, nil
}

func confirm() bool {
	fmt.Fprint(os.Stderr, "\nType RESTORE to apply the displayed recovery scope: ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(answer) == "RESTORE"
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "weazlback-restore:", err); os.Exit(1) }
func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
