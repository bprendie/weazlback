package browserrepair

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type ProcFS struct{ Root string }

func (p ProcFS) Running(family Family, uid int) bool {
	root := p.Root
	if root == "" {
		root = "/proc"
	}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != uid {
			continue
		}
		exe, _ := os.Readlink(filepath.Join(dir, "exe"))
		cmd, _ := os.ReadFile(filepath.Join(dir, "cmdline"))
		if matchesProcess(family, filepath.Base(exe), cmd) {
			return true
		}
	}
	return false
}

func matchesProcess(family Family, exe string, command []byte) bool {
	allowed := chromiumProcesses
	if family == Mozilla {
		allowed = mozillaProcesses
	}
	first := exe
	if first == "" {
		first = filepath.Base(strings.Split(string(bytes.ReplaceAll(command, []byte{0}, []byte{' '})), " ")[0])
	}
	first = strings.ToLower(first)
	for _, name := range allowed {
		if first == name {
			return true
		}
	}
	return false
}

var chromiumProcesses = []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "brave", "brave-browser", "microsoft-edge", "vivaldi", "opera", "thorium-browser", "slimjet", "yandex-browser"}
var mozillaProcesses = []string{"firefox", "firefox-esr", "librewolf", "waterfox", "floorp", "zen", "mullvad-browser", "tor-browser"}
