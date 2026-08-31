package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/vault"
)

func TestOperationLogIsEncryptedAndPrivate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WEAZLBACK_HOME", root)
	v := vault.New(filepath.Join(root, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	plain := []byte("/home/alice/private-api-keys.txt")
	if err := writeEncryptedOperationLog(v, "operation", plain); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "logs", "operation.wzlog")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, plain) {
		t.Fatal("operation log exposed a plaintext filename")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log mode=%o", info.Mode().Perm())
	}
}
