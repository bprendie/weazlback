package targetpackages

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/packagecapsule"
)

type downloadRunner struct {
	t        *testing.T
	root     string
	plan     string
	download func() error
	calls    int
}

func (r *downloadRunner) Output(name string, args ...string) (string, error) {
	r.t.Helper()
	joined := strings.Join(args, " ")
	if name != "pacman" || !strings.Contains(joined, "--dbpath "+filepath.Join(r.root, "db")) || !strings.Contains(joined, "--arch aarch64") || !strings.Contains(joined, "-Sp --noconfirm --print-format %f %s -- editor browser") {
		r.t.Fatalf("unsafe plan command: %s %s", name, joined)
	}
	return r.plan, nil
}
func (r *downloadRunner) Run(name string, args ...string) error {
	r.t.Helper()
	r.calls++
	joined := strings.Join(args, " ")
	if name != "sudo" || !strings.Contains(joined, "--dbpath "+filepath.Join(r.root, "db")) || !strings.Contains(joined, "--cachedir "+filepath.Join(r.root, "cache")) || !strings.HasSuffix(joined, "-Sw --noconfirm --noprogressbar -- editor browser") {
		r.t.Fatalf("unsafe download command: %s %s", name, joined)
	}
	return r.download()
}
func (r *downloadRunner) RunDir(string, string, ...string) error {
	r.t.Fatal("unexpected build")
	return nil
}

func TestParallelDownloadConfigPreservesTargetPolicy(t *testing.T) {
	config := "[options]\nArchitecture = aarch64\nParallelDownloads = 5\nIgnorePkg = hyprland\nSigLevel = Required DatabaseOptional\n[core]\nServer = http://fixture.invalid/$arch/$repo\n"
	actual := downloadConfig(config)
	if strings.Count(actual, "ParallelDownloads") != 1 || !strings.Contains(actual, "ParallelDownloads = 3") || !strings.Contains(actual, "IgnorePkg = hyprland") || !strings.Contains(actual, "SigLevel = Required") {
		t.Fatal(actual)
	}
	if _, err := exec.LookPath("pacman-conf"); err == nil {
		path := filepath.Join(t.TempDir(), "pacman.conf")
		if err := os.WriteFile(path, []byte(actual), 0600); err != nil {
			t.Fatal(err)
		}
		value, err := exec.Command("pacman-conf", "--config", path, "ParallelDownloads").CombinedOutput()
		if err != nil || strings.TrimSpace(string(value)) != "3" {
			t.Fatalf("pacman did not accept three streams: %s %v", value, err)
		}
	}
}

func TestBatchDownloadReportsPartialAndCompletedFiles(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cache, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("dependency-1-1-aarch64.pkg.tar.zst", "cached")
	release := make(chan struct{})
	run := &downloadRunner{t: t, root: root, plan: "editor-1-1-aarch64.pkg.tar.zst 8\ndependency-1-1-aarch64.pkg.tar.zst 6\ndependency-1-1-aarch64.pkg.tar.zst 6\n"}
	run.download = func() error {
		write("editor-1-1-aarch64.pkg.tar.zst.part", "half")
		select {
		case <-release:
		case <-time.After(3 * time.Second):
			return errors.New("progress observer stalled")
		}
		write("editor-1-1-aarch64.pkg.tar.zst", "complete")
		return nil
	}
	var last packagecapsule.Progress
	sawPartial := false
	err := downloadOfficial(context.Background(), IsolatedRunner{Base: run, Root: root, Environment: Environment{Architecture: "aarch64"}}, []string{"editor", "browser"}, func(p packagecapsule.Progress) {
		last = p
		if p.Bytes == 10 && p.Completed == 1 && !sawPartial {
			sawPartial = true
			close(release)
		}
	})
	if err != nil || run.calls != 1 || !sawPartial || last.Completed != 2 || last.Total != 2 || last.Bytes != 14 || last.BytesTotal != 14 {
		t.Fatalf("err=%v calls=%d partial=%v progress=%+v", err, run.calls, sawPartial, last)
	}
}

func TestBatchDownloadFailureNeverClaimsCompletion(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("mirror failure")
	run := &downloadRunner{t: t, root: root, plan: "editor-1-1-aarch64.pkg.tar.zst 8", download: func() error { return failure }}
	var last packagecapsule.Progress
	err := downloadOfficial(context.Background(), IsolatedRunner{Base: run, Root: root, Environment: Environment{Architecture: "aarch64"}}, []string{"editor", "browser"}, func(p packagecapsule.Progress) { last = p })
	if !errors.Is(err, failure) || last.Completed != 0 || last.Bytes != 0 || run.calls != 1 {
		t.Fatalf("%v %+v", err, last)
	}
}

func TestBatchDownloadCancellationWaitsForPacman(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := false
	run := &downloadRunner{t: t, root: t.TempDir(), plan: "editor-1-1-aarch64.pkg.tar.zst 8"}
	run.download = func() error { cancel(); <-ctx.Done(); stopped = true; return ctx.Err() }
	err := downloadOfficial(ctx, IsolatedRunner{Base: run, Root: run.root, Environment: Environment{Architecture: "aarch64"}}, []string{"editor", "browser"}, nil)
	if !errors.Is(err, context.Canceled) || !stopped {
		t.Fatalf("returned before cancelled pacman stopped: %v", err)
	}
}

func TestDownloadPlanRejectsUnsafePathsAndSizes(t *testing.T) {
	for _, value := range []string{"../escape.pkg.tar.zst 8", "/tmp/escape.pkg.tar.zst 8", "editor.pkg.tar.zst -1", "editor.pkg.tar.zst nope", ""} {
		if _, err := parseDownloadFiles(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

type splittingRunner struct {
	t         *testing.T
	root      string
	downloads [][]string
}

func (r *splittingRunner) Output(name string, args ...string) (string, error) {
	index := 0
	for i, arg := range args {
		if arg == "--" {
			index = i + 1
			break
		}
	}
	names := args[index:]
	for _, pkg := range names {
		if pkg == "broken" {
			return ":: unavailable dependency required by broken", errors.New("cannot satisfy dependencies")
		}
	}
	var lines []string
	for _, pkg := range names {
		lines = append(lines, pkg+"-1-1-aarch64.pkg.tar.zst 8")
	}
	lines = append(lines, "shared-1-1-aarch64.pkg.tar.zst 8")
	return strings.Join(lines, "\n"), nil
}
func (r *splittingRunner) Run(name string, args ...string) error {
	index := 0
	for i, arg := range args {
		if arg == "--" {
			index = i + 1
			break
		}
	}
	names := append([]string(nil), args[index:]...)
	r.downloads = append(r.downloads, names)
	for _, pkg := range append(names, "shared") {
		if pkg == "broken" {
			r.t.Fatal("attempted unsatisfiable download")
		}
		if err := os.WriteFile(filepath.Join(r.root, "cache", pkg+"-1-1-aarch64.pkg.tar.zst"), []byte("complete"), 0600); err != nil {
			return err
		}
	}
	return nil
}
func (r *splittingRunner) RunDir(string, string, ...string) error {
	r.t.Fatal("unexpected build")
	return nil
}

func TestUnsatisfiableApplicationDoesNotBlockOtherDownloads(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	base := &splittingRunner{t: t, root: root}
	run := IsolatedRunner{Base: base, Root: root, Environment: Environment{Architecture: "aarch64"}}
	var last packagecapsule.Progress
	err := downloadOfficial(context.Background(), run, []string{"alpha", "broken", "beta", "gamma"}, func(p packagecapsule.Progress) { last = p })
	if err == nil || !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "unavailable dependency") {
		t.Fatalf("missing failure details: %v", err)
	}
	if len(base.downloads) != 2 || strings.Join(base.downloads[0], ",") != "alpha" || strings.Join(base.downloads[1], ",") != "beta,gamma" {
		t.Fatalf("groups: %v", base.downloads)
	}
	if last.Completed != 4 || last.Total != 4 || last.Bytes != 32 {
		t.Fatalf("shared dependencies not aggregated correctly: %+v", last)
	}
}
