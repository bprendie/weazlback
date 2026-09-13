package freshrestore

import (
	"debug/elf"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/architecture"
)

type Migration struct {
	Active             bool     `json:"active"`
	SourceArchitecture string   `json:"source_architecture"`
	TargetArchitecture string   `json:"target_architecture"`
	TargetFingerprint  string   `json:"target_fingerprint"`
	Preserve           []string `json:"preserve"`
	Review             []string `json:"review"`
	ProtectedPackages  []string `json:"protected_packages"`
}

func migrationPolicy(source, target string) Migration {
	if source == "" && architecture.Go(target) == "arm64" {
		source = "unknown"
	}
	m := Migration{Active: source != "" && target != "" && architecture.Go(source) != architecture.Go(target), SourceArchitecture: architecture.Package(source), TargetArchitecture: architecture.Package(target)}
	if !m.Active {
		return m
	}
	m.Preserve = []string{".config/hypr", ".config/omarchy", ".config/uwsm", ".config/systemd", ".local/share/systemd", ".local/share/omarchy", ".local/state/omarchy", ".weazlback", ".rustup", ".cargo/bin", ".local/share/mise", ".local/share/fnm", ".nvm", ".pyenv"}
	m.Review = []string{"Target hardware and session configuration retained; review source device settings before reuse", "Foreign native binaries and reproducible native environments are withheld; reinstall their native equivalents", "VM and container images are restored as data; native execution is not implied"}
	return m
}

// Sanitize only private staging. Preserve destination components inside staged
// parent trees before atomic placement, so a broad Home overlay cannot erase them.
func (r *Restore) migrateStage(root string) error {
	if !r.Plan.Migration.Active {
		return nil
	}
	sourceHome := stagedPath(root, r.Plan.OriginalHome)
	if info, err := os.Lstat(sourceHome); err != nil || !info.IsDir() {
		return fmt.Errorf("migration source home must be a staged directory")
	}
	for _, rel := range r.Plan.Migration.Preserve {
		if err := retainTarget(sourceHome, r.Plan.TargetHome, rel); err != nil {
			return err
		}
	}
	err := filepath.WalkDir(sourceHome, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceHome, path)
		if err != nil {
			return err
		}
		for _, heavy := range r.Plan.HeavyPlacementPaths {
			heavyRel, err := filepath.Rel(r.Plan.TargetHome, heavy)
			if err == nil && (rel == heavyRel || strings.HasPrefix(rel, heavyRel+"/")) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		for _, keep := range r.Plan.Migration.Preserve {
			if rel == keep || strings.HasPrefix(rel, keep+"/") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", ".venv", "venv", "__pycache__":
				r.MigrationWithheld = append(r.MigrationWithheld, rel+": native environment retained from target or needs rebuilding")
				if err := retainTarget(sourceHome, r.Plan.TargetHome, rel); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		file, err := elf.Open(path)
		if err != nil {
			return nil
		}
		machine := file.Machine
		file.Close()
		expected := map[string]elf.Machine{"aarch64": elf.EM_AARCH64, "x86_64": elf.EM_X86_64}[r.Plan.Migration.TargetArchitecture]
		if machine != expected {
			r.MigrationWithheld = append(r.MigrationWithheld, rel+": foreign binary withheld; install a native build")
			return retainTarget(sourceHome, r.Plan.TargetHome, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return r.stageNativeRuntime(sourceHome)
}

func retainTarget(stageHome, targetHome, relative string) error {
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("migration path escapes home")
	}
	// Never follow a staged parent symlink while replacing a protected subtree.
	current := stageHome
	for _, part := range strings.Split(filepath.Dir(relative), string(filepath.Separator)) {
		if part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if info, err := os.Lstat(current); err == nil && !info.IsDir() {
			return fmt.Errorf("migration parent is not a directory: %s", relative)
		}
	}
	staged, target := filepath.Join(stageHome, relative), filepath.Join(targetHome, relative)
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(staged), 0700); err != nil {
		return err
	}
	return copyAcrossFilesystems(target, staged)
}
