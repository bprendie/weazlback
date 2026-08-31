package inventory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func Capture(ctx context.Context) (Report, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Report{}, err
	}
	hostname, _ := os.Hostname()
	report := Report{
		SchemaVersion: 1,
		CapturedAt:    time.Now().UTC(),
		Hostname:      hostname,
		Architecture:  runtime.GOARCH,
		Home:          home,
	}
	report.Omarchy = firstLine(run(ctx, "omarchy", "version"))
	report.HomeEntries = diskUsage(ctx, home)
	report.ConfigEntries = diskUsage(ctx, filepath.Join(home, ".config"))
	report.LargeFiles = findLargeFiles(filepath.Join(home, "containers"), 1<<30)
	report.Packages = packages(ctx)
	report.Services = services(ctx)
	report.PkgInstall = relativeFiles(filepath.Join(home, "pkginstall"))
	classify(report.HomeEntries)
	classifyConfig(report.ConfigEntries)
	return report, nil
}

func Write(path string, report Report) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func diskUsage(ctx context.Context, root string) []PathEntry {
	out := run(ctx, "du", "-x", "-b", "--max-depth=1", root)
	var entries []PathEntry
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) != 2 || fields[1] == root {
			continue
		}
		bytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err == nil {
			entries = append(entries, PathEntry{Path: fields[1], Bytes: bytes})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Bytes > entries[j].Bytes })
	return entries
}

func findLargeFiles(root string, minimum int64) []LargeFile {
	var files []LargeFile
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil || info.Size() < minimum {
			return err
		}
		allocated := info.Size()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			allocated = stat.Blocks * 512
		}
		format := "file"
		if strings.HasSuffix(strings.ToLower(path), ".img") {
			format = "raw-candidate"
		} else if strings.HasSuffix(strings.ToLower(path), ".qcow2") {
			format = "qcow2"
		}
		ratio := fmt.Sprintf("%.1f%%", float64(allocated)/float64(info.Size())*100)
		files = append(files, LargeFile{Path: path, Format: format, Logical: info.Size(), Allocated: allocated, SparseRatio: ratio})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Logical > files[j].Logical })
	return files
}

func run(ctx context.Context, name string, args ...string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstLine(value string) string {
	line, _, _ := strings.Cut(value, "\n")
	return strings.TrimSpace(line)
}
