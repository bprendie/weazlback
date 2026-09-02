package freshrestore

import (
	"context"
	"fmt"
)

const (
	EngineStandard = "standard"
	EngineTurbo    = "turbo"
)

type RestoreEngine interface {
	Name() string
	Stage(context.Context, *Restore) error
}

type standardEngine struct{}

func (standardEngine) Name() string { return EngineStandard }

func (standardEngine) Stage(ctx context.Context, restore *Restore) error {
	return restore.stageCoreStandard(ctx)
}

func (r *Restore) selectedEngine() RestoreEngine {
	if r.Options.RestoreEngine != nil {
		return r.Options.RestoreEngine
	}
	if r.Options.Engine == EngineTurbo && r.Journal.Qualification.Eligible {
		return turboLandingEngine{}
	}
	return standardEngine{}
}

func (r *Restore) stageWithFallback(ctx context.Context) error {
	engine := r.selectedEngine()
	if r.Journal.RequestedEngine == "" {
		r.Journal.RequestedEngine = engine.Name()
	}
	r.Journal.Engine = engine.Name()
	if err := engine.Stage(ctx, r); err == nil {
		return nil
	} else if engine.Name() == EngineStandard {
		return err
	} else {
		r.Journal.FallbackEngine = EngineStandard
		r.Journal.FallbackPhase = "staging"
		r.Journal.FallbackReason = fmt.Sprintf("%s staging failed: %v", engine.Name(), err)
		r.Journal.Engine = EngineStandard
		if saveErr := SaveJournal(r.JournalPath, r.Journal); saveErr != nil {
			return fmt.Errorf("record fallback: %w", saveErr)
		}
		if r.Session == nil {
			if r.Options.FallbackEngine == nil {
				return fmt.Errorf("standard fallback unavailable: restore session is missing")
			}
		}
		fallback := RestoreEngine(standardEngine{})
		if r.Options.FallbackEngine != nil {
			fallback = r.Options.FallbackEngine
		}
		return fallback.Stage(ctx, r)
	}
}
