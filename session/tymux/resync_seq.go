package tymux

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Resync reasons for tymux_stream_resync_total{reason}.
const (
	resyncReasonOutputGap = "output_gap" // tymuxd announced lost output
	resyncReasonSeqGap    = "seq_gap"    // a chunk's seq jumped with no announcement
	resyncReasonDrop      = "drop"       // a subscriber's queue overflowed
	resyncReasonRequest   = "request"    // a consumer asked for a full redraw (CapturePaneRedraw)
	resyncReasonResize    = "resize"     // the stream hub finished a negotiated resize
)

// Outcomes for tymux_stream_resync_total{outcome}.
const (
	resyncOutcomeOK        = "ok"
	resyncOutcomeCoalesced = "coalesced" // shared another caller's in-flight capture
	resyncOutcomeFresh     = "fresh"     // spliced from a recent capture plus the ring, no new capture
	resyncOutcomeRetry     = "retry"
	resyncOutcomeGaveUp    = "gave_up"
	resyncOutcomeFallback  = "fallback" // unsequenced stream: legacy, not exactly ordered
)

const (
	defaultResyncFreshFor      = 200 * time.Millisecond
	defaultResyncRetryBase     = 100 * time.Millisecond
	defaultResyncRetryCap      = 5 * time.Second
	defaultResyncMaxAttempts   = 8
	defaultResyncCaptureBudget = 5 * time.Second
)

var errUnsequenced = errors.New("tymux: resync: stream is not sequenced")

var errTailEvicted = errors.New("tymux: resync: output after the snapshot is no longer buffered")

// resyncSnap is one CapturePane result plus its rendered redraw.
type resyncSnap struct {
	epoch  uint64 // coordinator epoch when the capture started
	seqGen uint64 // fanout sequence generation when the capture started
	snap   *v1.PaneSnapshot
	redraw []byte
	at     time.Time
	gapGen uint64
}

func (r *resyncSnap) seq() (uint64, bool) {
	if r.snap.Seq == nil {
		return 0, false
	}
	return r.snap.GetSeq(), true
}

// resyncFlight is one in-flight capture shared by every caller that can use its result.
type resyncFlight struct {
	epoch  uint64 // coordinator epoch when the capture started
	revive bool
	done   chan struct{}
	res    *resyncSnap
	err    error
}

func (f *resyncFlight) wait(ctx context.Context) (*resyncSnap, error) {
	select {
	case <-f.done:
		return f.res, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// resyncCoordinator deduplicates captures and runs the retrying resync loop for one session.
// Zero tunables mean the defaults above; tests shrink them.
type resyncCoordinator struct {
	mu       sync.Mutex
	epoch    uint64 // bumped by every request that lost upstream output
	pending  uint64 // epoch the background loop must satisfy
	again    bool   // a request arrived since the current pass began
	flight   *resyncFlight
	last     *resyncSnap
	running  bool
	scopeAll bool

	captures atomic.Int64 // CapturePane RPCs issued by shared flights

	freshFor, retryBase, retryCap, captureBudget time.Duration
	maxAttempts                                  int
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

// sharedSnapshot returns a CapturePane result no older than the caller's need (an epoch from
// resyncCoordinator.epoch): it joins the in-flight capture when that started late enough, else
// waits it out and starts one. A caller that tolerates reviving a dead pane may only join a
// flight that revives; one that does not may join any.
func (s *tymuxGRPCSession) sharedSnapshot(ctx context.Context, need uint64, revive bool) (res *resyncSnap, joined bool, err error) {
	c := &s.resync
	for {
		c.mu.Lock()
		f := c.flight
		switch {
		case f != nil && f.epoch >= need && (f.revive || !revive):
			c.mu.Unlock()
			res, err = f.wait(ctx)
			return res, true, err
		case f != nil:
			c.mu.Unlock()
			_, _ = f.wait(ctx) // too early or incompatible to use; wait it out, then start our own
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			continue
		}
		f = &resyncFlight{epoch: c.epoch, revive: revive, done: make(chan struct{})}
		c.flight = f
		c.mu.Unlock()
		go s.runFlight(f)
		res, err = f.wait(ctx)
		return res, false, err
	}
}

func (s *tymuxGRPCSession) runFlight(f *resyncFlight) {
	c := &s.resync
	c.captures.Add(1)
	ctx, cancel := s.interruptibleContext()
	ctx, cancelTimeout := context.WithTimeout(ctx, orDefault(c.captureBudget, defaultResyncCaptureBudget))
	gapGen, seqGen := s.fanout.GapGeneration(), s.fanout.SeqGeneration()
	snap, err := s.capturePaneWith(ctx, 0, f.revive)
	cancelTimeout()
	cancel()
	if err == nil {
		var redraw string
		if redraw, err = RenderSnapshotRedraw(snap); err == nil {
			f.res = &resyncSnap{epoch: f.epoch, seqGen: seqGen, snap: snap, redraw: []byte(redraw), at: time.Now(), gapGen: gapGen}
		}
	}
	f.err = err
	c.mu.Lock()
	if err == nil {
		c.last = f.res
	}
	c.flight = nil
	c.mu.Unlock()
	close(f.done)
}

// redrawFor answers a consumer's request for a full redraw of the pane. Concurrent callers share
// one capture, so a burst of requests costs one CapturePane. The reply is the capture alone: it
// travels outside the output stream, which carries no marker to splice at, so adding the output
// that followed the capture would apply it twice once the stream delivers it. It is therefore as
// old as the shared capture, a few milliseconds at most, the same window a lone capture has.
func (s *tymuxGRPCSession) redrawFor(ctx context.Context) (*resyncSnap, error) {
	snap, joined, err := s.sharedSnapshot(ctx, 0, true)
	if err != nil {
		return nil, err
	}
	if joined {
		recordStreamResync(resyncReasonRequest, resyncOutcomeCoalesced)
	}
	return snap, nil
}

// requestStreamResync repairs every subscriber (ScopeAll, upstream output was lost) or only those
// that lost a frame (ScopeDropped) in the background. Requests arriving during a resync are
// folded into one more pass; a failed pass retries with capped, jittered exponential backoff.
// Only ScopeAll needs a capture taken after the request: output a subscriber dropped is still in
// the ring and is replayed after any snapshot.
func (s *tymuxGRPCSession) requestStreamResync(reason string, scope ResyncScope) {
	c := &s.resync
	c.mu.Lock()
	if scope == ScopeAll {
		c.epoch++
		c.pending = c.epoch
	}
	c.again = true
	c.scopeAll = c.scopeAll || scope == ScopeAll
	start := !c.running
	c.running = true
	c.mu.Unlock()
	if start {
		go s.runStreamResync(reason)
	}
}

func (s *tymuxGRPCSession) runStreamResync(reason string) {
	c := &s.resync
	for attempt := 1; ; attempt++ {
		c.mu.Lock()
		need, all := c.pending, c.scopeAll
		c.scopeAll, c.again = false, false
		c.mu.Unlock()

		scope := ScopeDropped
		if all {
			scope = ScopeAll
		}
		err := s.spliceResync(need, scope, reason)
		if err == nil {
			attempt = 0
			c.mu.Lock()
			if !c.again {
				c.running = false
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
			continue
		}

		c.mu.Lock()
		c.scopeAll = c.scopeAll || all
		c.mu.Unlock()
		if attempt >= orDefault(c.maxAttempts, defaultResyncMaxAttempts) || s.closing.Load() {
			recordStreamResync(reason, resyncOutcomeGaveUp)
			log.Error("tymux: stream resync gave up; subscribers stay stale until the next gap or drop",
				"session_id", s.GetSessionIdentifier(), "reason", reason, "attempts", attempt, "error", err)
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
			return
		}
		recordStreamResync(reason, resyncOutcomeRetry)
		if !s.waitResyncBackoff(context.Background(), attempt) {
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
			return
		}
	}
}

func (s *tymuxGRPCSession) waitResyncBackoff(ctx context.Context, attempt int) bool {
	c := &s.resync
	s.mu.RLock()
	abort := s.abortReconnect
	s.mu.RUnlock()
	d := reconnectBackoffDelay(attempt, orDefault(c.retryBase, defaultResyncRetryBase), orDefault(c.retryCap, defaultResyncRetryCap))
	select {
	case <-time.After(d):
		return true
	case <-abort:
		return false
	case <-ctx.Done():
		return false
	}
}

// spliceResync captures (sharing an in-flight capture started after need) and splices the result
// into the subscribers' streams. It fails, changing nothing, if the capture fails or the output
// after it is no longer buffered.
func (s *tymuxGRPCSession) spliceResync(need uint64, scope ResyncScope, reason string) error {
	ctx, cancel := s.interruptibleContext()
	defer cancel()
	return s.spliceResyncCtx(ctx, need, scope, reason)
}

func (s *tymuxGRPCSession) spliceResyncCtx(ctx context.Context, need uint64, scope ResyncScope, reason string) error {
	snap, joined, err := s.snapshotForSplice(ctx, need, reason)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.liveness = snap.snap.GetLiveness()
	s.restartIsPending = false
	s.mu.Unlock()

	seq, hasSeq := snap.seq()
	var n int
	switch {
	case hasSeq && s.fanout.SeqValid():
		var ok bool
		if n, ok = s.fanout.ResyncAt(seq, snap.seqGen, snap.redraw, scope); !ok {
			s.resync.mu.Lock()
			s.resync.last = nil // too old to extend; the retry must capture again
			s.resync.mu.Unlock()
			return errTailEvicted
		}
	case reason == resyncReasonResize:
		return errUnsequenced // the hub must not treat an unordered broadcast as an exact splice
	case scope == ScopeAll:
		s.fanout.Broadcast(snap.redraw)
		recordStreamResync(reason, resyncOutcomeFallback)
		return nil
	default:
		n = s.fanout.RepaintDropped(snap.redraw)
		recordStreamResync(reason, resyncOutcomeFallback)
		return nil
	}
	if joined {
		recordStreamResync(reason, resyncOutcomeCoalesced)
	}
	recordStreamResync(reason, resyncOutcomeOK)
	if n > 0 {
		recordAttachEvent(attachEventSnapshotRedraw)
	}
	return nil
}

// snapshotForSplice reuses the last capture when it started after need, is under freshFor old
// and no output was lost since; ResyncAt then replays what followed it from the ring.
func (s *tymuxGRPCSession) snapshotForSplice(ctx context.Context, need uint64, reason string) (*resyncSnap, bool, error) {
	c := &s.resync
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	if last != nil && last.epoch >= need && time.Since(last.at) < orDefault(c.freshFor, defaultResyncFreshFor) &&
		last.gapGen == s.fanout.GapGeneration() && last.seqGen == s.fanout.SeqGeneration() {
		recordStreamResync(reason, resyncOutcomeFresh)
		return last, false, nil
	}
	return s.sharedSnapshot(ctx, need, false)
}

// SupportsInStreamResync reports whether ResyncInStream can splice exactly: the daemon numbers
// its output and a priming snapshot has seeded the sequence.
func (s *tymuxGRPCSession) SupportsInStreamResync() bool {
	return s.fanout.SeqValid()
}

// resizeResyncAttempts bounds ResyncInStream's retries; the caller has a legacy path to fall
// back to, so it should not wait out the full background backoff.
const resizeResyncAttempts = 3

// ResyncInStream captures the pane (after any resize the caller just applied) and replaces every
// subscriber's queue with the redraw followed by all output after the capture, so output that
// raced the capture is neither lost nor applied twice. It returns an error without changing
// anything if it cannot, and the caller falls back.
func (s *tymuxGRPCSession) ResyncInStream(ctx context.Context) error {
	if !s.fanout.SeqValid() {
		return errUnsequenced
	}
	c := &s.resync
	c.mu.Lock()
	c.epoch++
	need := c.epoch
	c.mu.Unlock()

	var err error
	for attempt := 1; attempt <= resizeResyncAttempts; attempt++ {
		if err = s.spliceResyncCtx(ctx, need, ScopeAll, resyncReasonResize); err == nil {
			return nil
		}
		if ctx.Err() != nil || attempt == resizeResyncAttempts {
			break
		}
		recordStreamResync(resyncReasonResize, resyncOutcomeRetry)
		if !s.waitResyncBackoff(ctx, attempt) {
			break
		}
	}
	recordStreamResync(resyncReasonResize, resyncOutcomeGaveUp)
	return err
}
