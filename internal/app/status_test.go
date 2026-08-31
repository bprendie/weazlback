package app

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/bprendie/weazlback/internal/contracts"
)

func TestStatusMarksOwnerlessStaleBackupInterrupted(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	store, err := defaultStatusStore()
	if err != nil {
		t.Fatal(err)
	}
	value := contracts.Status{State: "backing-up"}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	value, _ = store.Load()
	value.UpdatedAt = time.Now().Add(-10 * time.Minute)
	data, _ := json.Marshal(value)
	if err := atomicAppWrite(store.Path, data); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := statusCommand([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var got contracts.Status
	_ = json.Unmarshal(output.Bytes(), &got)
	if got.State != "failed" {
		t.Fatalf("state=%q", got.State)
	}
}
