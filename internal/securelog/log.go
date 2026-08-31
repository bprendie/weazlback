package securelog

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Cryptor interface{ Encrypt([]byte) ([]byte, error) }

func Write(cryptor Cryptor, kind, id string, payload []byte) (string, error) {
	root := os.Getenv("WEAZLBACK_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".weazlback")
	}
	dir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sealed, err := cryptor.Encrypt(payload)
	if err != nil {
		return "", err
	}
	cleanID := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, id)
	path := filepath.Join(dir, kind+"-"+cleanID+".wzlog")
	tmp, err := os.CreateTemp(dir, ".secure-log-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(sealed)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(name, path); err != nil {
		return "", err
	}
	prune(dir, 50)
	return path, nil
}

func prune(dir string, retain int) {
	entries, _ := filepath.Glob(filepath.Join(dir, "*.wzlog"))
	sort.Slice(entries, func(i, j int) bool {
		left, _ := os.Stat(entries[i])
		right, _ := os.Stat(entries[j])
		if left == nil || right == nil {
			return entries[i] > entries[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, path := range entries[min(retain, len(entries)):] {
		_ = os.Remove(path)
	}
}

func ID() string { return time.Now().UTC().Format("20060102T150405.000000000") }
