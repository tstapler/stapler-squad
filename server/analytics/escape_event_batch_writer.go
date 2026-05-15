package analytics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
)

// EscapeEventBatchWriter persists escape events to SQLite via batched ent writes.
// It implements pkganalytics.EscapeEventWriter.
type EscapeEventBatchWriter struct {
	ch                chan pkganalytics.EscapeEventRecord
	client            *ent.Client
	maxRowsPerSession int
	sessionRowCounts  map[string]int
	dropped           int64 // atomic counter for dropped events
}

// NewEscapeEventBatchWriter creates a new batch writer. Call Start to begin processing.
func NewEscapeEventBatchWriter(client *ent.Client, maxRowsPerSession int) *EscapeEventBatchWriter {
	return &EscapeEventBatchWriter{
		ch:                make(chan pkganalytics.EscapeEventRecord, 1000),
		client:            client,
		maxRowsPerSession: maxRowsPerSession,
		sessionRowCounts:  make(map[string]int),
	}
}

// WriteEscapeEvent enqueues an event. Non-blocking: drops if channel is full.
func (w *EscapeEventBatchWriter) WriteEscapeEvent(_ context.Context, event pkganalytics.EscapeEventRecord) {
	select {
	case w.ch <- event:
	default:
		atomic.AddInt64(&w.dropped, 1)
	}
}

// Start begins the background flush goroutine. Returns when ctx is cancelled.
func (w *EscapeEventBatchWriter) Start(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var batch []pkganalytics.EscapeEventRecord

	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.flushBatch(ctx, batch)
		batch = batch[:0]
	}

	for {
		select {
		case ev := <-w.ch:
			// Check per-session row cap (in-memory)
			if w.maxRowsPerSession > 0 {
				if w.sessionRowCounts[ev.SessionID] >= w.maxRowsPerSession {
					continue
				}
				w.sessionRowCounts[ev.SessionID]++
			}
			batch = append(batch, ev)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			// Drain remaining, applying the same per-session row cap as normal processing.
			for {
				select {
				case ev := <-w.ch:
					if w.maxRowsPerSession > 0 {
						if w.sessionRowCounts[ev.SessionID] >= w.maxRowsPerSession {
							continue
						}
						w.sessionRowCounts[ev.SessionID]++
					}
					batch = append(batch, ev)
				default:
					flush()
					return
				}
			}
		}
	}
}

// DroppedCount returns the number of events dropped due to backpressure.
func (w *EscapeEventBatchWriter) DroppedCount() int64 {
	return atomic.LoadInt64(&w.dropped)
}

func (w *EscapeEventBatchWriter) flushBatch(ctx context.Context, batch []pkganalytics.EscapeEventRecord) {
	if len(batch) == 0 {
		return
	}

	creators := make([]*ent.EscapeEventCreate, 0, len(batch))
	for _, ev := range batch {
		id := generateEscapeEventID()
		c := w.client.EscapeEvent.Create().
			SetID(id).
			SetSessionID(ev.SessionID).
			SetStage(string(ev.Stage)).
			SetSequenceType(ev.SequenceType).
			SetByteLength(ev.ByteLen).
			SetMangled(ev.Mangled).
			SetWallTime(ev.WallTime).
			SetSessionSeq(ev.SessionSeq)

		if ev.SequenceSubtype != "" {
			c = c.SetSequenceSubtype(ev.SequenceSubtype)
		}
		if ev.PayloadHash != "" {
			c = c.SetPayloadHash(ev.PayloadHash)
		}
		if len(ev.RawBytes) > 0 {
			c = c.SetRawBytes(ev.RawBytes)
		}
		if ev.MangleType != "" {
			c = c.SetMangleType(ev.MangleType)
		}

		creators = append(creators, c)
	}

	if err := w.client.EscapeEvent.CreateBulk(creators...).Exec(ctx); err != nil {
		log.Warn("escape analytics: flush batch failed", "err", err, "batch_size", len(batch))
	}
}

func generateEscapeEventID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
