package tui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bprendie/weazlback/internal/catalog"
	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRealSearchVersionBasketStagingJourney(t *testing.T) {
	for _, transport := range []string{"local", "sftp-protocol"} {
		t.Run(transport, func(t *testing.T) { realRestoreJourney(t, transport) })
	}
}

func realRestoreJourney(t *testing.T, transport string) {
	if testing.Short() {
		t.Skip("real repository journey")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic unavailable")
	}
	root := t.TempDir()
	t.Setenv("WEAZLBACK_HOME", filepath.Join(root, "private"))
	source := filepath.Join(root, "source")
	batch := filepath.Join(source, "batch")
	if err := os.MkdirAll(batch, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if err := os.WriteFile(filepath.Join(batch, fmt.Sprintf("item-%04d", i)), []byte("batch data\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(source, "needle-history.txt")
	old := []byte("older authenticated contents\n")
	if err := os.WriteFile(file, old, 0600); err != nil {
		t.Fatal(err)
	}
	repo := restic.Repository{Location: filepath.Join(root, "repo"), Password: []byte("fixture")}
	if transport == "sftp-protocol" {
		server := "/usr/lib/ssh/sftp-server"
		if _, err := os.Stat(server); err != nil {
			t.Skip("local SFTP server unavailable")
		}
		bin := filepath.Join(root, "bin")
		if err := os.Mkdir(bin, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nexec "+server+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
		repo.Location = "sftp:fixture@localhost:" + repo.Location
	}
	ctx := context.Background()
	service := restic.NewService(io.Discard)
	if err := service.Initialize(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if err := service.BackupMachineWithProgress(ctx, repo, "home", "alice", []string{source}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	points, err := service.Snapshots(ctx, repo)
	if err != nil || len(points) != 1 {
		t.Fatalf("points: %v", err)
	}
	oldPoint := points[0]
	oldFiles, err := service.Files(ctx, repo, oldPoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	newer := []byte("newer live contents must remain untouched\n")
	if err := os.WriteFile(file, newer, 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.BackupMachineWithProgress(ctx, repo, "home", "alice", []string{source}, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	points, err = service.Snapshots(ctx, repo)
	if err != nil || len(points) != 2 {
		t.Fatalf("points: %v", err)
	}
	var newest restic.Snapshot
	for _, p := range points {
		if p.ID != oldPoint.ID {
			newest = p
		}
	}
	files, err := service.Files(ctx, repo, newest.ID)
	if err != nil {
		t.Fatal(err)
	}
	v := vault.New(filepath.Join(root, "vault"))
	if err := v.Create([]byte("x")); err != nil {
		t.Fatal(err)
	}
	defer v.Lock()
	if err := v.Put("repo", repo.Password); err != nil {
		t.Fatal(err)
	}
	m := restoreFixture()
	m.vault = v
	m.cfg = config.Config{Machine: config.Machine{ID: "alice"}, ActiveDestination: "fixture", Destinations: []config.Destination{{ID: "fixture", Repository: repo.Location, PasswordKey: "repo"}}}
	m.snapshots = []restic.Snapshot{newest, oldPoint}
	m.restoreEntries = files
	m.restoreTreePath = source
	m.buildRestoreTree()
	m.restoreCatalog = catalog.New()
	m.restoreCatalog.Baseline(oldPoint, oldFiles)
	m.restoreCatalog.Baseline(newest, files)
	m.restoreSearching = true
	m.restoreInput.SetValue("needle-history")
	next, cmd := m.queueRestoreSearch()
	m = next.(Model)
	next, _, _ = m.updateRestoreMessage(cmd())
	m = next.(Model)
	m = press(m, tea.KeyEnter, "")
	if m.restoreStage != "versions" {
		t.Fatal("search failed to open versions")
	}
	m = press(m, tea.KeyDown, "")
	m = press(m, tea.KeyRunes, " ")
	if m.restoreBasket[file].Snapshot != oldPoint.ID {
		t.Fatal("historical point not pinned")
	}
	// Include a large folder in the same batch; one confirmation covers its files.
	m.restoreStage = "browse"
	m.restoreSearching = false
	m.restoreSnapshot = 1
	m.restoreTreePath = source
	m.restoreEntries = oldFiles
	m.buildRestoreTree()
	m.filterRestoreTree()
	m.selectTreePath(batch)
	m = press(m, tea.KeyRunes, " ")
	m = press(m, tea.KeyRunes, "e")
	m = press(m, tea.KeyEnter, "")
	if m.restoreStage != "transaction-target" {
		t.Fatal("basket destination not reachable")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("preflight missing: %s", m.err)
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.restoreStage != "transaction-preview" {
		t.Fatalf("preview: %s", m.err)
	}
	m = press(m, tea.KeyEnter, "")
	m = press(m, tea.KeyRunes, "RESTORE")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(Model)
	}
	if m.err != "" || m.restoreStage != "transaction-result" {
		t.Fatalf("restore: %s (%s)", m.err, m.restoreStage)
	}
	restored, err := os.ReadFile(filepath.Join(m.restoreResult.StagedAt, file))
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(restored) != sha256.Sum256(old) {
		t.Fatal("historical bytes differ")
	}
	live, _ := os.ReadFile(file)
	if string(live) != string(newer) {
		t.Fatal("staging changed live source")
	}
	entries, err := os.ReadDir(filepath.Join(m.restoreResult.StagedAt, batch))
	if err != nil || len(entries) != 1000 {
		t.Fatalf("batch count=%d err=%v", len(entries), err)
	}
}
