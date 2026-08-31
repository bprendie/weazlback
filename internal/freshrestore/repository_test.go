package freshrestore

import (
	"errors"
	"strings"
	"testing"

	"github.com/bprendie/weazlback/internal/config"
)

func TestClassifyLocalPermissionErrorIsActionable(t *testing.T) {
	dir := t.TempDir()
	err := ClassifyRepositoryError(config.Destination{Kind: "local"}, dir, errors.New("open index/abc: permission denied"))
	message := err.Error()
	if !strings.Contains(message, "--adopt-local-repository") || !strings.Contains(message, dir) {
		t.Fatalf("error is not actionable: %s", message)
	}
}

func TestClassifySSHErrorDoesNotSuggestLocalAdoption(t *testing.T) {
	err := ClassifyRepositoryError(config.Destination{Kind: "ssh"}, "sftp:user@host:/repo", errors.New("permission denied"))
	if strings.Contains(err.Error(), "--adopt-local-repository") {
		t.Fatalf("SSH error suggested local ownership mutation: %s", err)
	}
	if !strings.Contains(err.Error(), "remote account") || !strings.Contains(err.Error(), "will not run remote sudo") {
		t.Fatalf("SSH error is not actionable: %s", err)
	}
}

func TestAdoptionRejectsRootAndRemote(t *testing.T) {
	if err := AdoptLocalRepository(t.Context(), config.Destination{Kind: "local"}, "/"); err == nil {
		t.Fatal("root adoption was accepted")
	}
	if err := AdoptLocalRepository(t.Context(), config.Destination{Kind: "ssh"}, t.TempDir()); err == nil {
		t.Fatal("remote adoption was accepted")
	}
}
