package freshrestore

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func recompressBtrfs(ctx context.Context, paths []string) error {
	for _, path := range paths {
		out, err := exec.CommandContext(ctx, "btrfs", "filesystem", "defragment", "-r", "-czstd:1", "--", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("recompress %s: %w: %s", path, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
