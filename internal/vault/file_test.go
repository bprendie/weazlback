package vault

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const v050CompatibilityPassphrase = "weazlback-v050-compatibility"

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

func TestV050Argon2CompatibilityVector(t *testing.T) {
	salt := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	want, err := hex.DecodeString("37996bf7841a5b14043080b2f05b48a9d724bc387283160ffb7eb2272829a75c")
	if err != nil {
		t.Fatal(err)
	}
	if got := derive([]byte(v050CompatibilityPassphrase), salt); !bytes.Equal(got, want) {
		t.Fatalf("Argon2id output changed: got %x", got)
	}
}

func TestUnlocksV050Vault(t *testing.T) {
	v := New(filepath.Join("testdata", "v050.vault"))
	if err := v.Unlock([]byte(v050CompatibilityPassphrase)); err != nil {
		t.Fatalf("unlock pre-upgrade vault: %v", err)
	}
	defer v.Lock()
	value, err := v.Get("fixture/value")
	if err != nil || string(value) != "synthetic-v050-secret" {
		t.Fatalf("fixture value=%q err=%v", value, err)
	}
}
