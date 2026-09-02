package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Instructions = `WEAZLBACK RECOVERY

You need this folder and the vault passphrase. There is no passphrase recovery.

On a fresh Omarchy system:

  ./weazlback-restore

The guided terminal interface locates the kit, resolves repository access,
lets you choose any local or SSH destination embedded in the kit, reviews
source and target identity, Restore Point, hostname, ownership and application
queues, and runs the resumable restore. Applications-only and catalog-free
selective file/directory recovery are available from the same workspace.

The bundled Restic binary is used when Restic is not installed on the fresh system.

Break glass repository destruction (vault passphrase and exact confirmation required):

  ./weazlback-restore --recovery ./weazlback-recovery.wzrk --nuke-repository

Verify this media before relying on it:

  sha256sum -c SHA256SUMS

The restore workflow verifies the encrypted recovery kit and pinned SSH host key
before accessing the repository. Keep at least two offline copies on separate media.
`

type MediaSources struct {
	Weazlback string
	Restore   string
	Kit       string
	Restic    string
}

func PrepareMedia(target string, sources MediaSources) error {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("recovery target: %w", err)
	}
	if !info.IsDir() {
		return errors.New("recovery target is not a directory")
	}
	files := []struct{ name, source string }{
		{"weazlback", sources.Weazlback},
		{"weazlback-restore", sources.Restore},
	}
	for _, file := range files {
		_, err := copyVerified(filepath.Join(target, file.name), file.source, 0o755)
		if err != nil {
			return err
		}
	}
	if sources.Restic != "" {
		if _, err := copyVerified(filepath.Join(target, "restic"), sources.Restic, 0o755); err != nil {
			return err
		}
	}
	if _, err := copyVerified(filepath.Join(target, "weazlback-recovery.wzrk"), sources.Kit, 0o644); err != nil {
		return err
	}
	return writeSupportFiles(target)
}

func RefreshMedia(target, weazlback, restore, restic string) error {
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return errors.New("recovery target must be an existing directory")
	}
	for _, source := range []struct{ name, path string }{{"weazlback", weazlback}, {"weazlback-restore", restore}} {
		if _, err := copyVerified(filepath.Join(target, source.name), source.path, 0o755); err != nil {
			return err
		}
	}
	if restic != "" {
		if _, err := copyVerified(filepath.Join(target, "restic"), restic, 0o755); err != nil {
			return err
		}
	}
	kit := filepath.Join(target, "weazlback-recovery.wzrk")
	if _, err := os.Stat(kit); err == nil {
		if err := os.Chmod(kit, 0o644); err != nil {
			return err
		}
	}
	return writeSupportFiles(target)
}

func writeSupportFiles(target string) error {
	if err := atomicBytes(filepath.Join(target, "RESTORE.txt"), []byte(Instructions), 0o644); err != nil {
		return err
	}
	if err := atomicBytes(filepath.Join(target, "THIRD_PARTY_NOTICES.txt"), []byte(ThirdPartyNotices), 0o644); err != nil {
		return err
	}
	names := []string{"weazlback", "weazlback-restore", "RESTORE.txt", "THIRD_PARTY_NOTICES.txt"}
	if _, err := os.Stat(filepath.Join(target, "restic")); err == nil {
		names = append(names, "restic")
	}
	if _, err := os.Stat(filepath.Join(target, "weazlback-recovery.wzrk")); err == nil {
		names = append(names, "weazlback-recovery.wzrk")
	}
	checksums := make([]string, 0, len(names))
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(target, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		checksums = append(checksums, hex.EncodeToString(sum[:])+"  "+name)
	}
	return atomicBytes(filepath.Join(target, "SHA256SUMS"), []byte(strings.Join(checksums, "\n")+"\n"), 0o644)
}

func copyVerified(destination, source string, mode os.FileMode) (string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filepath.Base(source), err)
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".weazlback-media-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	hash := sha256.New()
	if _, err = io.Copy(io.MultiWriter(tmp, hash), in); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Chmod(mode); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(name, destination); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func atomicBytes(destination string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".weazlback-media-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, destination)
}
