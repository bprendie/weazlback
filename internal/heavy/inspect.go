package heavy

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type Image struct {
	Path      string `json:"path"`
	Format    string `json:"format"`
	Logical   int64  `json:"logical_bytes"`
	Allocated int64  `json:"allocated_bytes"`
}

type Writer struct {
	Path    string `json:"path"`
	PID     int    `json:"pid"`
	Process string `json:"process"`
}

type Report struct {
	Roots   []string `json:"roots"`
	Images  []Image  `json:"images"`
	Writers []Writer `json:"live_writers,omitempty"`
	Safe    bool     `json:"safe"`
}

func Inspect(roots []string) Report {
	report := Report{Roots: append([]string(nil), roots...), Safe: true}
	for _, root := range roots {
		report.Images = append(report.Images, images(root)...)
		report.Writers = append(report.Writers, writers(root)...)
	}
	report.Writers = uniqueWriters(report.Writers)
	report.Safe = len(report.Writers) == 0
	sort.Slice(report.Images, func(i, j int) bool { return report.Images[i].Logical > report.Images[j].Logical })
	return report
}

func images(root string) []Image {
	var result []Image
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !isDisk(path) {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		allocated := info.Size()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			allocated = stat.Blocks * 512
		}
		result = append(result, Image{Path: path, Format: diskFormat(path), Logical: info.Size(), Allocated: allocated})
		return nil
	})
	return result
}

func writers(root string) []Writer {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	processes, _ := filepath.Glob("/proc/[0-9]*")
	var result []Writer
	for _, process := range processes {
		pid, _ := strconv.Atoi(filepath.Base(process))
		fds, _ := filepath.Glob(filepath.Join(process, "fd", "*"))
		for _, fd := range fds {
			path, err := os.Readlink(fd)
			if err != nil || !inside(root, path) || !writableFD(process, filepath.Base(fd)) {
				continue
			}
			result = append(result, Writer{Path: path, PID: pid, Process: processName(process)})
		}
	}
	return result
}

func writableFD(process, fd string) bool {
	file, err := os.Open(filepath.Join(process, "fdinfo", fd))
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if value, found := strings.CutPrefix(scanner.Text(), "flags:\t"); found {
			flags, err := strconv.ParseUint(strings.TrimSpace(value), 8, 64)
			return err == nil && flags&syscall.O_ACCMODE != syscall.O_RDONLY
		}
	}
	return false
}

func processName(process string) string {
	b, _ := os.ReadFile(filepath.Join(process, "comm"))
	name := strings.TrimSpace(string(b))
	if name == "" {
		return "unknown"
	}
	return name
}

func inside(root, path string) bool {
	path = strings.TrimSuffix(path, " (deleted)")
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isDisk(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".qcow2", ".raw", ".img", ".vdi", ".vhd", ".vhdx":
		return true
	}
	return false
}

func diskFormat(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "img" {
		return "raw-candidate"
	}
	return ext
}

func uniqueWriters(values []Writer) []Writer {
	seen := map[string]bool{}
	var result []Writer
	for _, value := range values {
		key := fmt.Sprintf("%d:%s", value.PID, value.Path)
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	return result
}
