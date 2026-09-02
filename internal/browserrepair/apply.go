package browserrepair

import (
	"errors"
	"os"
	"path/filepath"
)

func Apply(options Options, plan Plan) Result {
	if options.Lstat == nil {
		options.Lstat = os.Lstat
	}
	if options.Remove == nil {
		options.Remove = os.Remove
	}
	result := summarizeSkips(plan)
	for _, entry := range plan.Entries {
		if entry.Action != Remove {
			continue
		}
		if err := revalidate(options, entry); err != nil {
			result.Failed++
			continue
		}
		if err := options.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Failed++
			continue
		}
		result.Removed++
	}
	return result
}

func revalidate(options Options, entry Entry) error {
	if err := secureRoot(options, entry.Root); err != nil {
		return err
	}
	if filepath.Dir(entry.Path) != entry.Root || !exactLock(entry.Family, filepath.Base(entry.Path)) {
		return errors.New("invalid transient path")
	}
	info, err := options.Lstat(entry.Path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("transient entry is a directory")
	}
	if options.Processes != nil && options.Processes.Running(entry.Family, options.UID) {
		return errors.New("browser is running")
	}
	if entry.Family == Chromium && (!regular(filepath.Join(entry.Root, "Local State"), options.Lstat) || !chromiumProfile(entry.Root, options.Lstat)) {
		return errors.New("markers changed")
	}
	if entry.Family == Mozilla && !mozillaProfile(entry.Root, options.Lstat) {
		return errors.New("markers changed")
	}
	return nil
}

func exactLock(family Family, name string) bool {
	names := chromiumLocks
	if family == Mozilla {
		names = mozillaLocks
	}
	for _, allowed := range names {
		if name == allowed {
			return true
		}
	}
	return false
}

func summarizeSkips(plan Plan) Result {
	var result Result
	for _, entry := range plan.Entries {
		switch entry.Action {
		case SkipLive:
			result.Live++
		case SkipAmbiguous:
			result.Ambiguous++
		case SkipBoundary:
			result.Boundary++
		}
	}
	return result
}

func Exclusions(options Options) []string {
	plan := Detect(options)
	var paths []string
	for _, entry := range plan.Entries {
		if entry.Action == Remove || entry.Action == SkipLive {
			paths = append(paths, entry.Path)
		}
	}
	return paths
}
