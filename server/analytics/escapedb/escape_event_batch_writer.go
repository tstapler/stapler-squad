package escapedb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/log"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/dbobs"
)

const (
	// DefaultFlushRows is the queued-event count that triggers an immediate flush.
	DefaultFlushRows = 500
	// DefaultFlushInterval is the longest an event waits in memory before it is written.
	DefaultFlushInterval = 5 * time.Second
	// DefaultQueueSize bounds the in-memory queue; events beyond it are dropped and counted.
	DefaultQueueSize = 10000

	// insertChunkRows keeps one multi-row INSERT at 14 bind variables per row, well under SQLite's limit.
	insertChunkRows = 500

	slowFlushWarn = time.Second
)

// EscapeEventBatchWriter persists escape events to SQLite. It implements
// pkganalytics.EscapeEventWriter.
//
// Each flush is one transaction holding multi-row INSERTs of the raw events plus an
// additive upsert of their hourly rollups, so history survives raw-row eviction.
// Per-session caps and age eviction are not done here: see Retention.
type EscapeEventBatchWriter struct {
	ch  chan pkganalytics.EscapeEventRecord
	drv dialect.Driver

	flushRows  int
	flushEvery time.Duration
	newTicker  func(time.Duration) (<-chan time.Time, func())

	enqueued, dropped, flushes, rowsWritten, failedRows atomic.Int64
	lastFlushNanos, maxFlushNanos                       atomic.Int64
	lastDropWarned                                      atomic.Int64
}

// WriterOption customizes an EscapeEventBatchWriter.
type WriterOption func(*EscapeEventBatchWriter)

// WithFlushRows sets the queued-event count that triggers an immediate flush.
func WithFlushRows(n int) WriterOption {
	return func(w *EscapeEventBatchWriter) { w.flushRows = max(n, 1) }
}

// WithFlushInterval sets the time-based flush period.
func WithFlushInterval(d time.Duration) WriterOption {
	return func(w *EscapeEventBatchWriter) { w.flushEvery = d }
}

// WithQueueSize sets the bounded queue capacity.
func WithQueueSize(n int) WriterOption {
	return func(w *EscapeEventBatchWriter) { w.ch = make(chan pkganalytics.EscapeEventRecord, max(n, 1)) }
}

// WithTicker injects the flush timer so tests drive time deterministically.
func WithTicker(f func(time.Duration) (<-chan time.Time, func())) WriterOption {
	return func(w *EscapeEventBatchWriter) { w.newTicker = f }
}

// NewEscapeEventBatchWriter creates a batch writer over drv (the escape-events database
// driver, with the rollup schema already created by Open). Call Start to begin processing.
func NewEscapeEventBatchWriter(drv dialect.Driver, opts ...WriterOption) *EscapeEventBatchWriter {
	w := &EscapeEventBatchWriter{
		ch:         make(chan pkganalytics.EscapeEventRecord, DefaultQueueSize),
		drv:        drv,
		flushRows:  DefaultFlushRows,
		flushEvery: DefaultFlushInterval,
		newTicker: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		},
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// WriteEscapeEvent enqueues an event. Never blocks the terminal path: when the queue is
// full the event is dropped and counted.
func (w *EscapeEventBatchWriter) WriteEscapeEvent(_ context.Context, event pkganalytics.EscapeEventRecord) {
	select {
	case w.ch <- event:
		w.enqueued.Add(1)
	default:
		w.dropped.Add(1)
	}
}

// Start begins the background flush loop and returns when ctx is cancelled, after
// persisting what is still queued.
func (w *EscapeEventBatchWriter) Start(ctx context.Context) {
	tick, stop := w.newTicker(w.flushEvery)
	defer stop()
	registerWriterMetrics(w)
	defer unregisterWriterMetrics(w)
	// Stamp live_since before the first flush: rows older than it are backfilled, newer ones counted here.
	if _, err := ensureRollupState(ctx, w.drv, time.Now()); err != nil {
		log.Warn("escape analytics: rollup state init failed", "err", err)
	}

	batch := make([]pkganalytics.EscapeEventRecord, 0, w.flushRows)
	flush := func(c context.Context) {
		if len(batch) > 0 {
			w.flushBatch(c, batch)
			batch = batch[:0]
		}
	}
	for {
		select {
		case ev := <-w.ch:
			batch = append(batch, ev)
			if len(batch) >= w.flushRows {
				flush(ctx)
			}
		case <-tick:
			flush(ctx)
		case <-ctx.Done():
			// The cancelled ctx would fail the insert, so drain on a bounded fresh one.
			drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			for {
				select {
				case ev := <-w.ch:
					batch = append(batch, ev)
					if len(batch) >= w.flushRows {
						flush(drainCtx)
					}
				default:
					flush(drainCtx)
					return
				}
			}
		}
	}
}

// WriterStats is a point-in-time copy of the writer's counters.
type WriterStats struct {
	Enqueued, Dropped, Flushes, RowsWritten, FailedRows int64
	QueueDepth, QueueCapacity                           int
	LastFlush, MaxFlush                                 time.Duration
}

// Stats returns the writer's counters.
func (w *EscapeEventBatchWriter) Stats() WriterStats {
	return WriterStats{
		Enqueued: w.enqueued.Load(), Dropped: w.dropped.Load(), Flushes: w.flushes.Load(),
		RowsWritten: w.rowsWritten.Load(), FailedRows: w.failedRows.Load(),
		QueueDepth: len(w.ch), QueueCapacity: cap(w.ch),
		LastFlush: time.Duration(w.lastFlushNanos.Load()), MaxFlush: time.Duration(w.maxFlushNanos.Load()),
	}
}

// DroppedCount returns the number of events dropped due to backpressure.
func (w *EscapeEventBatchWriter) DroppedCount() int64 { return w.dropped.Load() }

// flushBatch writes batch and its hourly rollups in one transaction. On failure nothing is
// written (the raw rows and their counts stay consistent) and the batch is counted as failed.
func (w *EscapeEventBatchWriter) flushBatch(ctx context.Context, batch []pkganalytics.EscapeEventRecord) {
	if len(batch) == 0 {
		return
	}
	began := time.Now()
	rollups := make(rollupSet)
	for _, ev := range batch {
		rollups.add(RollupHourSecs, eventFacts{
			SessionID: ev.SessionID, ProjectPath: ev.ProjectPath, SequenceType: ev.SequenceType,
			Stage: string(ev.Stage), MangleType: ev.MangleType, Mangled: ev.Mangled,
			ByteLen: int64(ev.ByteLen), At: ev.WallTime,
		})
	}

	ctx = dbobs.WithCaller(ctx, "server/analytics.EscapeEventBatchWriter.flushBatch")
	err := withTx(ctx, w.drv, func(tx dialect.Tx) error {
		for start := 0; start < len(batch); start += insertChunkRows {
			if err := insertEvents(ctx, tx, batch[start:min(start+insertChunkRows, len(batch))]); err != nil {
				return err
			}
		}
		return upsertRollups(ctx, tx, rollups)
	})
	w.recordFlush(time.Since(began), len(batch), len(rollups), err)
}

func (w *EscapeEventBatchWriter) recordFlush(elapsed time.Duration, rows, rollupRows int, err error) {
	w.lastFlushNanos.Store(int64(elapsed))
	if int64(elapsed) > w.maxFlushNanos.Load() {
		w.maxFlushNanos.Store(int64(elapsed))
	}
	if err != nil {
		w.failedRows.Add(int64(rows))
		log.Warn("escape analytics: flush batch failed", "err", err, "batch_size", rows)
		return
	}
	w.flushes.Add(1)
	w.rowsWritten.Add(int64(rows))
	if elapsed > slowFlushWarn {
		log.Warn("escape analytics: slow flush", "batch_size", rows, "rollup_rows", rollupRows, "elapsed", elapsed)
	}
	if d := w.dropped.Load(); d > w.lastDropWarned.Load() {
		log.Warn("escape analytics: events dropped under backpressure", "dropped_total", d, "queue_capacity", cap(w.ch))
		w.lastDropWarned.Store(d)
	}
}

const insertEventPrefix = "INSERT INTO `escape_events` (`id`, `session_id`, `project_path`, `stage`, `sequence_type`, `sequence_subtype`, `sequence_signature`, `byte_length`, `payload_hash`, `raw_bytes`, `mangled`, `mangle_type`, `wall_time`, `session_seq`) VALUES "

// insertEvents writes events as one multi-row INSERT. Optional columns are NULL when empty,
// matching what ent's builder wrote.
func insertEvents(ctx context.Context, q dialect.ExecQuerier, events []pkganalytics.EscapeEventRecord) error {
	var sb strings.Builder
	sb.WriteString(insertEventPrefix)
	args := make([]any, 0, len(events)*14)
	for i, ev := range events {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args, generateEscapeEventID(), ev.SessionID, nullIfEmpty(ev.ProjectPath), string(ev.Stage),
			ev.SequenceType, nullIfEmpty(ev.SequenceSubtype), nullIfEmpty(ev.SequenceSignature), ev.ByteLen,
			nullIfEmpty(ev.PayloadHash), nullIfEmptyBytes(ev.RawBytes), ev.Mangled, nullIfEmpty(ev.MangleType),
			ev.WallTime.Round(0), ev.SessionSeq)
	}
	return q.Exec(ctx, sb.String(), args, nil)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfEmptyBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func generateEscapeEventID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
