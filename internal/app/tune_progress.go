package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var tuneFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type tuneSpinner struct {
	writer io.Writer
	label  string
	done   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

func startTuneSpinner(writer io.Writer, label string) *tuneSpinner {
	spinner := &tuneSpinner{writer: writer, label: label, done: make(chan struct{})}
	spinner.wg.Add(1)
	go func() {
		defer spinner.wg.Done()
		started := time.Now()
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			fmt.Fprintf(writer, "\r\x1b[2K%s  %-28s %s", tuneFrames[frame], label, time.Since(started).Round(100*time.Millisecond))
			select {
			case <-spinner.done:
				fmt.Fprint(writer, "\r\x1b[2K")
				return
			case <-ticker.C:
				frame = (frame + 1) % len(tuneFrames)
			}
		}
	}()
	return spinner
}

func (s *tuneSpinner) stop() {
	if s == nil {
		return
	}
	s.once.Do(func() { close(s.done) })
	s.wg.Wait()
}

type tuneBar struct {
	writer io.Writer
	last   time.Time
}

func (b *tuneBar) update(written, total int64, elapsed time.Duration) {
	if total <= 0 || elapsed <= 0 {
		return
	}
	now := time.Now()
	if written < total && !b.last.IsZero() && now.Sub(b.last) < 80*time.Millisecond {
		return
	}
	b.last = now
	const width = 24
	percent := float64(written) / float64(total)
	if percent > 1 {
		percent = 1
	}
	filled := int(percent * width)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	rate := float64(written) / (1 << 20) / elapsed.Seconds()
	fmt.Fprintf(b.writer, "\r\x1b[2K[%s] %3.0f%%  %5.1f/100 MiB  %5.1f MiB/s  %s",
		bar, percent*100, float64(written)/(1<<20), rate, elapsed.Round(100*time.Millisecond))
	if written >= total {
		fmt.Fprintln(b.writer)
	}
}
