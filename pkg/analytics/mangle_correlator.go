package analytics

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Stage1Observation records a sequence seen at Stage 1 (PTY read) for later correlation with Stage 2.
type Stage1Observation struct {
	PayloadHash string
	ByteLen     int
	WallTime    time.Time
	SessionID   string
	SessionSeq  int64
}

// MangleCorrelator correlates Stage 1 and Stage 2 escape sequence observations to detect mangling.
// It holds Stage 1 observations in memory with a TTL; when Stage 2 arrives for the same sequence,
// it compares hashes to detect mutations or truncations. Observations not matched within the TTL
// are recorded as "stripped".
type MangleCorrelator struct {
	mu      sync.Mutex
	pending map[string]Stage1Observation // key: sessionID+":"+seqStr
	maxAge  time.Duration
	maxSize int
}

// NewMangleCorrelator creates a correlator with the given TTL and max pending size.
func NewMangleCorrelator(maxAge time.Duration, maxSize int) *MangleCorrelator {
	return &MangleCorrelator{
		pending: make(map[string]Stage1Observation),
		maxAge:  maxAge,
		maxSize: maxSize,
	}
}

// RecordStage1 records a Stage 1 observation for later correlation.
func (c *MangleCorrelator) RecordStage1(sessionID string, sessionSeq int64, hash string, byteLen int) {
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

	key := fmt.Sprintf("%s:%d", sessionID, sessionSeq)
	c.pending[key] = Stage1Observation{
		PayloadHash: hash,
		ByteLen:     byteLen,
		WallTime:    time.Now(),
		SessionID:   sessionID,
		SessionSeq:  sessionSeq,
	}
}

// CheckStage2 checks whether a Stage 2 observation matches the corresponding Stage 1 observation.
// Returns (mangled bool, mangleType string). mangleType is one of: "", "truncated", "mutated".
// If no Stage 1 observation is found, returns (false, "") — absence is not definitive.
func (c *MangleCorrelator) CheckStage2(sessionID string, sessionSeq int64, hash string, byteLen int) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%d", sessionID, sessionSeq)
	obs, ok := c.pending[key]
	if !ok {
		return false, ""
	}
	delete(c.pending, key)

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
	c.mu.Unlock()

	for _, obs := range expired {
		writer.WriteEscapeEvent(ctx, EscapeEventRecord{
			SessionID:    obs.SessionID,
			Stage:        StageTransport,
			SequenceType: "unknown",
			ByteLen:      obs.ByteLen,
			PayloadHash:  obs.PayloadHash,
			Mangled:      true,
			MangleType:   "stripped",
			WallTime:     obs.WallTime,
			SessionSeq:   obs.SessionSeq,
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
