package services

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"
)

// reply_idempotency.go holds the reply_id result cache and in-flight map of
// ADR-010 decision 3: a repeated reply_id waits for the first attempt and
// returns its result; after a terminal result the cached result is returned.
const (
	replyCacheMax = 1024
	replyCacheTTL = 10 * time.Minute // INFERRED
)

// replyPayloadHash binds a reply_id to its payload, so reusing the id for a
// different session, question or digit is an InvalidArgument.
type replyPayloadHash [sha256.Size]byte

func hashReplyPayload(sessionUUID, questionID string, digit byte) replyPayloadHash {
	h := sha256.New()
	h.Write([]byte(sessionUUID))
	h.Write([]byte{0})
	h.Write([]byte(questionID))
	h.Write([]byte{0})
	h.Write([]byte{digit})
	var out replyPayloadHash
	copy(out[:], h.Sum(nil))
	return out
}

type replyBeginRole uint8

const (
	// replyLeader: the caller runs the reply and must call finish.
	replyLeader replyBeginRole = iota
	// replyFollower: another attempt is in flight; wait on it.
	replyFollower
	// replyCached: a terminal result exists.
	replyCached
	// replyMismatch: the id was used with a different payload.
	replyMismatch
)

type replyFlight struct {
	hash     replyPayloadHash
	done     chan struct{}
	res      replyOutcomeResult
	finished bool
	at       time.Time
}

// wait returns the attempt's result, or ctx's error.
func (f *replyFlight) wait(ctx context.Context) (replyOutcomeResult, error) {
	select {
	case <-f.done:
		return f.res, nil
	case <-ctx.Done():
		return replyOutcomeResult{}, ctx.Err()
	}
}

type replyFlightTable struct {
	mu      sync.Mutex
	now     func() time.Time
	flights map[string]*replyFlight
	order   []string
}

func newReplyFlightTable(now func() time.Time) *replyFlightTable {
	if now == nil {
		now = time.Now
	}
	return &replyFlightTable{now: now, flights: map[string]*replyFlight{}}
}

// begin classifies replyID. A leader owns a new in-flight entry.
func (t *replyFlightTable) begin(replyID string, h replyPayloadHash) (*replyFlight, replyBeginRole) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if f := t.flights[replyID]; f != nil {
		switch {
		case f.hash != h:
			return nil, replyMismatch
		case !f.finished:
			return f, replyFollower
		case now.Sub(f.at) <= replyCacheTTL:
			return f, replyCached
		}
		delete(t.flights, replyID)
	}
	f := &replyFlight{hash: h, done: make(chan struct{}), at: now}
	t.flights[replyID] = f
	t.order = append(t.order, replyID)
	t.evictLocked()
	return f, replyLeader
}

// finish publishes the result to every waiter. Only a terminal result that a
// Retry must not re-attempt is kept (cache); otherwise the id is forgotten, so
// a Retry after NOT_SENT attempts the write again.
func (t *replyFlightTable) finish(replyID string, f *replyFlight, res replyOutcomeResult, cache bool) {
	t.mu.Lock()
	f.res, f.finished, f.at = res, true, t.now()
	if !cache && t.flights[replyID] == f {
		delete(t.flights, replyID)
	}
	t.mu.Unlock()
	close(f.done)
}

// evictLocked drops the oldest finished entries beyond the bound; an in-flight
// entry is never evicted.
func (t *replyFlightTable) evictLocked() {
	for len(t.flights) > replyCacheMax {
		evicted := false
		for i, id := range t.order {
			if f := t.flights[id]; f == nil {
				t.order = append(t.order[:i], t.order[i+1:]...)
				evicted = true
				break
			} else if f.finished {
				delete(t.flights, id)
				t.order = append(t.order[:i], t.order[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			return
		}
	}
	if len(t.order) > 2*replyCacheMax {
		kept := t.order[:0]
		for _, id := range t.order {
			if t.flights[id] != nil {
				kept = append(kept, id)
			}
		}
		t.order = kept
	}
}

// size is the number of tracked ids (tests).
func (t *replyFlightTable) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.flights)
}
