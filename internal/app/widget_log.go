package app

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bprendie/weazlback/internal/vault"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(value)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.b.Bytes()...)
}

func writeEncryptedOperationLog(v *vault.File, id string, plain []byte) error {
	if len(plain) == 0 {
		plain = []byte("operation completed without engine diagnostics\n")
	}
	ciphertext, err := v.Encrypt(plain)
	if err != nil {
		return err
	}
	root := os.Getenv("WEAZLBACK_HOME")
	if root == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return homeErr
		}
		root = filepath.Join(home, ".weazlback")
	}
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, id+".wzlog")
	tmp, err := os.CreateTemp(dir, ".log-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(ciphertext); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return retainWidgetLogs(dir, 50)
}

func retainWidgetLogs(dir string, keep int) error {
	entries, _ := filepath.Glob(filepath.Join(dir, "*.wzlog"))
	sort.Slice(entries, func(i, j int) bool {
		left, leftErr := os.Stat(entries[i])
		right, rightErr := os.Stat(entries[j])
		if leftErr != nil || rightErr != nil {
			return entries[i] < entries[j]
		}
		return left.ModTime().Before(right.ModTime())
	})
	for len(entries) > keep {
		if err := os.Remove(entries[0]); err != nil {
			return err
		}
		entries = entries[1:]
	}
	return nil
}
