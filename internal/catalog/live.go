package catalog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LivePathHints uses plocate only as a convenience hint source. Every result
// is verified against the live filesystem; repository history remains the
// authority for restore searches.
func LivePathHints(query string, limit int) ([]string, error) {
	query = strings.TrimSpace(query)
	if len(query) < 3 || limit <= 0 {
		return nil, nil
	}
	binary, err := exec.LookPath("plocate")
	if err != nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "--ignore-case", "--null", "--limit", "200", query).Output()
	if err != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, err
	}
	var hints []string
	for _, candidate := range strings.Split(string(output), "\x00") {
		if candidate == "" {
			continue
		}
		if _, statErr := os.Lstat(candidate); statErr == nil {
			hints = append(hints, candidate)
			if len(hints) == limit {
				break
			}
		}
	}
	return hints, nil
}
