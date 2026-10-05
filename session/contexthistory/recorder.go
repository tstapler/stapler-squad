package contexthistory

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// DefaultRetention bounds how long samples and compaction events are kept.
const DefaultRetention = 90 * 24 * time.Hour

// pruneInterval is how often the recorder applies retention.
const pruneInterval = 6 * time.Hour

// Recorder persists a ParseResult's context samples and compaction events.
type Recorder struct {
	store     *Store
	maxFn     func(model string) int
	retention time.Duration
	now       func() time.Time
}

// NewRecorder builds a Recorder. maxFn maps a model name to its context window;
// retention <= 0 selects DefaultRetention.
func NewRecorder(store *Store, maxFn func(model string) int, retention time.Duration) *Recorder {
	if retention <= 0 {
		retention = DefaultRetention
	}
	if maxFn == nil {
		maxFn = func(string) int { return 0 }
	}
	return &Recorder{store: store, maxFn: maxFn, retention: retention, now: time.Now}
}

// Record persists new turns and all compaction events from result. Idempotent:
// turns already stored are skipped and conflicting rows are upserted, so
// re-parsing the same transcript never duplicates rows. Turns older than the
// retention window are not (re-)inserted, so pruned history stays pruned.
func (r *Recorder) Record(ctx context.Context, result *tokens.ParseResult) error {
	if result == nil || result.SessionUUID == "" {
		return nil
	}
	cutoff := r.now().Add(-r.retention)

	next, err := r.store.NextTurnIndex(ctx, result.SessionUUID)
	if err != nil {
		return err
	}
	var samples []Sample
	for i := next; i < len(result.TurnTimeline); i++ {
		t := result.TurnTimeline[i]
		if !t.Timestamp.IsZero() && t.Timestamp.Before(cutoff) {
			continue
		}
		samples = append(samples, Sample{
			SessionUUID:   result.SessionUUID,
			TurnIndex:     i,
			SampledAt:     t.Timestamp,
			Model:         t.Model,
			ContextTokens: t.ContextTokens(),
			ContextMax:    int64(r.maxFn(t.Model)),
		})
	}
	if err := r.store.UpsertSamples(ctx, samples); err != nil {
		return err
	}

	var events []Compaction
	for _, ev := range result.CompactEvents {
		if !ev.Timestamp.IsZero() && ev.Timestamp.Before(cutoff) {
			continue
		}
		events = append(events, Compaction{
			SessionUUID:  result.SessionUUID,
			TurnIndex:    ev.TurnIndex,
			OccurredAt:   ev.Timestamp,
			Trigger:      ev.Trigger,
			TokensBefore: ev.TokensBefore,
			TokensAfter:  ev.TokensAfter,
			TokensFreed:  ev.TokensFreed(),
		})
	}
	return r.store.UpsertCompactions(ctx, events)
}

// Run records every result already in src, then follows its updates until ctx
// is cancelled. It runs on its own goroutine consuming TokenStore's
// non-blocking notifications, so persistence never delays CapacityMonitor or
// the parse workers. Blocks; callers start it with `go`.
func (r *Recorder) Run(ctx context.Context, src tokens.TokenStoreReader) {
	ch := src.Subscribe()
	defer src.Unsubscribe(ch)

	r.sweep(ctx, src)
	r.prune(ctx)

	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case res, ok := <-ch:
			if !ok {
				return
			}
			if res == nil {
				// TokenStore's initial walk finished. Its notifications are
				// non-blocking and may have been dropped while we were busy, so
				// re-sweep; Record is incremental, making this cheap.
				r.sweep(ctx, src)
				continue
			}
			r.recordLogged(ctx, res)
		case <-prune.C:
			r.sweep(ctx, src)
			r.prune(ctx)
		}
	}
}

func (r *Recorder) sweep(ctx context.Context, src tokens.TokenStoreReader) {
	for _, res := range src.GetAll() {
		r.recordLogged(ctx, res)
	}
}

func (r *Recorder) prune(ctx context.Context) {
	if n, err := r.store.Prune(ctx, r.now().Add(-r.retention)); err != nil {
		log.Warn("[ContextHistory] prune failed", "err", err)
	} else if n > 0 {
		log.Info("[ContextHistory] pruned expired rows", "rows", n)
	}
}

func (r *Recorder) recordLogged(ctx context.Context, res *tokens.ParseResult) {
	if res == nil {
		return
	}
	if err := r.Record(ctx, res); err != nil {
		log.Warn("[ContextHistory] record failed", "session", res.SessionUUID, "err", err)
	}
}
