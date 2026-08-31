package restoretxn

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

type bundleCryptor struct{ key byte }

func (c bundleCryptor) Encrypt(value []byte) ([]byte, error) {
	out := append([]byte(nil), value...)
	for i := range out {
		out[i] ^= c.key
	}
	return out, nil
}
func (c bundleCryptor) Decrypt(value []byte) ([]byte, error) { return c.Encrypt(value) }

func TestComposeNearestIsMachineScopedAndDeterministic(t *testing.T) {
	wanted := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	snapshots := []restic.Snapshot{
		{ID: "earlier", Time: wanted.Add(-time.Hour), Tags: []string{"machine:thinkpad", "profile:home"}},
		{ID: "later", Time: wanted.Add(time.Hour), Tags: []string{"machine:thinkpad", "profile:home"}},
		{ID: "hp", Time: wanted, Tags: []string{"machine:hp", "profile:home"}},
	}
	components, err := ComposeNearest(snapshots, "thinkpad", wanted, map[Bundle]string{PersonalFiles: "home"})
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Snapshot.ID != "earlier" {
		t.Fatalf("components=%#v", components)
	}
}

func TestExactDeletionsCannotEscapeAndListsOnlyAbsentPaths(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(target, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(target, "keep", "same.txt"), filepath.Join(target, "later.txt")} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository := []string{"/home/source", "/home/source/keep", "/home/source/keep/same.txt"}
	deletions, err := ExactDeletions(target, "/home/source", target, repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletions) != 1 || deletions[0] != filepath.Join(target, "later.txt") {
		t.Fatalf("deletions=%v", deletions)
	}
	if _, err := ExactDeletions(root, "/home/source", target, repository); err == nil {
		t.Fatal("boundary escape was accepted")
	}
}

func TestBundleJournalIsEncryptedAndRetainsComposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.enc")
	journal := BundleJournal{OperationID: "op", Mode: "exact", State: "running", Components: []Component{{MachineID: "thinkpad"}}, Deletions: []string{"/home/bob/private.txt"}}
	if err := SaveBundleJournal(path, bundleCryptor{0x5a}, journal); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private.txt")) || bytes.Contains(raw, []byte("thinkpad")) {
		t.Fatal("bundle journal leaked sensitive metadata")
	}
	plain, _ := bundleCryptor{0x5a}.Decrypt(raw)
	if !bytes.Contains(plain, []byte("private.txt")) || !bytes.Contains(plain, []byte("thinkpad")) {
		t.Fatal("bundle journal lost composition")
	}
}

func FuzzExactRewindNeverEscapesBoundary(f *testing.F) {
	f.Add("later.txt")
	f.Add("nested/file.txt")
	f.Fuzz(func(t *testing.T, relative string) {
		if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
			t.Skip()
		}
		root := t.TempDir()
		target := filepath.Join(root, "boundary")
		path := filepath.Join(target, relative)
		if !strings.HasPrefix(path, target+string(filepath.Separator)) {
			t.Skip()
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Skip()
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Skip()
		}
		deletions, err := ExactDeletions(target, "/source", target, []string{"/source"})
		if err != nil {
			t.Fatal(err)
		}
		for _, deletion := range deletions {
			if deletion != target && !strings.HasPrefix(deletion, target+string(filepath.Separator)) {
				t.Fatalf("escaped deletion %q", deletion)
			}
		}
	})
}
