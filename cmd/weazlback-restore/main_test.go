package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledResticWorksWithoutInstalledBinary(t *testing.T) {
	dir := t.TempDir()
	kit := filepath.Join(dir, "weazlback-recovery.wzrk")
	if err := os.WriteFile(kit, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "restic")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n[ \"$1\" = version ]\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, ok := bundledRestic(kit); !ok || got != binary {
		t.Fatalf("bundled=%q ok=%v", got, ok)
	}
	if err := os.Chmod(binary, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := bundledRestic(kit); ok {
		t.Fatal("non-executable bundled Restic was accepted")
	}
}
