package freshrestore

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
)

func TestCrossFilesystemFallbackPreservesTreeMetadata(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "secret")
	if err := os.WriteFile(file, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("secret", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyAcrossFilesystems(source, target); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target, "secret"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	if link, err := os.Readlink(filepath.Join(target, "link")); err != nil || link != "secret" {
		t.Fatalf("link=%q err=%v", link, err)
	}
}

func TestCrossDeviceClassification(t *testing.T) {
	err := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}
	if !isCrossDevice(err) {
		t.Fatal("EXDEV was not classified")
	}
}

func TestMoveOrCopyAcrossRealFilesystems(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot, err := os.MkdirTemp(".", ".cross-device-placement-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(targetRoot) })
	var sourceStat, targetStat syscall.Stat_t
	if err := syscall.Stat(sourceRoot, &sourceStat); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Stat(targetRoot, &targetStat); err != nil {
		t.Fatal(err)
	}
	if sourceStat.Dev == targetStat.Dev {
		t.Skip("test host does not expose separate temporary and workspace filesystems")
	}
	source := filepath.Join(sourceRoot, "tree")
	target := filepath.Join(targetRoot, "tree")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "secret"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := moveOrCopy(source, target); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(filepath.Join(target, "secret")); err != nil || string(payload) != "payload" {
		t.Fatalf("payload=%q err=%v", payload, err)
	}
}

func TestTurboCrossDevicePlacementRecordsStandardFallback(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot, err := os.MkdirTemp(".", ".turbo-cross-device-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(targetRoot) })
	var sourceStat, targetStat syscall.Stat_t
	_ = syscall.Stat(sourceRoot, &sourceStat)
	_ = syscall.Stat(targetRoot, &targetStat)
	if sourceStat.Dev == targetStat.Dev {
		t.Skip("test host has one filesystem")
	}
	source, target := filepath.Join(sourceRoot, "file"), filepath.Join(targetRoot, "file")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Restore{JournalPath: filepath.Join(sourceRoot, "journal.json"), Journal: Journal{Engine: EngineTurbo}}
	if err := r.moveOrFallback(source, target); err != nil {
		t.Fatal(err)
	}
	if r.Journal.Engine != EngineStandard || r.Journal.FallbackPhase != "placement" {
		t.Fatalf("journal=%+v", r.Journal)
	}
}

func TestCompleteJournalWithoutCorePathsRewindsToReusableStage(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "source", ".config")
	staged := stagedPath(filepath.Join(dir, "stage"), original)
	if err := os.MkdirAll(staged, 0o700); err != nil {
		t.Fatal(err)
	}
	r := Restore{StageDir: filepath.Join(dir, "stage"), JournalPath: filepath.Join(dir, "journal.json"),
		Plan:    Plan{OriginalHome: filepath.Dir(original), TargetHome: filepath.Join(dir, "target")},
		Session: &Session{Config: config.Config{Profiles: []config.Profile{{Name: "core", Includes: []string{original}}}}},
		Journal: Journal{Stage: "complete"}}
	if err := r.repairInconsistentJournal(); err != nil {
		t.Fatal(err)
	}
	if r.Journal.Stage != "packages_reconciled" {
		t.Fatalf("stage=%q", r.Journal.Stage)
	}
}

func TestJournalRoundTripIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "journal.json")
	want := Journal{RepositoryID: "opaque", SnapshotID: "abc", Stage: "core_staged"}
	if err := SaveJournal(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadJournal(path)
	if err != nil || got.Stage != want.Stage {
		t.Fatalf("journal=%#v err=%v", got, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode=%o", info.Mode().Perm())
	}
}

func TestSchemaOneJournalMigratesToStandardEngine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"repository_id":"r","snapshot_id":"s","stage":"core_staged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if journal.SchemaVersion != 2 || journal.Engine != EngineStandard {
		t.Fatalf("journal=%+v", journal)
	}
}

func TestMapHomePathRejectsEscape(t *testing.T) {
	if got, err := mapHomePath("/home/old/.config", "/home/old", "/home/new"); err != nil || got != "/home/new/.config" {
		t.Fatalf("mapped=%q err=%v", got, err)
	}
	if _, err := mapHomePath("/etc/shadow", "/home/old", "/home/new"); err == nil {
		t.Fatal("accepted path outside original home")
	}
}

func TestPlacePreservesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "stage", "file"), filepath.Join(dir, "home", "file")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(source, []byte("restored"), 0o600)
	_ = os.WriteFile(target, []byte("fresh"), 0o600)
	if err := placePreserving(source, target); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "restored" {
		t.Fatalf("target=%q", body)
	}
	matches, _ := filepath.Glob(target + ".weazlback-before-*")
	if len(matches) != 1 {
		t.Fatalf("recoverable copies=%v", matches)
	}
}

func TestJournaledPlacementResumesAfterMove(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "home", "file")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Restore{JournalPath: filepath.Join(dir, "journal.json"), Journal: Journal{
		RepositoryID: "repo", SnapshotID: "snapshot", Stage: "core_staged",
		PendingSource: filepath.Join(dir, "missing-stage"), PendingTarget: target, PendingBackup: target + ".before",
	}}
	if err := r.placeJournaled(r.Journal.PendingSource, target); err != nil {
		t.Fatal(err)
	}
	if !contains(r.Journal.CommittedPaths, target) || r.Journal.PendingTarget != "" {
		t.Fatalf("journal did not finish pending move: %#v", r.Journal)
	}
}

func TestResolveHostnameModes(t *testing.T) {
	if got, err := ResolveHostname("original", "dragonfly"); err != nil || got != "dragonfly" {
		t.Fatalf("hostname=%q err=%v", got, err)
	}
	if _, err := ResolveHostname("bad_name", "dragonfly"); err == nil {
		t.Fatal("accepted invalid hostname")
	}
}

func TestHostnameFilesReplaceOnlyManagedEntry(t *testing.T) {
	dir := t.TempDir()
	hostnamePath, hostsPath := filepath.Join(dir, "hostname"), filepath.Join(dir, "hosts")
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n127.0.1.1 old-name\n10.0.0.2 server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeHostnameFiles("dragonfly", hostnamePath, hostsPath); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.ReadFile(hostnamePath)
	hosts, _ := os.ReadFile(hostsPath)
	if string(hostname) != "dragonfly\n" || !strings.Contains(string(hosts), "127.0.1.1\tdragonfly") || !strings.Contains(string(hosts), "10.0.0.2 server") {
		t.Fatalf("hostname=%q hosts=%q", hostname, hosts)
	}
}
