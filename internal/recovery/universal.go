package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Each generation is immutable. CURRENT is replaced only after full verification.
// Interrupted refreshes leave the previous generation available.
func PrepareUniversal(target, bundle, kit, version string) error {
	if _, err := ValidateBundle(bundle, version); err != nil {
		return err
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return fmt.Errorf("recovery target must be an existing directory")
	}
	generations := filepath.Join(target, "payloads")
	if err := os.MkdirAll(generations, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(generations, "kit-")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	names := []string{BundleManifestName, "weazlback-recovery.wzrk", "RESTORE.txt", "THIRD_PARTY_NOTICES.txt"}
	for _, arch := range BundleArchitectures {
		if err := os.MkdirAll(filepath.Join(stage, "linux-"+arch), 0755); err != nil {
			return err
		}
		for _, tool := range BundleTools {
			name := "linux-" + arch + "/" + tool
			if _, err := copyVerified(filepath.Join(stage, name), filepath.Join(bundle, name), 0755); err != nil {
				return err
			}
			names = append(names, name)
		}
	}
	for _, name := range BundleRuntime {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(stage, name)), 0755); err != nil {
			return err
		}
		if _, err := copyVerified(filepath.Join(stage, name), filepath.Join(bundle, name), 0644); err != nil {
			return err
		}
		names = append(names, name)
	}
	if _, err := copyVerified(filepath.Join(stage, BundleManifestName), filepath.Join(bundle, BundleManifestName), 0644); err != nil {
		return err
	}
	if _, err := copyVerified(filepath.Join(stage, "weazlback-recovery.wzrk"), kit, 0644); err != nil {
		return err
	}
	if err := atomicBytes(filepath.Join(stage, "RESTORE.txt"), []byte(UniversalInstructions), 0644); err != nil {
		return err
	}
	if err := atomicBytes(filepath.Join(stage, "THIRD_PARTY_NOTICES.txt"), []byte(ThirdPartyNotices), 0644); err != nil {
		return err
	}
	var checks []string
	for _, name := range names {
		sum, err := fileSHA256(filepath.Join(stage, name))
		if err != nil {
			return err
		}
		checks = append(checks, sum+"  "+name)
	}
	if err := atomicBytes(filepath.Join(stage, "SHA256SUMS"), []byte(strings.Join(checks, "\n")+"\n"), 0644); err != nil {
		return err
	}
	if err := VerifyMediaDirectory(stage); err != nil {
		return err
	}
	for _, tool := range []string{"weazlback", "weazlback-restore", "weazlback-turbo-restore"} {
		if err := atomicBytes(filepath.Join(target, tool), []byte(universalLauncher), 0755); err != nil {
			return err
		}
	}
	if err := atomicBytes(filepath.Join(target, "RESTORE.txt"), []byte(UniversalInstructions), 0644); err != nil {
		return err
	}
	if err := os.Chmod(stage, 0755); err != nil {
		return err
	}
	if err := syncMediaDirectories(stage); err != nil {
		return err
	}
	if err := atomicBytes(filepath.Join(target, "CURRENT"), []byte(filepath.Base(stage)+"\n"), 0644); err != nil {
		return err
	}
	committed = true
	return syncDirectory(target)
}

func ActiveMedia(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "CURRENT"))
	if os.IsNotExist(err) {
		return root, nil
	}
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(data))
	if !strings.HasPrefix(name, "kit-") || name != filepath.Base(name) || strings.ContainsAny(name, "\\ \t\n") {
		return "", fmt.Errorf("invalid media generation")
	}
	return filepath.Join(root, "payloads", name), nil
}

const universalLauncher = `#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
IFS= read -r generation < "$root/CURRENT"
case "$generation" in kit-*) ;; *) echo "Invalid recovery generation" >&2; exit 1;; esac
case "$generation" in *[!a-zA-Z0-9-]*) echo "Invalid recovery generation" >&2; exit 1;; esac
root="$root/payloads/$generation"
case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo "Unsupported recovery architecture" >&2; exit 1;; esac
cd "$root"
sha256sum --strict -c SHA256SUMS >/dev/null
export WEAZLBACK_MEDIA_ROOT="$root"
export PATH="$root/linux-$arch:$PATH"
case "$(basename -- "$0")" in
 weazlback) exec "$root/linux-$arch/weazlback" "$@";;
 weazlback-turbo-restore) export WEAZLBACK_RESTORE_ENGINE=turbo;;
esac
exec "$root/linux-$arch/weazlback-restore" "$@"
`

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func syncMediaDirectories(root string) error {
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := syncDirectory(dirs[i]); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(root))
}

const UniversalInstructions = `WEAZLBACK UNIVERSAL RECOVERY

From the root of this media, run:
  ./weazlback-turbo-restore

The launcher detects x64 or ARM64, verifies both architectures and shared assets,
and starts native guided recovery with Turbo requested. Keep the vault passphrase
available. The kit contains repository credentials, not the backed-up files.
Missing native applications may download from the target repositories.

CURRENT selects an immutable directory under payloads/. To verify it manually:
  generation=$(cat CURRENT)
  (cd "payloads/$generation" && sha256sum -c SHA256SUMS)

Run launchers at the root of the media, not inside a payload directory. Refresh
with a complete matched universal bundle. Failed refreshes leave the previous
payload available. There is no lost-passphrase recovery.
`
