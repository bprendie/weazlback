package freshrestore

import (
	"context"
	"errors"
	"testing"
)

type memorySetting struct {
	name, value string
	fail        bool
}

func (s *memorySetting) Name() string                         { return s.name }
func (s *memorySetting) Read(context.Context) (string, error) { return s.value, nil }
func (s *memorySetting) Write(_ context.Context, value string) error {
	if s.fail {
		s.fail = false
		return errors.New("injected crash")
	}
	s.value = value
	return nil
}

func TestTuningFailureRestoresExactOriginalValues(t *testing.T) {
	first := &memorySetting{name: "read-ahead", value: "128"}
	second := &memorySetting{name: "dirty", value: "10", fail: true}
	controller := &TuningController{}
	err := controller.Apply(context.Background(), []TuningRequest{{first, "4096"}, {second, "40"}})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	if first.value != "128" || second.value != "10" {
		t.Fatalf("values=%q,%q", first.value, second.value)
	}
	if err := controller.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledWorkRestoresTuning(t *testing.T) {
	setting := &memorySetting{name: "read-ahead", value: "128"}
	ctx, cancel := context.WithCancel(context.Background())
	err := RunTuned(ctx, []TuningRequest{{setting, "4096"}}, func(context.Context) error {
		cancel()
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) || setting.value != "128" {
		t.Fatalf("err=%v value=%s", err, setting.value)
	}
}
