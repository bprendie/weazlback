package restic

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSFTPAlwaysPinsHostKeyAndKeepsSecretsOutOfArguments(t *testing.T) {
	if _, err := exec.LookPath("ssh-agent"); err != nil {
		t.Skip("ssh-agent unavailable")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath).CombinedOutput(); err != nil {
		t.Skipf("ssh-keygen unavailable: %v: %s", err, output)
	}
	key, _ := os.ReadFile(keyPath)
	knownHosts := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAAfixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "restic")
	script := "#!/bin/sh\nprintf '%s' \"$*\" > '" + argsPath + "'\ncat \"$RESTIC_PASSWORD_FILE\" >/dev/null\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	secret := "not-in-argv"
	_, err := runner.Run(context.Background(), Repository{Location: "sftp:user@host:/repo", Password: []byte(secret), SSHKey: key, KnownHosts: knownHosts}, "snapshots")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(argsPath)
	text := string(args)
	for _, required := range []string{"StrictHostKeyChecking=yes", "UserKnownHostsFile=" + knownHosts, "sftp.connections=4"} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing %q in %s", required, text)
		}
	}
	if strings.Contains(text, secret) {
		t.Fatalf("repository password leaked into argv: %s", text)
	}
}

func TestSFTPLossIsReturnedWithoutCredentialFallback(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "restic")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncat \"$RESTIC_PASSWORD_FILE\" >/dev/null\necho 'connection reset by peer' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := New(nil)
	runner.Binary = binary
	_, err := runner.Run(context.Background(), Repository{Location: "sftp:user@host:/repo", Password: []byte("x")}, "check")
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err=%v", err)
	}
}
