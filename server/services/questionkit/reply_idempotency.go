package questionkit

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

// ReplyPayloadHash binds a reply_id to its payload, so reusing the id for a
// different session, question or digit is an InvalidArgument.
type ReplyPayloadHash [sha256.Size]byte

// HashReplyPayload hashes the payload a reply_id is bound to.
func HashReplyPayload(sessionUUID, questionID string, digit byte) ReplyPayloadHash {
	h := sha256.New()
	h.Write([]byte(sessionUUID))
	h.Write([]byte{0})
	h.Write([]byte(questionID))
	h.Write([]byte{0})
	h.Write([]byte{digit})
	var out ReplyPayloadHash
	copy(out[:], h.Sum(nil))
	return out
}

// ReplyBeginRole is what Begin tells the caller to do.
type ReplyBeginRole uint8

const (
	// ReplyLeader: the caller runs the reply and must call finish.
	ReplyLeader ReplyBeginRole = iota
	// replyFollower: another attempt is in flight; wait on it.
	ReplyFollower
	// replyCached: a terminal result exists.
	ReplyCached
	// replyMismatch: the id was used with a different payload.
	ReplyMismatch
)

// ReplyFlight is one reply attempt, shared by its leader and followers.
type ReplyFlight struct {
	hash     ReplyPayloadHash
	done     chan struct{}
	res      ReplyOutcomeResult
	finished bool
	at       time.Time
}

// Wait returns the attempt's result, or ctx's error.
func (f *ReplyFlight) Wait(ctx context.Context) (ReplyOutcomeResult, error) {
	select {
	case <-f.done:
		return f.res, nil
	case <-ctx.Done():
		return ReplyOutcomeResult{}, ctx.Err()
	}
}

// ReplyFlightTable tracks in-flight and recently finished replies by reply_id.
type ReplyFlightTable struct {
	mu      sync.Mutex
	now     func() time.Time
	flights map[string]*ReplyFlight
	order   []string
}

// NewReplyFlightTable builds a table on now (time.Now when nil).
func NewReplyFlightTable(now func() time.Time) *ReplyFlightTable {
	if now == nil {
		now = time.Now
	}
	return &ReplyFlightTable{now: now, flights: map[string]*ReplyFlight{}}
}

// Begin classifies replyID. A leader owns a new in-flight entry.
func (t *ReplyFlightTable) Begin(replyID string, h ReplyPayloadHash) (*ReplyFlight, ReplyBeginRole) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if f := t.flights[replyID]; f != nil {
		switch {
		case f.hash != h:
			return nil, ReplyMismatch
		case !f.finished:
			return f, ReplyFollower
		case now.Sub(f.at) <= replyCacheTTL:
			return f, ReplyCached
		}
		delete(t.flights, replyID)
	}
	f := &ReplyFlight{hash: h, done: make(chan struct{}), at: now}
	t.flights[replyID] = f
	t.order = append(t.order, replyID)
	t.evictLocked()
	return f, ReplyLeader
}

// Finish publishes the result to every waiter. Only a terminal result that a
// Retry must not re-attempt is kept (cache); otherwise the id is forgotten, so
// a Retry after NOT_SENT attempts the write again.
func (t *ReplyFlightTable) Finish(replyID string, f *ReplyFlight, res ReplyOutcomeResult, cache bool) {
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
func (t *ReplyFlightTable) evictLocked() {
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

// Size is the number of tracked ids (tests).
func (t *ReplyFlightTable) Size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.flights)
}
