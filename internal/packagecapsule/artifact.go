package packagecapsule

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func findArtifact(roots []string, name, version string) string {
	var matches []string
	for _, root := range roots {
		patterns := []string{
			filepath.Join(root, name+"-"+version+"-*.pkg.tar.*"),
			filepath.Join(root, "*", name+"-"+version+"-*.pkg.tar.*"),
			filepath.Join(root, "*", "*", name+"-"+version+"-*.pkg.tar.*"),
		}
		for _, pattern := range patterns {
			found, _ := filepath.Glob(pattern)
			matches = append(matches, found...)
		}
	}
	sort.Strings(matches)
	for _, match := range matches {
		if !strings.HasSuffix(match, ".sig") && regular(match) {
			return match
		}
	}
	return ""
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func copyHashed(source, destination string) (string, int64, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hash), in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", written, copyErr
	}
	if closeErr != nil {
		return "", written, closeErr
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), written, nil
}

func digest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
