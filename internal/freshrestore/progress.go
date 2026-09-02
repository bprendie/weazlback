package freshrestore

import (
	"context"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

func (r *Restore) restorePoint(ctx context.Context, label, snapshot, target string) error {
	started := time.Now()
	return r.service.RestoreWithProgress(ctx, r.Session.Repository, snapshot, target, nil, func(value restic.RestoreProgress) {
		total, completed := int(value.TotalFiles), int(value.FilesRestored)
		if value.MessageType == "summary" && total > 0 {
			completed = total
		}
		elapsed := time.Since(started).Seconds()
		bytesRate, filesRate := 0.0, 0.0
		if elapsed > 0 {
			bytesRate = float64(value.BytesRestored) / elapsed
			filesRate = float64(value.FilesRestored) / elapsed
		}
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "filesystem", Lane: label, Current: "decrypting and extracting", Completed: completed, Total: total,
			BytesDone: value.BytesRestored, BytesTotal: value.TotalBytes, BytesPerSecond: bytesRate, FilesPerSecond: filesRate})
	})
}
