package app

import (
	"testing"

	"github.com/bprendie/weazlback/internal/restic"
)

func TestCurrentKeyID(t *testing.T) {
	keys := []restic.Key{{ID: "new"}, {ID: "old", Current: true}}
	if got := currentKeyID(keys); got != "old" {
		t.Fatalf("currentKeyID() = %q", got)
	}
}
