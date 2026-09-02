package browserrepair

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func Detect(options Options) Plan {
	if options.Lstat == nil {
		options.Lstat = os.Lstat
	}
	if options.Processes == nil {
		options.Processes = ProcFS{}
	}
	if options.ConfigHome == "" {
		currentHome, _ := os.UserHomeDir()
		if filepath.Clean(options.Home) == filepath.Clean(currentHome) {
			options.ConfigHome, _ = os.UserConfigDir()
		}
	}
	plan := Plan{}
	for _, spec := range roots {
		for _, root := range candidateRoots(options, spec) {
			if _, err := options.Lstat(root); err != nil {
				continue
			}
			if err := secureRoot(options, root); err != nil {
				plan.Entries = append(plan.Entries, Entry{Family: spec.Family, Action: SkipBoundary})
				continue
			}
			if spec.Family == Chromium {
				planChromium(&plan, options, root)
			} else {
				planMozilla(&plan, options, root)
			}
		}
	}
	return plan
}

func candidateRoots(options Options, spec rootSpec) []string {
	result := []string{filepath.Join(options.Home, filepath.FromSlash(spec.Path))}
	if options.ConfigHome != "" && strings.HasPrefix(spec.Path, ".config/") {
		custom := filepath.Join(options.ConfigHome, filepath.FromSlash(strings.TrimPrefix(spec.Path, ".config/")))
		if filepath.Clean(custom) != filepath.Clean(result[0]) {
			result = append(result, custom)
		}
	}
	return result
}

func planChromium(plan *Plan, options Options, root string) {
	if !regular(filepath.Join(root, "Local State"), options.Lstat) || !chromiumProfile(root, options.Lstat) {
		plan.Entries = append(plan.Entries, Entry{Family: Chromium, Action: SkipAmbiguous})
		return
	}
	live := options.Processes.Running(Chromium, options.UID)
	for _, name := range chromiumLocks {
		path := filepath.Join(root, name)
		if _, err := options.Lstat(path); err == nil {
			action := Remove
			if live {
				action = SkipLive
			}
			plan.Entries = append(plan.Entries, Entry{Family: Chromium, Action: action, Root: root, Path: path})
		}
	}
}

func chromiumProfile(root string, lstat func(string) (os.FileInfo, error)) bool {
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		name := entry.Name()
		if name == "Default" || name == "Guest Profile" || strings.HasPrefix(name, "Profile ") {
			if info, err := lstat(filepath.Join(root, name)); err == nil && info.IsDir() {
				return true
			}
		}
	}
	return false
}

func planMozilla(plan *Plan, options Options, root string) {
	profiles := declaredProfiles(root)
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if entry.IsDir() {
			profiles = appendUnique(profiles, filepath.Join(root, entry.Name()))
		}
	}
	for _, profile := range profiles {
		if err := secureRoot(options, profile); err != nil {
			plan.Entries = append(plan.Entries, Entry{Family: Mozilla, Action: SkipBoundary})
			continue
		}
		if !mozillaProfile(profile, options.Lstat) {
			continue
		}
		live := options.Processes.Running(Mozilla, options.UID)
		for _, name := range mozillaLocks {
			path := filepath.Join(profile, name)
			if _, err := options.Lstat(path); err == nil {
				action := Remove
				if live {
					action = SkipLive
				}
				plan.Entries = append(plan.Entries, Entry{Family: Mozilla, Action: action, Root: profile, Path: path})
			}
		}
	}
}

func declaredProfiles(root string) []string {
	file, err := os.Open(filepath.Join(root, "profiles.ini"))
	if err != nil {
		return nil
	}
	defer file.Close()
	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Path=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "Path="))
		if value == "" || filepath.IsAbs(value) {
			continue
		}
		clean := filepath.Clean(value)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			continue
		}
		paths = appendUnique(paths, filepath.Join(root, clean))
	}
	return paths
}

func mozillaProfile(path string, lstat func(string) (os.FileInfo, error)) bool {
	if !regular(filepath.Join(path, "prefs.js"), lstat) {
		return false
	}
	for _, marker := range []string{"compatibility.ini", "times.json"} {
		if regular(filepath.Join(path, marker), lstat) {
			return true
		}
	}
	return false
}

func regular(path string, lstat func(string) (os.FileInfo, error)) bool {
	info, err := lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func secureRoot(options Options, candidate string) error {
	home, err := filepath.Abs(options.Home)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("outside home")
	}
	current := home
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, statErr := options.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlinked parent")
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && options.UID >= 0 && int(stat.Uid) != options.UID {
			return errors.New("wrong owner " + strconv.Itoa(int(stat.Uid)))
		}
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	for _, old := range values {
		if old == value {
			return values
		}
	}
	return append(values, value)
}
