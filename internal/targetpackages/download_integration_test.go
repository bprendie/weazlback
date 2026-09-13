package targetpackages

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/packagecapsule"
)

// This test uses a temporary unsigned fixture repository and fakeroot, never sudo
// or a configured system/recovery repository. Production signature policy is untouched.
type fixturePacmanRunner struct{ packagecapsule.ExecRunner }

func (r fixturePacmanRunner) Run(name string, args ...string) error {
	if name == "sudo" {
		name = "fakeroot"
	}
	return r.ExecRunner.Run(name, args...)
}

func TestPacmanActuallyDownloadsThreePackagesConcurrently(t *testing.T) {
	if os.Getenv("WEAZLBACK_TEST_PACMAN_PARALLEL") != "1" {
		t.Skip("set WEAZLBACK_TEST_PACMAN_PARALLEL=1 for isolated real-pacman HTTP test")
	}
	for _, tool := range []string{"pacman", "fakeroot", "repo-add"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	repository := filepath.Join(root, "fixture-repo")
	if err := os.Mkdir(repository, 0700); err != nil {
		t.Fatal(err)
	}
	names := []string{"parallel-one", "parallel-two", "parallel-three", "parallel-four"}
	var archives []string
	for _, name := range names {
		path := filepath.Join(repository, name+"-1-1-aarch64.pkg.tar.gz")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		info := fmt.Sprintf("pkgname = %s\npkgbase = %s\npkgver = 1-1\npkgdesc = isolated download fixture\nurl = https://fixture.invalid\nbuilddate = 1\npackager = Test\nsize = 7\narch = aarch64\n", name, name)
		for filename, data := range map[string]string{".PKGINFO": info, "usr/share/" + name: "fixture"} {
			if err := tw.WriteHeader(&tar.Header{Name: filename, Mode: 0644, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		archives = append(archives, path)
	}
	command := exec.CommandContext(ctx, "repo-add", append([]string{filepath.Join(repository, "fixture.db.tar.gz")}, archives...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture repo: %s %v", output, err)
	}
	var active, maximum atomic.Int32
	ready := make(chan struct{})
	var release sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pkg.tar.gz") {
			n := active.Add(1)
			for old := maximum.Load(); n > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, n) {
					break
				}
			}
			if n == 3 {
				release.Do(func() { close(ready) })
			}
			select {
			case <-ready:
			case <-ctx.Done():
				active.Add(-1)
				http.Error(w, "concurrent download barrier timed out", 500)
				return
			}
			active.Add(-1)
		}
		http.ServeFile(w, r, filepath.Join(repository, filepath.Base(r.URL.Path)))
	}))
	defer server.Close()
	work := filepath.Join(root, "working")
	if err := os.MkdirAll(filepath.Join(work, "keyring"), 0700); err != nil {
		t.Fatal(err)
	}
	runner := IsolatedRunner{Base: fixturePacmanRunner{packagecapsule.ExecRunner{Context: ctx, Quiet: true}}, Root: work, Environment: Environment{Architecture: "aarch64", Config: "[options]\nSigLevel = Never\nParallelDownloads = 1\n[fixture]\nServer = " + server.URL + "\n"}}
	if err := runner.Initialize(); err != nil {
		t.Fatal(err)
	}
	var last packagecapsule.Progress
	if err := downloadOfficial(ctx, runner, names, func(p packagecapsule.Progress) { last = p }); err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 3 || last.Completed != 4 || last.Total != 4 {
		t.Fatalf("max streams=%d progress=%+v", maximum.Load(), last)
	}
	t.Log("Verified three simultaneous HTTP package downloads with real pacman")
}
