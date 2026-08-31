package app

import (
	"io"
	"os"
	"testing"
)

func TestConfirmPruneRequiresExactRepositoryID(t *testing.T) {
	original := os.Stdin
	defer func() { os.Stdin = original }()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = read
	_, _ = write.WriteString("PRUNE wrong\n")
	_ = write.Close()
	if confirmPrune("repo-1", io.Discard) == nil {
		t.Fatal("inexact retention confirmation accepted")
	}
}
