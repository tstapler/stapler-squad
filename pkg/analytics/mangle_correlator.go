package analytics

import (
	"context"
	"sync"
	"time"
)

// Stage1Observation records a sequence seen at Stage 1 (PTY read) for later correlation with Stage 2.
type Stage1Observation struct {
	PayloadHash  string
	ByteLen      int
	WallTime     time.Time
	SessionID    string
	SequenceType string
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
	ordinalLastSeen map[ordinalKey]time.Time
	maxAge          time.Duration
	maxSize         int
}

// NewMangleCorrelator creates a correlator with the given TTL and max pending size.
func NewMangleCorrelator(maxAge time.Duration, maxSize int) *MangleCorrelator {
	return &MangleCorrelator{
		pending:         make(map[pendingKey]Stage1Observation),
		stage1Ordinals:  make(map[ordinalKey]int64),
		stage2Ordinals:  make(map[ordinalKey]int64),
		ordinalLastSeen: make(map[ordinalKey]time.Time),
		maxAge:          maxAge,
		maxSize:         maxSize,
	}
}

// RecordStage1 records a Stage 1 observation for later correlation. It is assigned the next
// ordinal for this (sessionID, sequenceType) pair.
func (c *MangleCorrelator) RecordStage1(sessionID, sequenceType, hash string, byteLen int) {
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
		PayloadHash:  hash,
		ByteLen:      byteLen,
		WallTime:     time.Now(),
		SessionID:    sessionID,
		SequenceType: sequenceType,
	}
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
func (c *MangleCorrelator) EvictExpired(ctx context.Context, writer EscapeEventWriter) {
	c.mu.Lock()
	var expired []Stage1Observation
	cutoff := time.Now().Add(-c.maxAge)
	for key, obs := range c.pending {
		if obs.WallTime.Before(cutoff) {
			expired = append(expired, obs)
			delete(c.pending, key)
		}
	}
	// Prune ordinal counters for (session, type) pairs that have gone quiet:
	// a session that ended (or a sequence type it stopped emitting) leaves
	// stage1Ordinals/stage2Ordinals entries with no further writer to ever
	// clean them up otherwise.
	for key, lastSeen := range c.ordinalLastSeen {
		if lastSeen.Before(cutoff) {
			delete(c.ordinalLastSeen, key)
			delete(c.stage1Ordinals, key)
			delete(c.stage2Ordinals, key)
		}
	}
	c.mu.Unlock()

	for _, obs := range expired {
		writer.WriteEscapeEvent(ctx, EscapeEventRecord{
			SessionID:    obs.SessionID,
			Stage:        StageTransport,
			SequenceType: obs.SequenceType,
			ByteLen:      obs.ByteLen,
			PayloadHash:  obs.PayloadHash,
			Mangled:      true,
			MangleType:   "stripped",
			WallTime:     obs.WallTime,
		})
	}
}

// StartEviction starts a background goroutine that calls EvictExpired periodically.
// Returns when ctx is cancelled.
func (c *MangleCorrelator) StartEviction(ctx context.Context, writer EscapeEventWriter) {
	ticker := time.NewTicker(c.maxAge / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.EvictExpired(ctx, writer)
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
