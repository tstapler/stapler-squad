package analytics

import (
	"context"
	"sync"
	"time"
)

// Stage1Observation records a sequence seen at Stage 1 (PTY read) for later correlation with Stage 2.
type Stage1Observation struct {
	PayloadHash       string
	ByteLen           int
	WallTime          time.Time
	SessionID         string
	ProjectPath       string
	SequenceType      string
	SequenceSignature string
}

// MangleCorrelator correlates Stage 1 and Stage 2 escape sequence observations to detect
// mangling. It holds Stage 1 observations in memory with a TTL; when Stage 2 arrives for
// the same logical sequence, it compares hashes to detect mutations or truncations.
// Observations not matched within the TTL are recorded as "stripped".
//
// Stage 1 and Stage 2 are delivered by two independent tmux client attachments (a raw PTY
// attach and a control-mode "-C" attach) with no shared clock or byte-offset numbering
// between them — an earlier version of this correlator keyed on byte offset, which drifts
// whenever the two clients' independent connection/redraw timing diverges. Instead,
// sequences are correlated ORDINALLY per (session, sequence type): the Nth sequence of a
// given type seen at Stage 1 is assumed to be the same logical occurrence as the Nth
// sequence of that type seen at Stage 2, since tmux mirrors pane output to all attached
// clients in the same relative order. This is robust to timing drift but not to an actual
// dropped or duplicated sequence of that type, which desyncs every subsequent ordinal for
// that (session, type) pair — a real limitation, but an ongoing desync is itself a symptom
// worth surfacing (it shows up as a run of unexplained "stripped"/mismatched events) rather
// than a silently wrong byte-offset correlation.
// pendingKey is the zero-alloc map key for pending Stage 1 observations.
type pendingKey struct {
	sessionID    string
	sequenceType string
	ordinal      int64
}

// ordinalKey is the zero-alloc map key for per-(session, type) ordinal counters.
type ordinalKey struct {
	sessionID    string
	sequenceType string
}

type MangleCorrelator struct {
	mu      sync.Mutex
	pending map[pendingKey]Stage1Observation
	// stage1Ordinals/stage2Ordinals are independent per-(session,type) counters. They are
	// deliberately not shared: Stage 1 and Stage 2 are driven by different goroutines
	// observing different transports, and each counts only what it has itself seen.
	stage1Ordinals map[ordinalKey]int64
	stage2Ordinals map[ordinalKey]int64
	// ordinalLastSeen tracks the last time either ordinal counter for a given
	// (session, type) pair was touched. stage1Ordinals/stage2Ordinals have no
	// TTL of their own — a session's ordinal counters would otherwise grow
	// unboundedly for the life of the process, since EvictExpired only ever
	// pruned pending. Confirmed as the #1 live-heap consumer (25.18% inuse_space)
	// before this field was added: sessions come and go, but their ordinal
	// counters never did.
	ordinalLastSeen   map[ordinalKey]time.Time
	transportLastSeen map[string]time.Time
	maxAge            time.Duration
	maxSize           int
}

// NewMangleCorrelator creates a correlator with the given TTL and max pending size.
func NewMangleCorrelator(maxAge time.Duration, maxSize int) *MangleCorrelator {
	return &MangleCorrelator{
		pending:           make(map[pendingKey]Stage1Observation),
		stage1Ordinals:    make(map[ordinalKey]int64),
		stage2Ordinals:    make(map[ordinalKey]int64),
		ordinalLastSeen:   make(map[ordinalKey]time.Time),
		transportLastSeen: make(map[string]time.Time),
		maxAge:            maxAge,
		maxSize:           maxSize,
	}
}

// RecordStage1 records a Stage 1 observation for later correlation. It is assigned the next
// ordinal for this (sessionID, sequenceType) pair.
func (c *MangleCorrelator) RecordStage1(sessionID, sequenceType, hash string, byteLen int) {
	c.RecordStage1WithMetadata(sessionID, "", sequenceType, "", hash, byteLen)
}

// RecordStage1WithMetadata preserves project and normalized command attribution
// when an unmatched observation is later emitted as stripped.
func (c *MangleCorrelator) RecordStage1WithMetadata(sessionID, projectPath, sequenceType, sequenceSignature, hash string, byteLen int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entry when at capacity (simple FIFO approximation)
	if len(c.pending) >= c.maxSize {
		// Delete one arbitrary entry to make room
		for k := range c.pending {
			delete(c.pending, k)
			break
		}
	}

	ok := ordinalKey{sessionID, sequenceType}
	c.stage1Ordinals[ok]++
	ordinal := c.stage1Ordinals[ok]
	c.ordinalLastSeen[ok] = time.Now()

	c.pending[pendingKey{sessionID, sequenceType, ordinal}] = Stage1Observation{
		PayloadHash:       hash,
		ByteLen:           byteLen,
		WallTime:          time.Now(),
		SessionID:         sessionID,
		ProjectPath:       projectPath,
		SequenceType:      sequenceType,
		SequenceSignature: sequenceSignature,
	}
}

// ObserveTransport records that a transport is actively carrying output for
// the session. This prevents sessions with no consumer from being reported as
// stripped merely because Stage 2 was never expected.
func (c *MangleCorrelator) ObserveTransport(sessionID string) {
	c.mu.Lock()
	c.transportLastSeen[sessionID] = time.Now()
	c.mu.Unlock()
}

// CheckStage2 checks whether the next Stage 2 observation for this (sessionID, sequenceType)
// pair — the next ordinal — matches its corresponding Stage 1 observation.
// Returns (mangled bool, mangleType string). mangleType is one of: "", "truncated", "mutated".
// If no Stage 1 observation is found at that ordinal, returns (false, "") — absence is not
// definitive; the eviction pass handles the "never arrived" (stripped) case.
func (c *MangleCorrelator) CheckStage2(sessionID, sequenceType, hash string, byteLen int) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ok2 := ordinalKey{sessionID, sequenceType}
	c.stage2Ordinals[ok2]++
	ordinal := c.stage2Ordinals[ok2]
	c.ordinalLastSeen[ok2] = time.Now()

	pk := pendingKey{sessionID, sequenceType, ordinal}
	obs, ok := c.pending[pk]
	if !ok {
		return false, ""
	}
	delete(c.pending, pk)

	if obs.PayloadHash == hash {
		return false, ""
	}
	if byteLen < obs.ByteLen {
		return true, "truncated"
	}
	return true, "mutated"
}

// EvictExpired removes observations older than maxAge and emits them as "stripped" escape events.
// Call this periodically (e.g., every maxAge/2).
//
// Deliberately scans only c.pending, not the ordinal-counter maps (see PruneStaleOrdinals) —
// this runs on StartEviction's frequent maxAge/2 ticker (2.5s for the production TTL), so
// its critical section stays as short as live pprof/Pyroscope evidence showed matters: on a
// production instance running ~50-100 concurrent sessions, mutex-contention profiling
// attributed 1,229 events / 224.6s of waiter delay to this function — ~183ms average per
// event, far too long for a map scan this small to explain on its own. Cross-referencing the
// same window's CPU profile (17% in runtime.gcBgMarkWorker/scanSpan) and `uptime`'s load
// average (32 runnable threads on an 18-core machine) pins the real cause on scheduler/GC
// preemption while the lock is held, not this critical section's own cost — so the fix here
// is to minimize exposure (shrink what runs under the frequent lock), not to restructure the
// lock itself; the systemic fix is reducing allocation/goroutine pressure elsewhere.
func (c *MangleCorrelator) EvictExpired(ctx context.Context, writer EscapeEventWriter) {
	c.mu.Lock()
	var expired []Stage1Observation
	cutoff := time.Now().Add(-c.maxAge)
	for key, obs := range c.pending {
		if obs.WallTime.Before(cutoff) {
			// Absence is only evidence of stripping while a transport was
			// active near this observation. Otherwise discard the pending
			// correlation without manufacturing a false failure.
			if seen, ok := c.transportLastSeen[obs.SessionID]; ok && seen.After(obs.WallTime.Add(-c.maxAge)) {
				expired = append(expired, obs)
			}
			delete(c.pending, key)
		}
	}
	c.mu.Unlock()

	for _, obs := range expired {
		writer.WriteEscapeEvent(ctx, EscapeEventRecord{
			SessionID:         obs.SessionID,
			ProjectPath:       obs.ProjectPath,
			Stage:             StageTransport,
			SequenceType:      obs.SequenceType,
			SequenceSignature: obs.SequenceSignature,
			ByteLen:           obs.ByteLen,
			PayloadHash:       obs.PayloadHash,
			Mangled:           true,
			MangleType:        "stripped",
			WallTime:          obs.WallTime,
		})
	}
}

// PruneStaleOrdinals removes stage1Ordinals/stage2Ordinals/ordinalLastSeen entries for
// (session, type) pairs that have gone quiet for longer than maxAge: a session that ended
// (or a sequence type it stopped emitting) leaves these with no further writer to ever clean
// them up otherwise. Split out of EvictExpired (see its doc comment) so this second map scan
// doesn't add to the lock hold time of the frequent maxAge/2 eviction tick — StartEviction
// calls this on its own, much less frequent, ordinalPruneEveryNTicks cadence instead, since
// staleness here is only ever session-idle-scale, not sequence-TTL-scale.
func (c *MangleCorrelator) PruneStaleOrdinals() {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-c.maxAge)
	for key, lastSeen := range c.ordinalLastSeen {
		if lastSeen.Before(cutoff) {
			delete(c.ordinalLastSeen, key)
			delete(c.stage1Ordinals, key)
			delete(c.stage2Ordinals, key)
		}
	}
	for sessionID, lastSeen := range c.transportLastSeen {
		if lastSeen.Before(cutoff) {
			delete(c.transportLastSeen, sessionID)
		}
	}
}

// ordinalPruneEveryNTicks is how many EvictExpired ticks pass between PruneStaleOrdinals
// calls in StartEviction's loop. Ordinal staleness only matters at session-idle timescales,
// so it doesn't need the same maxAge/2 cadence pending-eviction does; 10 ticks keeps it
// comfortably faster than any reasonable session lifetime while cutting the frequent lock's
// critical section down to just the pending scan.
const ordinalPruneEveryNTicks = 10

// StartEviction starts a background goroutine that calls EvictExpired periodically, and
// PruneStaleOrdinals every ordinalPruneEveryNTicks-th tick. Returns when ctx is cancelled.
func (c *MangleCorrelator) StartEviction(ctx context.Context, writer EscapeEventWriter) {
	ticker := time.NewTicker(c.maxAge / 2)
	defer ticker.Stop()
	tick := 0
	for {
		select {
		case <-ticker.C:
			c.EvictExpired(ctx, writer)
			tick++
			if tick%ordinalPruneEveryNTicks == 0 {
				c.PruneStaleOrdinals()
			}
		case <-ctx.Done():
			return
		}
	}
}

// PendingCount returns the number of unmatched Stage 1 observations (for monitoring).
func (c *MangleCorrelator) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}
