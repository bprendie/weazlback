package backupmeta

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/inventory"
)

const ManifestName = "weazlback-applications-v1.json"

func PrepareApplications(ctx context.Context, profile string) (string, func(), error) {
	if profile != "core" {
		return "", func() {}, nil
	}
	root, err := privateRoot()
	if err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", func() {}, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", func() {}, err
	}
	staging, err := os.MkdirTemp(root, "applications-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	manifest, err := inventory.CaptureApplications(ctx)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	path := filepath.Join(staging, ManifestName)
	if err := inventory.WriteApplications(path, manifest); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return staging, cleanup, nil
}

func privateRoot() (string, error) {
	path, err := config.Path()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(path), "staging")
	if dir == "" || dir == "." {
		return "", errors.New("invalid private staging path")
	}
	return dir, nil
}
