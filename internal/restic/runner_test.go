package restic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordUsesDescriptorNotArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "restic")
	body := "#!/bin/sh\ncase \"$*\" in *top-secret*) exit 91;; esac\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\n[ \"$pass\" = top-secret ] || exit 92\nprintf '{\"ok\":true}'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = script
	out, err := runner.Run(context.Background(), Repository{Location: dir, Password: []byte("top-secret")}, "snapshots", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"ok":true`) {
		t.Fatalf("output=%q", out)
	}
}

func TestSFTPConnectionCountIsPassedToRestic(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "restic")
	body := "#!/bin/sh\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s' \"$*\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = script
	out, err := runner.Run(context.Background(), Repository{Location: "sftp:user@host:/repo", Password: []byte("secret"), Connections: 10}, "check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "-o sftp.connections=10") {
		t.Fatalf("args=%q", out)
	}
}

func TestAggregateUploadLimitIsPassedToRestic(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "restic")
	body := "#!/bin/sh\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s' \"$*\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = script
	out, err := runner.Run(context.Background(), Repository{
		Location: "sftp:user@host:/repo", Password: []byte("secret"), Connections: 10, UploadLimitKiB: 80896,
	}, "check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "--limit-upload 80896") {
		t.Fatalf("args=%q", out)
	}
}

func TestDefaultConnectionCountIsFour(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "restic")
	body := "#!/bin/sh\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\nprintf '%s' \"$*\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = script
	out, err := runner.Run(context.Background(), Repository{Location: dir, Password: []byte("secret")}, "check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "-o local.connections=4") {
		t.Fatalf("args=%q", out)
	}
}

func TestElevatedPasswordUsesParentPipe(t *testing.T) {
	dir := t.TempDir()
	resticPath, sudoPath := filepath.Join(dir, "restic"), filepath.Join(dir, "sudo")
	resticBody := "#!/bin/sh\nIFS= read -r pass < \"$RESTIC_PASSWORD_FILE\"\n[ \"$pass\" = root-secret ] || exit 92\nprintf elevated-ok\n"
	sudoBody := "#!/bin/sh\nwhile [ \"$1\" != env ]; do shift; done\nshift\nexec env \"$@\"\n"
	if err := os.WriteFile(resticPath, []byte(resticBody), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sudoPath, []byte(sudoBody), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	runner := New(nil)
	runner.Binary = resticPath
	out, err := runner.Run(context.Background(), Repository{Location: dir, Password: []byte("root-secret"), Elevated: true}, "check")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "elevated-ok" {
		t.Fatalf("output=%q", out)
	}
}
