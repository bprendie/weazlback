package freshrestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type injectedEngine struct {
	name string
	err  error
	run  func(*Restore) error
}

func (e injectedEngine) Name() string { return e.name }
func (e injectedEngine) Stage(_ context.Context, restore *Restore) error {
	if e.run != nil {
		return e.run(restore)
	}
	return e.err
}

func TestInjectedTurboFailureRecordsFallback(t *testing.T) {
	dir := t.TempDir()
	fallbackRan := false
	r := Restore{Options: Options{RestoreEngine: injectedEngine{name: EngineTurbo, err: errors.New("injected failure")},
		FallbackEngine: injectedEngine{name: EngineStandard, run: func(*Restore) error { fallbackRan = true; return nil }}},
		JournalPath: filepath.Join(dir, "journal.json"), StageDir: filepath.Join(dir, "stage"),
		Journal: Journal{SchemaVersion: JournalSchemaVersion}, Plan: Plan{OriginalHome: dir}}
	if err := r.stageWithFallback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !fallbackRan {
		t.Fatal("Standard fallback did not execute")
	}
	journal, err := LoadJournal(r.JournalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.FallbackEngine != EngineStandard || journal.FallbackPhase != "staging" || journal.FallbackReason == "" {
		t.Fatalf("fallback=%+v", journal)
	}
	if _, err := os.Stat(r.JournalPath); err != nil {
		t.Fatal(err)
	}
}
