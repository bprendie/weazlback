package packagecapsule

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var safePackageName = regexp.MustCompile(`^[a-zA-Z0-9@._+:-]+$`)

func downloadOfficial(options Options, root string, manifest *Manifest) {
	roots := cacheRoots(root)
	var missing []string
	for _, pkg := range manifest.Packages {
		if pkg.Source == "official" && findArtifact(roots, pkg.Name, pkg.Installed) == "" {
			missing = append(missing, pkg.Name)
		}
	}
	if len(missing) == 0 {
		return
	}
	emit(options, Progress{Phase: "download", Package: missing[0], Total: len(missing)})
	destination := filepath.Join(root, "download-cache")
	_ = os.MkdirAll(destination, 0o700)
	args := []string{"-n", "pacman", "-Sw", "--noconfirm", "--cachedir", destination, "--"}
	args = append(args, missing...)
	if err := options.Run.Run("sudo", args...); err != nil {
		manifest.Exceptions = append(manifest.Exceptions, Exception{Code: "official-download-failed", Detail: err.Error()})
	}
}

func buildForeign(options Options, root string, manifest *Manifest) {
	roots := cacheRoots(root)
	buildRoot := filepath.Join(root, "aur-build")
	_ = os.MkdirAll(buildRoot, 0o700)
	for _, pkg := range manifest.Packages {
		if pkg.Source != "foreign" || findArtifact(roots, pkg.Name, pkg.Installed) != "" {
			continue
		}
		if !safePackageName.MatchString(pkg.Name) {
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "unsafe-package-name", Detail: "refused AUR URL construction"})
			continue
		}
		emit(options, Progress{Phase: "build", Package: pkg.Name, Total: manifest.Summary.Foreign})
		directory := filepath.Join(buildRoot, pkg.Name)
		if err := options.Run.Run("git", "clone", "--depth", "1", "https://aur.archlinux.org/"+pkg.Name+".git", directory); err != nil {
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "aur-clone-failed", Detail: err.Error()})
			continue
		}
		if err := options.Run.RunDir(directory, "makepkg", "--syncdeps", "--noconfirm", "--cleanbuild", "--clean"); err != nil {
			manifest.Exceptions = append(manifest.Exceptions, Exception{Package: pkg.Name, Code: "aur-build-failed", Detail: err.Error()})
		}
	}
}

func cacheRoots(root string) []string {
	home, _ := os.UserHomeDir()
	return []string{filepath.Join(root, "download-cache"), filepath.Join(root, "aur-build"),
		"/var/cache/pacman/pkg", filepath.Join(home, ".cache", "paru", "clone"), filepath.Join(home, ".cache", "yay")}
}

func emit(options Options, progress Progress) {
	if options.Progress != nil {
		options.Progress(progress)
	}
}

func acquisitionHint(pkg Package) string {
	if pkg.Source == "foreign" {
		return "refresh with --build-missing-aur after reviewing the PKGBUILD"
	}
	return "refresh with --download-missing while sudo authorization is active"
}

func compactError(err error) string {
	return strings.TrimSpace(fmt.Sprint(err))
}
