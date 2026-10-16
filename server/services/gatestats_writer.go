package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// StatsFlushTimeout bounds the deferred final flush so a stalled fsync cannot
// extend Shutdown past the service manager's stop window.
const StatsFlushTimeout = 5 * time.Second

// foreignAware is implemented by stores that can tell another live process is
// writing the same file.
type foreignAware interface {
	ForeignWriterActive() bool
}

// StatsWriter ticks the stats accumulator and persists it through a StatsStore.
// It is started by the server (not by BuildRuntimeDeps, which has no server
// context) and joined through the server's background-task WaitGroup.
type StatsWriter struct {
	stats  *deliverygate.Stats
	store  deliverygate.StatsStore
	now    func() time.Time
	logger *slog.Logger
	// after is the flush-timeout timer factory; injected so tests need no sleep.
	after func(time.Duration) <-chan time.Time

	mu      sync.Mutex // guards foreign
	foreign bool
}

// NewStatsWriter builds a writer. now and after default to the real clock.
func NewStatsWriter(stats *deliverygate.Stats, store deliverygate.StatsStore, now func() time.Time,
	after func(time.Duration) <-chan time.Time, logger *slog.Logger) *StatsWriter {
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = time.After
	}
	if logger == nil {
		logger = deliverygate.NewRepoLogger()
	}
	return &StatsWriter{stats: stats, store: store, now: now, after: after, logger: logger}
}

// Start reloads the previous window synchronously (so the first events land in
// a restored accumulator), marks the writer running and starts the tick loop.
// The loop ends when ctx is cancelled; wg joins it.
func (w *StatsWriter) Start(ctx context.Context, tick <-chan time.Time, wg *sync.WaitGroup) {
	if p, ok := w.store.Load(); ok {
		w.stats.Merge(p)
	}
	w.stats.SetWriterRunning(true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer w.stats.SetWriterRunning(false)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-tick:
				if !ok {
					return
				}
				w.flush()
			}
		}
	}()
}

// flush credits elapsed time and persists. Every error is already logged and
// reflected in Status by the store; none propagates.
func (w *StatsWriter) flush() {
	w.stats.Tick(w.now())
	if w.resumeIfForeignGone() {
		return
	}
	if err := w.store.Save(w.stats.Persist()); errors.Is(err, errForeignWriter) {
		w.mu.Lock()
		w.foreign = true
		w.mu.Unlock()
	}
}

// resumeIfForeignGone reports true while another process still owns the file.
// When it has gone, the file is merged per hour_start into memory first.
func (w *StatsWriter) resumeIfForeignGone() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.foreign {
		return false
	}
	if fa, ok := w.store.(foreignAware); ok && fa.ForeignWriterActive() {
		return true
	}
	if p, ok := w.store.Load(); ok {
		w.stats.Merge(p)
	}
	w.foreign = false
	return false
}

// FlushFinal is the shutdown flush: it runs the save in a helper goroutine and
// waits at most StatsFlushTimeout, so a stalled fsync only abandons a write
// whose stale temp file the next start removes.
func (w *StatsWriter) FlushFinal() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.flush()
	}()
	select {
	case <-done:
	case <-w.after(StatsFlushTimeout):
		w.logger.Warn("delivery_gate_stats_final_flush_timed_out", "timeout", StatsFlushTimeout)
	}
}
