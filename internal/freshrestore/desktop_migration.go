package freshrestore

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var localHyprRequire = regexp.MustCompile(`^\s*require\(["']hypr\.([A-Za-z0-9_]+)["']\)\s*(?:--.*)?$`)

// Preserve target-only helper modules called by its bindings (for example the
// HP media-key workaround), while restoring the source's personal bindings.
func (r *Restore) mergeTargetDesktopHelpers(stageHome string) error {
	relative := ".config/hypr/bindings.lua"
	sourcePath := filepath.Join(stageHome, relative)
	source, err := os.ReadFile(sourcePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	target, err := os.ReadFile(filepath.Join(r.Plan.TargetHome, relative))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	original := string(source)
	for _, line := range strings.Split(string(target), "\n") {
		match := localHyprRequire.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		module := ".config/hypr/" + match[1] + ".lua"
		if _, err := os.Lstat(filepath.Join(stageHome, module)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if _, err := os.Lstat(filepath.Join(r.Plan.TargetHome, module)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := retainTarget(stageHome, r.Plan.TargetHome, module); err != nil {
			return err
		}
		source = append(source, []byte("\n-- Retained target hardware helper.\nrequire(\"hypr."+match[1]+"\")\n")...)
		r.Journal.PreservedPaths = appendUniqueStrings(r.Journal.PreservedPaths, module)
	}
	if string(source) != original {
		if err := os.MkdirAll(filepath.Dir(sourcePath), 0700); err != nil {
			return err
		}
		// Never write through a symlink supplied by the source snapshot.
		if info, err := os.Lstat(sourcePath); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("staged bindings must be a regular file")
		}
		if err := os.WriteFile(sourcePath, source, 0600); err != nil {
			return err
		}
	}
	// Keep plugins installed only on the target; source plugins retain precedence.
	plugins := ".config/omarchy/plugins"
	entries, err := os.ReadDir(filepath.Join(r.Plan.TargetHome, plugins))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		rel := filepath.Join(plugins, entry.Name())
		if _, err := os.Lstat(filepath.Join(stageHome, rel)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := retainTarget(stageHome, r.Plan.TargetHome, rel); err != nil {
			return err
		}
		r.Journal.PreservedPaths = appendUniqueStrings(r.Journal.PreservedPaths, rel)
	}
	return nil
}
