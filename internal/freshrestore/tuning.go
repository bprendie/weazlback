package freshrestore

import (
	"context"
	"fmt"
	"sync"
)

type TuningSetting interface {
	Name() string
	Read(context.Context) (string, error)
	Write(context.Context, string) error
}

type TuningRequest struct {
	Setting TuningSetting
	Value   string
}

// TuningController records every original value before mutation and restores in
// reverse order. Restore is idempotent so normal completion, cancellation, and
// signal handlers can safely race to clean up.
type TuningController struct {
	mu       sync.Mutex
	applied  []appliedTuning
	restored bool
}

type appliedTuning struct {
	setting  TuningSetting
	original string
}

func (c *TuningController) Apply(ctx context.Context, requests []TuningRequest) error {
	for _, request := range requests {
		original, err := request.Setting.Read(ctx)
		if err != nil {
			_ = c.Restore(context.Background())
			return fmt.Errorf("read %s: %w", request.Setting.Name(), err)
		}
		c.mu.Lock()
		c.applied = append(c.applied, appliedTuning{request.Setting, original})
		c.mu.Unlock()
		if err := request.Setting.Write(ctx, request.Value); err != nil {
			_ = c.Restore(context.Background())
			return fmt.Errorf("apply %s: %w", request.Setting.Name(), err)
		}
	}
	return nil
}

func (c *TuningController) Restore(ctx context.Context) error {
	c.mu.Lock()
	if c.restored {
		c.mu.Unlock()
		return nil
	}
	c.restored = true
	applied := append([]appliedTuning(nil), c.applied...)
	c.mu.Unlock()
	var first error
	for i := len(applied) - 1; i >= 0; i-- {
		if err := applied[i].setting.Write(ctx, applied[i].original); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func RunTuned(ctx context.Context, requests []TuningRequest, work func(context.Context) error) error {
	controller := &TuningController{}
	if err := controller.Apply(ctx, requests); err != nil {
		return err
	}
	defer controller.Restore(context.Background())
	return work(ctx)
}
