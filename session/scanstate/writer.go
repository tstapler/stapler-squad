package scanstate

import (
	"context"
	"sync"
	"time"
)

// Saver is the persistence side of a Scoped store.
type Saver interface {
	Save(ctx context.Context, recs []Record) error
}

// Writer coalesces scan-state write-backs: marking a path again before the next
// flush replaces its pending value, so a transcript that changes every second
// is written once per flush interval, and a cold backfill's thousands of
// states trickle out in batches instead of bursting onto the database.
type Writer[T any] struct {
	saver Saver
	build func(path string, v T) (Record, error)

	mu    sync.Mutex
	dirty map[string]T
}

// NewWriter builds a Writer. build turns a pending value into a Record at flush
// time, so costly encoding is skipped for values superseded before the flush.
func NewWriter[T any](s Saver, build func(path string, v T) (Record, error)) *Writer[T] {
	return &Writer[T]{saver: s, build: build, dirty: map[string]T{}}
}

// Mark schedules v for path; newest wins.
func (w *Writer[T]) Mark(path string, v T) {
	w.mu.Lock()
	w.dirty[path] = v
	w.mu.Unlock()
}

// Pending is the number of paths awaiting write-back.
func (w *Writer[T]) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.dirty)
}

// Flush writes up to limit pending values (all when limit <= 0). A value that
// fails to build is dropped until its next Mark; a failed Save re-queues the
// batch unless newer values arrived.
func (w *Writer[T]) Flush(ctx context.Context, limit int) error {
	batch := w.take(limit)
	if len(batch) == 0 {
		return nil
	}
	recs := make([]Record, 0, len(batch))
	for path, v := range batch {
		rec, err := w.build(path, v)
		if err != nil {
			continue
		}
		rec.Path = path
		recs = append(recs, rec)
	}
	if err := w.saver.Save(ctx, recs); err != nil {
		w.requeue(batch)
		return err
	}
	return nil
}

func (w *Writer[T]) take(limit int) map[string]T {
	w.mu.Lock()
	defer w.mu.Unlock()
	if limit <= 0 || limit >= len(w.dirty) {
		batch := w.dirty
		w.dirty = map[string]T{}
		return batch
	}
	batch := make(map[string]T, limit)
	for path, v := range w.dirty {
		if len(batch) == limit {
			break
		}
		batch[path] = v
		delete(w.dirty, path)
	}
	return batch
}

func (w *Writer[T]) requeue(batch map[string]T) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for path, v := range batch {
		if _, newer := w.dirty[path]; !newer {
			w.dirty[path] = v
		}
	}
}

// Run flushes up to batch values every interval until ctx is done, then makes a
// final unbounded flush. onErr receives flush failures; it may be nil.
func (w *Writer[T]) Run(ctx context.Context, every time.Duration, batch int, onErr func(error)) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := w.Flush(final, 0); err != nil && onErr != nil {
				onErr(err)
			}
			return
		case <-tick.C:
			if err := w.Flush(ctx, batch); err != nil && ctx.Err() == nil && onErr != nil {
				onErr(err)
			}
		}
	}
}
