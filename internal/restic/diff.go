package restic

import (
	"context"
	"encoding/json"
	"strings"
)

type DiffChange struct {
	Path     string `json:"path"`
	Modifier string `json:"modifier"`
}

func (s Service) Diff(ctx context.Context, repo Repository, source, target string) ([]DiffChange, error) {
	b, err := s.Runner.Run(ctx, repo, "diff", "--json", source, target)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	var changes []DiffChange
	for decoder.More() {
		var event struct {
			MessageType string `json:"message_type"`
			DiffChange
		}
		if err := decoder.Decode(&event); err != nil {
			return nil, err
		}
		if event.MessageType == "change" {
			changes = append(changes, event.DiffChange)
		}
	}
	return changes, nil
}
