// Package contexthistory durably records per-turn context-window occupancy and
// compaction events derived from Claude transcripts, so questions like "which
// sessions lived near the ceiling" are answerable after a restart or after the
// transcript is pruned.
package contexthistory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/compactionevent"
	"github.com/tstapler/stapler-squad/session/ent/contextsample"
)

// Default ceiling thresholds, matching CapacityMonitor's warn/transition defaults.
const (
	WarnFraction     = 0.75
	CriticalFraction = 0.90

	// insertChunk keeps bulk inserts under SQLite's bound-variable limit.
	insertChunk = 100
)

// Sample is one persisted turn of context occupancy.
type Sample struct {
	SessionUUID   string
	TurnIndex     int
	SampledAt     time.Time
	Model         string
	ContextTokens int64
	ContextMax    int64
}

// Compaction is one persisted compaction event.
type Compaction struct {
	SessionUUID  string
	Ordinal      int // position among the session's compactions; the idempotency key
	TurnIndex    int
	OccurredAt   time.Time
	Trigger      string
	TokensBefore int64
	TokensAfter  int64
	TokensFreed  int64
}

// SessionSummary answers "how much of its life did this session spend near the ceiling".
type SessionSummary struct {
	SessionUUID string
	Turns       int
	PeakTokens  int64
	// FractionWarn / FractionCritical are the share of turns at or above the
	// warn (75%) and critical (90%) fractions of ContextMax. Turns with an
	// unknown max (zero) are excluded from the fractions' numerator and denominator.
	FractionWarn     float64
	FractionCritical float64
}

// CompactionStats aggregates compaction telemetry.
type CompactionStats struct {
	Count           int
	ManualCount     int
	AutoCount       int
	TotalFreed      int64
	AverageFreed    int64
	SessionsWithAny int
}

// Store persists and queries context history on an already-migrated ent client.
type Store struct {
	client *ent.Client

	// nextIdx caches NextTurnIndex per session so the recorder's per-notification
	// hot path never reads context_samples. Writes through this Store raise
	// entries (samples are only ever upserted, so a cached value never exceeds
	// the table); Prune and failed writes drop them. cold flights coalesce
	// concurrent first reads of one session.
	mu      sync.Mutex
	nextIdx map[string]int
	gen     uint64 // bumped by every write/invalidation; fences cold fills
	cold    singleflight.Group
}

// NewStore wraps client; the caller must have run schema migration.
func NewStore(client *ent.Client) *Store {
	return &Store{client: client, nextIdx: make(map[string]int)}
}

// UpsertSamples inserts samples, updating existing (session, turn) rows. Safe to re-run.
func (s *Store) UpsertSamples(ctx context.Context, samples []Sample) (err error) {
	maxNext := make(map[string]int)
	for _, sm := range samples {
		maxNext[sm.SessionUUID] = max(maxNext[sm.SessionUUID], sm.TurnIndex+1)
	}
	// Chunks commit independently, so even a failed call may have written some
	// rows: on failure, drop the cached indexes rather than trust either value.
	defer func() { s.noteWrite(maxNext, err == nil) }()
	for start := 0; start < len(samples); start += insertChunk {
		end := min(start+insertChunk, len(samples))
		creates := make([]*ent.ContextSampleCreate, 0, end-start)
		for _, sm := range samples[start:end] {
			creates = append(creates, s.client.ContextSample.Create().
				SetSessionUUID(sm.SessionUUID).
				SetTurnIndex(sm.TurnIndex).
				SetSampledAt(sm.SampledAt).
				SetModel(sm.Model).
				SetContextTokens(sm.ContextTokens).
				SetContextMax(sm.ContextMax))
		}
		err := s.client.ContextSample.CreateBulk(creates...).
			OnConflictColumns(contextsample.FieldSessionUUID, contextsample.FieldTurnIndex).
			UpdateModel().UpdateContextTokens().UpdateContextMax().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("upsert context samples: %w", err)
		}
	}
	return nil
}

// UpsertCompactions inserts events, updating existing (session, turn) rows. Safe to re-run.
func (s *Store) UpsertCompactions(ctx context.Context, events []Compaction) error {
	for start := 0; start < len(events); start += insertChunk {
		end := min(start+insertChunk, len(events))
		creates := make([]*ent.CompactionEventCreate, 0, end-start)
		for _, ev := range events[start:end] {
			creates = append(creates, s.client.CompactionEvent.Create().
				SetSessionUUID(ev.SessionUUID).
				SetOrdinal(ev.Ordinal).
				SetTurnIndex(ev.TurnIndex).
				SetOccurredAt(ev.OccurredAt).
				SetTrigger(ev.Trigger).
				SetTokensBefore(ev.TokensBefore).
				SetTokensAfter(ev.TokensAfter).
				SetTokensFreed(ev.TokensFreed))
		}
		err := s.client.CompactionEvent.CreateBulk(creates...).
			OnConflictColumns(compactionevent.FieldSessionUUID, compactionevent.FieldOrdinal).
			UpdateTurnIndex().UpdateTrigger().UpdateTokensBefore().UpdateTokensAfter().UpdateTokensFreed().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("upsert compaction events: %w", err)
		}
	}
	return nil
}

// NextTurnIndex returns one past the highest stored turn index for a session
// (0 when none), letting callers skip already-persisted turns. Served from an
// in-memory cache that assumes this Store is the table's only writer.
func (s *Store) NextTurnIndex(ctx context.Context, sessionUUID string) (int, error) {
	if n, ok := s.cachedNext(sessionUUID); ok {
		return n, nil
	}
	v, err, _ := s.cold.Do(sessionUUID, func() (any, error) {
		s.mu.Lock()
		genAtStart := s.gen
		s.mu.Unlock()
		// The flight is shared, so it must not die with its first caller's ctx.
		n, err := s.queryNextTurnIndex(context.WithoutCancel(ctx), sessionUUID)
		if err != nil {
			return 0, err
		}
		s.mu.Lock()
		// A write that landed while we were reading may not be reflected in n.
		if s.gen == genAtStart {
			s.nextIdx[sessionUUID] = n
		}
		s.mu.Unlock()
		return flight{n: n, gen: genAtStart}, nil
	})
	if err != nil {
		return 0, err
	}
	f := v.(flight)
	n := f.n
	if cached, ok := s.cachedNext(sessionUUID); ok {
		// A joined flight can predate a write that has since raised the cache.
		return max(n, cached), nil
	}
	s.mu.Lock()
	wroteSince := s.gen != f.gen
	s.mu.Unlock()
	if wroteSince {
		// A write landed after the flight began and nothing was cached to
		// correct it, so its value may predate the write: read the table directly.
		return s.queryNextTurnIndex(ctx, sessionUUID)
	}
	return n, nil
}

// flight is a cold read's result and the write generation it started at.
type flight struct {
	n   int
	gen uint64
}

func (s *Store) cachedNext(sessionUUID string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nextIdx[sessionUUID]
	return n, ok
}

// noteWrite applies a completed UpsertSamples to the cache.
func (s *Store) noteWrite(maxNext map[string]int, ok bool) {
	if len(maxNext) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	for uuid, next := range maxNext {
		cached, have := s.nextIdx[uuid]
		switch {
		case !ok:
			delete(s.nextIdx, uuid)
		case have:
			s.nextIdx[uuid] = max(cached, next)
		}
	}
}

// invalidateNext drops every cached index (after rows are deleted).
func (s *Store) invalidateNext() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	clear(s.nextIdx)
}

func (s *Store) queryNextTurnIndex(ctx context.Context, sessionUUID string) (int, error) {
	last, err := s.client.ContextSample.Query().
		Where(contextsample.SessionUUIDEQ(sessionUUID)).
		Order(ent.Desc(contextsample.FieldTurnIndex)).First(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query next turn index: %w", err)
	}
	return last.TurnIndex + 1, nil
}

// Series returns a session's samples in turn order.
func (s *Store) Series(ctx context.Context, sessionUUID string) ([]Sample, error) {
	rows, err := s.client.ContextSample.Query().
		Where(contextsample.SessionUUIDEQ(sessionUUID)).
		Order(ent.Asc(contextsample.FieldTurnIndex)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query context series: %w", err)
	}
	out := make([]Sample, len(rows))
	for i, r := range rows {
		out[i] = sampleFromRow(r)
	}
	return out, nil
}

// SummarizeSession computes ceiling-time fractions for one stored session; zero value if no samples.
func (s *Store) SummarizeSession(ctx context.Context, sessionUUID string) (SessionSummary, error) {
	series, err := s.Series(ctx, sessionUUID)
	if err != nil {
		return SessionSummary{}, err
	}
	return Summarize(sessionUUID, series), nil
}

// RankByCeilingTime returns up to limit sessions ordered by FractionCritical then
// FractionWarn descending. limit <= 0 returns all.
func (s *Store) RankByCeilingTime(ctx context.Context, limit int) ([]SessionSummary, error) {
	// Ranking needs every session's samples; row count is bounded by retention.
	rows, err := s.client.ContextSample.Query(). //nolint:entfullscan
							Order(ent.Asc(contextsample.FieldSessionUUID), ent.Asc(contextsample.FieldTurnIndex)).
							All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query context samples: %w", err)
	}
	bySession := map[string][]Sample{}
	for _, r := range rows {
		bySession[r.SessionUUID] = append(bySession[r.SessionUUID],
			sampleFromRow(r))
	}
	out := make([]SessionSummary, 0, len(bySession))
	for id, series := range bySession {
		out = append(out, Summarize(id, series))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FractionCritical != out[j].FractionCritical {
			return out[i].FractionCritical > out[j].FractionCritical
		}
		if out[i].FractionWarn != out[j].FractionWarn {
			return out[i].FractionWarn > out[j].FractionWarn
		}
		return out[i].SessionUUID < out[j].SessionUUID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Summarize computes ceiling-time fractions for an in-memory series.
func Summarize(sessionUUID string, series []Sample) SessionSummary {
	sum := SessionSummary{SessionUUID: sessionUUID, Turns: len(series)}
	var known, warn, crit int
	for _, sm := range series {
		sum.PeakTokens = max(sum.PeakTokens, sm.ContextTokens)
		if sm.ContextMax <= 0 {
			continue
		}
		known++
		frac := float64(sm.ContextTokens) / float64(sm.ContextMax)
		if frac >= WarnFraction {
			warn++
		}
		if frac >= CriticalFraction {
			crit++
		}
	}
	if known > 0 {
		sum.FractionWarn = float64(warn) / float64(known)
		sum.FractionCritical = float64(crit) / float64(known)
	}
	return sum
}

// Compactions lists a session's compaction events in turn order.
func (s *Store) Compactions(ctx context.Context, sessionUUID string) ([]Compaction, error) {
	rows, err := s.client.CompactionEvent.Query().
		Where(compactionevent.SessionUUIDEQ(sessionUUID)).
		Order(ent.Asc(compactionevent.FieldOrdinal)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query compactions: %w", err)
	}
	return toCompactions(rows), nil
}

// CompactionStats aggregates across all sessions, optionally limited to events at or after since.
func (s *Store) CompactionStats(ctx context.Context, since time.Time) (CompactionStats, error) {
	q := s.client.CompactionEvent.Query()
	if !since.IsZero() {
		q = q.Where(compactionevent.OccurredAtGTE(since))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return CompactionStats{}, fmt.Errorf("query compaction stats: %w", err)
	}
	var st CompactionStats
	sessions := map[string]struct{}{}
	for _, r := range rows {
		st.Count++
		st.TotalFreed += r.TokensFreed
		sessions[r.SessionUUID] = struct{}{}
		switch r.Trigger {
		case "manual":
			st.ManualCount++
		case "auto":
			st.AutoCount++
		}
	}
	st.SessionsWithAny = len(sessions)
	if st.Count > 0 {
		st.AverageFreed = st.TotalFreed / int64(st.Count)
	}
	return st, nil
}

func sampleFromRow(r *ent.ContextSample) Sample {
	return Sample{
		SessionUUID: r.SessionUUID, TurnIndex: r.TurnIndex, SampledAt: r.SampledAt,
		Model: r.Model, ContextTokens: r.ContextTokens, ContextMax: r.ContextMax,
	}
}

func toCompactions(rows []*ent.CompactionEvent) []Compaction {
	out := make([]Compaction, len(rows))
	for i, r := range rows {
		out[i] = Compaction{
			SessionUUID: r.SessionUUID, Ordinal: r.Ordinal, TurnIndex: r.TurnIndex, OccurredAt: r.OccurredAt,
			Trigger: r.Trigger, TokensBefore: r.TokensBefore, TokensAfter: r.TokensAfter,
			TokensFreed: r.TokensFreed,
		}
	}
	return out
}

// Prune deletes samples and compaction events older than cutoff and returns the
// number of rows removed. This is the retention bound.
func (s *Store) Prune(ctx context.Context, cutoff time.Time) (int, error) {
	n1, err := s.client.ContextSample.Delete().
		Where(contextsample.SampledAtLT(cutoff)).Exec(ctx)
	if n1 > 0 || err != nil {
		s.invalidateNext()
	}
	if err != nil {
		return 0, fmt.Errorf("prune context samples: %w", err)
	}
	n2, err := s.client.CompactionEvent.Delete().
		Where(compactionevent.OccurredAtLT(cutoff)).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune compaction events: %w", err)
	}
	return n1 + n2, nil
}
