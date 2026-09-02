package weazlbridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAuthenticatedLocalRestoreLeavesRepositoryObjectsUnchanged(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic is not installed")
	}
	root := t.TempDir()
	repository, source, target := filepath.Join(root, "repo"), filepath.Join(root, "source"), filepath.Join(root, "restore")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "payload"), []byte("authenticated payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRestic(t, repository, "init")
	runRestic(t, repository, "backup", source)
	var snapshots []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(runRestic(t, repository, "snapshots", "--json", "--latest", "1"), &snapshots); err != nil || len(snapshots) != 1 {
		t.Fatalf("snapshots=%v err=%v", snapshots, err)
	}
	before := repositoryProof(t, repository)
	var final Progress
	count, err := Restore(context.Background(), Options{Repository: repository, Password: "bridge-test", Snapshot: snapshots[0].ID,
		Target: target, Connections: 4, Progress: func(value Progress) { final = value }})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 || final.FilesDone == 0 || final.WireBytes == 0 {
		t.Fatalf("count=%d progress=%+v", count, final)
	}
	if body, err := os.ReadFile(filepath.Join(target, source, "payload")); err != nil || string(body) != "authenticated payload" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	after := repositoryProof(t, repository)
	if len(before) != len(after) {
		t.Fatalf("repository objects changed: before=%d after=%d", len(before), len(after))
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatalf("repository object changed: %s", path)
		}
	}
}

func TestCorruptPackNeverProducesValidPlaintext(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic is not installed")
	}
	root := t.TempDir()
	repository, source, target := filepath.Join(root, "repo"), filepath.Join(root, "source"), filepath.Join(root, "restore")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("authenticated payload that must never survive a corrupt pack")
	if err := os.WriteFile(filepath.Join(source, "payload"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	runRestic(t, repository, "init")
	runRestic(t, repository, "backup", source)
	var snapshots []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(runRestic(t, repository, "snapshots", "--json", "--latest", "1"), &snapshots); err != nil {
		t.Fatal(err)
	}
	var pack string
	_ = filepath.WalkDir(filepath.Join(repository, "data"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && pack == "" {
			pack = path
		}
		return err
	})
	data, err := os.ReadFile(pack)
	if err != nil || len(data) < 64 {
		t.Fatalf("pack=%s err=%v", pack, err)
	}
	data[len(data)/2] ^= 0xff
	if err := os.Chmod(pack, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pack, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Restore(context.Background(), Options{Repository: repository, Password: "bridge-test", Snapshot: snapshots[0].ID, Target: target})
	if err == nil {
		t.Fatal("corrupt pack was accepted")
	}
	restored, readErr := os.ReadFile(filepath.Join(target, source, "payload"))
	if readErr == nil && string(restored) == string(payload) {
		t.Fatal("corrupt pack emitted valid plaintext")
	}
}

func runRestic(t *testing.T, repository string, args ...string) []byte {
	t.Helper()
	command := exec.Command("restic", append([]string{"-r", repository}, args...)...)
	command.Env = append(os.Environ(), "RESTIC_PASSWORD=bridge-test")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("restic %v: %v: %s", args, err, out)
	}
	return out
}

func repositoryProof(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()
	proof := map[string][sha256.Size]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.Dir(rel) == "locks" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		proof[rel] = sha256.Sum256(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return proof
}
