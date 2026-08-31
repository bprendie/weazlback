package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateUnlockAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "test.vault")
	v := New(path)
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("repo/password", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	if _, err := v.Get("repo/password"); err != ErrLocked {
		t.Fatalf("Get while locked: %v", err)
	}
	if err := v.Unlock([]byte("wrong")); err == nil {
		t.Fatal("wrong passphrase unlocked vault")
	}
	time.Sleep(1100 * time.Millisecond)
	if err := v.Unlock([]byte("x")); err != nil {
		t.Fatal(err)
	}
	got, err := v.Get("repo/password")
	if err != nil || !bytes.Equal(got, []byte("secret")) {
		t.Fatalf("Get = %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", info.Mode().Perm(), err)
	}
}

func TestAnyNonemptyPassphraseAccepted(t *testing.T) {
	for _, passphrase := range []string{" ", "1", "password"} {
		v := New(filepath.Join(t.TempDir(), "vault"))
		if err := v.Create([]byte(passphrase)); err != nil {
			t.Errorf("passphrase %q rejected: %v", passphrase, err)
		}
	}
}

func TestCiphertextDoesNotContainSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault")
	v := New(path)
	if err := v.Create([]byte("pass")); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("ssh", []byte("PRIVATE-KEY-MARKER")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("PRIVATE-KEY-MARKER")) || bytes.Contains(b, []byte("pass")) {
		t.Fatal("vault leaked plaintext")
	}
}
