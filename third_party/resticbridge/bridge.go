package weazlbridge

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/sftp"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	irestic "github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/restorer"
	"github.com/restic/restic/internal/ui"
	uiprogress "github.com/restic/restic/internal/ui/progress"
	restoreui "github.com/restic/restic/internal/ui/restore"
)

const UpstreamVersion = "v0.19.1"

type Options struct {
	Repository, Password, Snapshot, Target string
	SSHArgs                                string
	Connections                            int
	DownloadLimitKiB                       int
	Progress                               func(Progress)
}

type Progress struct {
	FilesDone, FilesTotal, BytesDone, BytesTotal uint64
	WireBytes                                    uint64
	Elapsed                                      time.Duration
}

func Restore(ctx context.Context, opts Options) (uint64, error) {
	if opts.Repository == "" || opts.Password == "" || opts.Snapshot == "" || opts.Target == "" {
		return 0, fmt.Errorf("repository, password, snapshot, and target are required")
	}
	extended := options.Options{}
	if opts.Connections > 0 {
		value := fmt.Sprint(opts.Connections)
		extended["local.connections"], extended["sftp.connections"] = value, value
	}
	if opts.SSHArgs != "" {
		extended["sftp.args"] = opts.SSHArgs
	}
	backends := location.NewRegistry()
	backends.Register(local.NewFactory())
	backends.Register(sftp.NewFactory())
	counter := &atomic.Uint64{}
	gopts := global.Options{Repo: opts.Repository, Password: opts.Password, Backends: backends, NoCache: true, Extended: extended, Term: &ui.MockTerminal{},
		BackendInnerTestHook: func(inner backend.Backend) (backend.Backend, error) {
			return &readOnlyCountingBackend{Backend: inner, bytes: counter}, nil
		}}
	gopts.Limits.DownloadKb = opts.DownloadLimitKiB
	printer := &uiprogress.NoopPrinter{}
	repo, err := global.OpenRepository(ctx, gopts, printer)
	if err != nil {
		return 0, err
	}
	defer repo.Close()
	unlock, lockedCtx, err := repository.Lock(ctx, repo, false, 0, func(string) {}, func(string, ...interface{}) {})
	if err != nil {
		return 0, err
	}
	defer unlock.Unlock()
	if err := repo.LoadIndex(lockedCtx, nil); err != nil {
		return 0, err
	}
	id, err := irestic.ParseID(opts.Snapshot)
	if err != nil {
		return 0, err
	}
	snapshot, err := data.LoadSnapshot(lockedCtx, repo, id)
	if err != nil {
		return 0, err
	}
	progress := newBridgeProgress(opts.Progress, counter)
	engine := restorer.NewRestorer(repo, snapshot, restorer.Options{Sparse: true, Progress: progress})
	engine.Error = func(location string, err error) error { return fmt.Errorf("restore %s: %w", location, err) }
	count, err := engine.RestoreTo(lockedCtx, opts.Target)
	if progress != nil {
		progress.Finish()
	}
	return count, err
}

func newBridgeProgress(callback func(Progress), counter *atomic.Uint64) *restoreui.Progress {
	if callback == nil {
		return nil
	}
	return restoreui.NewProgress(&progressPrinter{callback: callback, wire: counter}, 200*time.Millisecond)
}

type progressPrinter struct {
	uiprogress.NoopPrinter
	callback func(Progress)
	wire     *atomic.Uint64
}

func (p *progressPrinter) Update(state restoreui.State, elapsed time.Duration) {
	p.emit(state, elapsed)
}
func (p *progressPrinter) Finish(state restoreui.State, elapsed time.Duration) {
	p.emit(state, elapsed)
}
func (p *progressPrinter) CompleteItem(restoreui.ItemAction, string, uint64) {}
func (p *progressPrinter) Error(_ string, err error) error                   { return err }
func (p *progressPrinter) emit(state restoreui.State, elapsed time.Duration) {
	p.callback(Progress{FilesDone: state.FilesFinished, FilesTotal: state.FilesTotal,
		BytesDone: state.AllBytesWritten, BytesTotal: state.AllBytesTotal, WireBytes: p.wire.Load(), Elapsed: elapsed})
}

type readOnlyCountingBackend struct {
	backend.Backend
	bytes *atomic.Uint64
}

func (b *readOnlyCountingBackend) Load(ctx context.Context, handle backend.Handle, length int, offset int64, fn func(io.Reader) error) error {
	return b.Backend.Load(ctx, handle, length, offset, func(reader io.Reader) error {
		return fn(&countingReader{reader: reader, bytes: b.bytes})
	})
}

func (b *readOnlyCountingBackend) Save(ctx context.Context, handle backend.Handle, reader backend.RewindReader) error {
	if handle.Type != backend.LockFile {
		return fmt.Errorf("Turbo reader refused repository write")
	}
	return b.Backend.Save(ctx, handle, reader)
}

func (b *readOnlyCountingBackend) Remove(ctx context.Context, handle backend.Handle) error {
	if handle.Type != backend.LockFile {
		return fmt.Errorf("Turbo reader refused repository removal")
	}
	return b.Backend.Remove(ctx, handle)
}

func (b *readOnlyCountingBackend) Delete(context.Context) error {
	return fmt.Errorf("Turbo reader refused repository deletion")
}

type countingReader struct {
	reader io.Reader
	bytes  *atomic.Uint64
}

func (r *countingReader) Read(value []byte) (int, error) {
	count, err := r.reader.Read(value)
	r.bytes.Add(uint64(count))
	return count, err
}
