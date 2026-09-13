package targetpackages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/packagecapsule"
)

const ARMParallelDownloads = 3

// Concurrency is an operational setting in the isolated working config, not a
// change to the exported target descriptor or the host's pacman configuration.
func downloadConfig(config string) string {
	var lines []string
	for _, line := range strings.Split(config, "\n") {
		key, _, _ := strings.Cut(line, "=")
		if strings.TrimSpace(key) == "ParallelDownloads" {
			continue
		}
		lines = append(lines, line)
		if strings.TrimSpace(line) == "[options]" {
			lines = append(lines, fmt.Sprintf("ParallelDownloads = %d", ARMParallelDownloads))
		}
	}
	return strings.Join(lines, "\n")
}

type downloadFile struct {
	name string
	size int64
}

func parseDownloadFiles(output string) ([]downloadFile, error) {
	var files []downloadFile
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid package download plan: %q", line)
		}
		name := parts[0]
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || !strings.Contains(name, ".pkg.tar.") {
			return nil, fmt.Errorf("invalid package filename in download plan: %q", name)
		}
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid package size in download plan: %q", line)
		}
		if !seen[name] {
			files = append(files, downloadFile{name, size})
			seen[name] = true
		}
	}
	return files, nil
}

// Pacman owns one dependency transaction and its lock; libalpm runs the three
// download streams. Starting several pacman processes against one DB/cache races
// their locks and shared dependency downloads.
func downloadOfficial(ctx context.Context, run IsolatedRunner, names []string, progress func(packagecapsule.Progress)) error {
	if len(names) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if progress != nil {
		progress(packagecapsule.Progress{Phase: "download", Source: "official", Package: "planning package and dependency downloads"})
	}
	output, err := run.Output("pacman", append([]string{"-Sp", "--noconfirm", "--print-format", "%f %s", "--"}, names...)...)
	if err != nil {
		return fmt.Errorf("plan ARM package downloads: %w", err)
	}
	files, err := parseDownloadFiles(output)
	if err != nil {
		return err
	}
	cache := filepath.Join(run.Root, "cache")
	previous := downloadProgress(cache, files)
	previousTime := time.Now()
	report := func() {
		if progress == nil {
			return
		}
		now := time.Now()
		value := downloadProgress(cache, files)
		if elapsed := now.Sub(previousTime).Seconds(); elapsed > 0 && value.Bytes > previous.Bytes {
			value.Rate = float64(value.Bytes-previous.Bytes) / elapsed
		}
		progress(value)
		previous, previousTime = value, now
	}
	report()
	done := make(chan error, 1)
	go func() {
		done <- run.Run("pacman", append([]string{"-Sw", "--noconfirm", "--noprogressbar", "--"}, names...)...)
	}()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			report()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		case <-ticker.C:
			report()
		case <-ctx.Done():
			// ExecRunner uses the same context. Wait for pacman to stop before capture
			// or cleanup can use files it might still be writing.
			<-done
			return ctx.Err()
		}
	}
}

func downloadProgress(cache string, files []downloadFile) packagecapsule.Progress {
	p := packagecapsule.Progress{Phase: "download", Source: "official", Total: len(files)}
	var active []string
	for _, file := range files {
		p.BytesTotal += file.size
		path := filepath.Join(cache, file.name)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Size() == file.size {
			p.Completed++
			p.Bytes += info.Size()
			continue
		}
		if info, err := os.Lstat(path + ".part"); err == nil && info.Mode().IsRegular() {
			n := info.Size()
			if n > file.size {
				n = file.size
			}
			p.Bytes += n
			if len(active) < ARMParallelDownloads {
				active = append(active, file.name)
			}
		}
	}
	p.Package = fmt.Sprintf("%d download streams", ARMParallelDownloads)
	if len(active) > 0 {
		p.Package += " • " + strings.Join(active, ", ")
	} else if p.Completed == p.Total {
		p.Package = "downloads present; checking package integrity"
	}
	return p
}
