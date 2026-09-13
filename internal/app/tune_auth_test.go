package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/vault"
)

func TestTuneAuthorizesBeforeRepositoryAccess(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WEAZLBACK_HOME", root)
	t.Setenv("WEAZLBACK_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("WEAZLBACK_TEST_PASSPHRASE", "test-only")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Reject authentication and record arguments without invoking real sudo.
	log := filepath.Join(root, "sudo-args")
	t.Setenv("TUNE_AUTH_LOG", log)
	if err := os.WriteFile(filepath.Join(root, "sudo"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$TUNE_AUTH_LOG\"\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	path, err := vault.Path("test")
	if err != nil {
		t.Fatal(err)
	}
	v := vault.New(path)
	if err := v.Create([]byte("test-only")); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("repo", []byte("test-repository-secret")); err != nil {
		t.Fatal(err)
	}
	v.Lock()
	cfg := config.Default()
	cfg.ActiveVault = "test"
	cfg.Destinations = []config.Destination{{ID: "local", Kind: "local", Repository: filepath.Join(root, "repo"), PasswordKey: "repo", Privileged: true}}
	cfg.ActiveDestination = "local"
	if err := config.Save(os.Getenv("WEAZLBACK_CONFIG"), cfg); err != nil {
		t.Fatal(err)
	}
	err = tuneCommand(context.Background(), nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "sudo authorization failed") {
		t.Fatalf("expected authorization failure before Restic, got %v", err)
	}
	args, err := os.ReadFile(log)
	if err != nil || string(args) != "-v\n" {
		t.Fatalf("sudo args=%q err=%v", args, err)
	}
	if err := tuneCommand(context.Background(), []string{"--connections", "10"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("manual settings should not require sudo: %v", err)
	}
	args, err = os.ReadFile(log)
	if err != nil || string(args) != "-v\n" {
		t.Fatalf("manual settings invoked sudo: %q, %v", args, err)
	}
}
