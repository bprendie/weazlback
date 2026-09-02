package restic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupStreamsMachineProgress(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s\\n' '{\"message_type\":\"status\",\"percent_done\":0.5,\"files_done\":4,\"total_files\":8,\"bytes_done\":10,\"total_bytes\":20,\"seconds_elapsed\":2,\"seconds_remaining\":2}' '{\"message_type\":\"summary\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	service := Service{Runner: runner}
	var got BackupProgress
	err := service.BackupWithProgress(context.Background(), Repository{Location: dir, Password: []byte("x")}, "core", []string{dir}, nil, false, false, func(progress BackupProgress) {
		if progress.MessageType == "status" {
			got = progress
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PercentDone != .5 || got.FilesDone != 4 || got.SecondsRemaining != 2 {
		t.Fatalf("progress=%#v", got)
	}
}

func TestRestoreStreamsStatusAndSummary(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >\"" + argsPath + "\"\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s\\n' '{\"message_type\":\"status\",\"files_restored\":4,\"total_files\":8,\"bytes_restored\":10,\"total_bytes\":20}' '{\"message_type\":\"summary\",\"files_restored\":8,\"total_files\":8,\"bytes_restored\":20,\"total_bytes\":20}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	var events []RestoreProgress
	err := (Service{Runner: runner}).RestoreWithProgress(context.Background(), Repository{Location: dir, Password: []byte("x")}, "latest", filepath.Join(dir, "out"), nil, func(value RestoreProgress) {
		events = append(events, value)
	})
	if err != nil || len(events) != 2 || events[0].FilesRestored != 4 || events[1].BytesRestored != 20 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil || !strings.Contains(string(args), "--sparse") {
		t.Fatalf("restore args=%q err=%v", args, err)
	}
}

func TestBackupUsesScanAndOnlyEmitsStatusProgress(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"" + argsPath + "\"\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s\\n' '{\"message_type\":\"status\",\"percent_done\":0.5}' '{\"message_type\":\"summary\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	count := 0
	err := (Service{Runner: runner}).BackupWithProgress(context.Background(), Repository{Location: dir, Password: []byte("x")}, "core", []string{dir}, nil, false, false, func(progress BackupProgress) {
		count++
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "--no-scan") {
		t.Fatalf("backup disabled progress scan: %s", args)
	}
	if count != 1 {
		t.Fatalf("progress callbacks=%d, want only one status event", count)
	}
}

func TestBackupAlwaysSweepsStaleLockAfterChildExits(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	marker := filepath.Join(dir, "unlock-called")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\ncase \"$*\" in\n  *unlock*) touch \"" + marker + "\"; exit 0;;\n  *backup*) exit 7;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	err := (Service{Runner: runner}).Backup(context.Background(), Repository{Location: dir, Password: []byte("x")}, "heavy", []string{dir}, nil, false)
	if err == nil {
		t.Fatal("failed backup was reported successful")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatal("post-backup stale-lock sweep did not run")
	}
}

func TestBackupAndRetentionAreMachineScoped(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"" + argsPath + "\"\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	service := Service{Runner: runner}
	repo := Repository{Location: dir, Password: []byte("x")}
	machine := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := service.BackupMachineWithProgress(context.Background(), repo, "home", machine, []string{dir}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := service.PruneMachineProfile(context.Background(), repo, machine, "home", 1, 2, 3, 4); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsPath)
	text := string(args)
	if !strings.Contains(text, "--tag machine:"+machine) || !strings.Contains(text, "--tag weazlback,profile:home,machine:"+machine) {
		t.Fatalf("machine scope missing from restic args:\n%s", text)
	}
}

func TestFilesParsesRestorePointNodes(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s\\n' '{\"struct_type\":\"snapshot\",\"id\":\"abc\"}' '{\"struct_type\":\"node\",\"name\":\"keys.txt\",\"type\":\"file\",\"path\":\"/home/test/keys.txt\",\"size\":12,\"mode\":384}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	files, err := (Service{Runner: runner}).Files(context.Background(), Repository{Location: dir, Password: []byte("x")}, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "/home/test/keys.txt" || files[0].Mode != 0o600 {
		t.Fatalf("files=%#v", files)
	}
}

func TestHeavyRetentionSelectsOnlyHeavyTag(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >\"" + argsPath + "\"\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	err := (Service{Runner: runner}).PruneProfile(context.Background(), Repository{Location: dir, Password: []byte("x")}, "heavy", 0, 7, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsPath)
	if !strings.Contains(string(args), "--tag weazlback,profile:heavy") || !strings.Contains(string(args), "--keep-daily 7") {
		t.Fatalf("retention args=%s", args)
	}
}

func TestCheckUnlocksStaleRepositoryLockAndRetries(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	marker := filepath.Join(dir, "unlocked")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\ncase \"$*\" in\n  *unlock*) touch \"" + marker + "\";;\n  *check*) [ -f \"" + marker + "\" ] || { echo 'unable to create lock in backend: repository is already locked' >&2; exit 11; };;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	if err := (Service{Runner: runner}).Check(context.Background(), Repository{Location: dir, Password: []byte("x")}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("stale unlock was not attempted")
	}
}

func TestCheckDoesNotUnlockUnrelatedFailure(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	marker := filepath.Join(dir, "unlock-was-called")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\ncase \"$*\" in *unlock*) touch \"" + marker + "\";; esac\necho 'authentication failed' >&2\nexit 1\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	if err := (Service{Runner: runner}).Check(context.Background(), Repository{Location: dir, Password: []byte("x")}, false); err == nil {
		t.Fatal("unrelated check failure was ignored")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("unlock was attempted for a non-lock failure")
	}
}

func TestUnlockStaleUsesSafeResticDefault(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\ncat >/dev/null < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s' \"$*\" > \"" + argsPath + "\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	if err := (Service{Runner: runner}).UnlockStale(context.Background(), Repository{Location: dir, Password: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsPath)
	if !strings.HasSuffix(string(args), " unlock") || strings.Contains(string(args), "--remove-all") {
		t.Fatalf("unsafe unlock invocation: %q", args)
	}
}
