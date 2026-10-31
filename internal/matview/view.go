// Package matview keeps a materialized, event-fed cache of values computed
// from an expensive function (ADR-034). Change sources invalidate keys; a
// per-key debounce and a bounded, prioritised scheduler recompute them off the
// request path; readers get the cached value and subscribers get conflated
// updates. Get never runs the compute function on the caller's goroutine.
//
// Values are shared between readers and subscribers, so V must be treated as
// immutable once Compute returns it. Limiter methods run under the view's lock
// (TryAcquire) and must never call back into the view.
package matview

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Reason says why a key was invalidated.
type Reason string

// Invalidation reasons. AppWrite, Overflow and Lifecycle skip the debounce:
// the app knows the change just happened, or events were lost.
const (
	ReasonFS        Reason = "fs"
	ReasonAppWrite  Reason = "app_write"
	ReasonLifecycle Reason = "lifecycle"
	ReasonPR        Reason = "pr"
	ReasonBackstop  Reason = "backstop"
	ReasonOverflow  Reason = "overflow"
	ReasonResume    Reason = "resume"
)

func (r Reason) immediate() bool {
	return r == ReasonAppWrite || r == ReasonOverflow || r == ReasonLifecycle
}

var (
	// ErrTimeout means a Get deadline passed with no value to fall back on.
	ErrTimeout = errors.New("matview: timed out waiting for compute")
	// ErrClosed means the view was closed.
	ErrClosed = errors.New("matview: closed")
	// ErrRemoved means the key was removed while a Get was waiting.
	ErrRemoved = errors.New("matview: key removed")
)

// Versioned is a computed value with its identity.
type Versioned[V any] struct {
	Value V
	// Rev is a per-view counter that bumps only when the value's ETag changes.
	Rev        uint64
	ETag       string
	ComputedAt time.Time
	// Stale marks a value returned after a failed or timed-out refresh. It is
	// set on the returned copy only, never stored.
	Stale bool
}

// Update is delivered to subscribers.
type Update[K comparable, V any] struct {
	Key K
	Versioned[V]
	Removed bool
}

// GetOpts tunes a Get.
type GetOpts struct {
	// WaitFresh waits for a compute that started after this call.
	WaitFresh bool
	// Deadline overrides Config.GetDeadline when non-zero.
	Deadline time.Duration
}

// Limiter bounds concurrent computes and may be shared with other users of the
// same resource (the VCS request-path flights). TryAcquire must not block.
type Limiter interface {
	TryAcquire() bool
	Release()
}

// ChangeSource reports changed keys. Subscribe returns a stop function.
type ChangeSource[K comparable] interface {
	Subscribe(fn func(K, Reason)) (stop func())
}

// Config configures a View. Compute and ETag are required.
type Config[K comparable, V any] struct {
	Compute func(ctx context.Context, k K) (V, error)
	ETag    func(V) string

	Clock   Clock
	Limiter Limiter

	Quiet   time.Duration // trailing debounce; default 300ms
	MaxWait time.Duration // forced compute under continuous events; default 2s

	FreshTTL time.Duration // unwatched entry served as-is; default 15s
	StaleTTL time.Duration // unwatched entry served while refreshing; default 60s

	GetDeadline    time.Duration // default 10s
	ComputeTimeout time.Duration // default 30s
	RetryDelay     time.Duration // re-check when the limiter is saturated; default 50ms
	ErrorRetry     time.Duration // retry of a pinned key whose first compute failed; default 5s

	MaxConcurrent  int           // used when Limiter is nil; default 4
	UnpinnedCap    int           // default 256
	MaxPinned      int           // default 512
	MaxKeysPerSub  int           // default 64
	IdleEvictAfter time.Duration // default 10m; <0 disables the idle sweep
}

func (c *Config[K, V]) defaults() {
	setDur := func(p *time.Duration, d time.Duration) {
		if *p == 0 {
			*p = d
		}
	}
	setDur(&c.Quiet, 300*time.Millisecond)
	setDur(&c.MaxWait, 2*time.Second)
	setDur(&c.FreshTTL, 15*time.Second)
	setDur(&c.StaleTTL, 60*time.Second)
	setDur(&c.GetDeadline, 10*time.Second)
	setDur(&c.ComputeTimeout, 30*time.Second)
	setDur(&c.RetryDelay, 50*time.Millisecond)
	setDur(&c.ErrorRetry, 5*time.Second)
	setDur(&c.IdleEvictAfter, 10*time.Minute)
	setInt := func(p *int, n int) {
		if *p == 0 {
			*p = n
		}
	}
	setInt(&c.MaxConcurrent, 4)
	setInt(&c.UnpinnedCap, 256)
	setInt(&c.MaxPinned, 512)
	setInt(&c.MaxKeysPerSub, 64)
	if c.Clock == nil {
		c.Clock = RealClock{}
	}
	if c.Limiter == nil {
		c.Limiter = newSemaphore(c.MaxConcurrent)
	}
}

// Stats is a point-in-time copy of the view's counters, for metrics.
type Stats struct {
	Entries, Pinned, Queued, Running, Degraded     int
	Subscribers                                    int // live Subscribe streams
	Subscriptions                                  int // (stream, key) pairs
	EventsReceived, EventsCoalesced, EventsIgnored uint64
	Recomputes, RecomputeErrors, Evictions         uint64
	PinRejected                                    uint64
}

type waiter[V any] struct {
	since uint64
	done  chan waitResult[V]
}

type waitResult[V any] struct {
	v   Versioned[V]
	err error
}

type entry[K comparable, V any] struct {
	key      K
	cur      Versioned[V]
	has      bool
	pins     int
	degraded bool
	lastRead time.Time

	timer      Timer
	gen        uint64
	firstDirty time.Time
	queued     bool
	queuedAt   time.Time
	running    bool
	startSeq   uint64
	rerun      bool
	dirty      bool // an event arrived for an entry nobody watches or waits on; refresh on next read
	cancel     context.CancelFunc
	cancelled  bool      // abandon cancelled the running compute; its result is discarded
	failing    bool      // the last compute failed; the served value is Stale
	retryAfter time.Time // no reader-initiated refresh before this (failed refresh backoff)
	rerunNow   bool      // an immediate-reason event arrived during the running compute
	waiters    []*waiter[V]
	subs       map[*subscriber[K, V]]struct{}
}

// watched means events keep the entry current, so its age says nothing.
func (e *entry[K, V]) watched() bool { return e.pins > 0 && !e.degraded }

// View is the materialized view. The zero value is not usable; call New.
type View[K comparable, V any] struct {
	cfg Config[K, V]

	mu      sync.Mutex
	entries map[K]*entry[K, V]
	queued  map[*entry[K, V]]struct{}
	retry   Timer
	sweeper Timer
	stops   []func()
	seq     uint64
	rev     uint64
	pinned  int
	closed  bool
	done    chan struct{}
	zombies map[K]int // computes still running for entries dropped from the map
	streams int
	stats   Stats
}

// New builds a View. It panics if Compute or ETag is nil.
func New[K comparable, V any](cfg Config[K, V]) *View[K, V] {
	if cfg.Compute == nil || cfg.ETag == nil {
		panic("matview: Compute and ETag are required")
	}
	cfg.defaults()
	v := &View[K, V]{
		cfg:     cfg,
		entries: map[K]*entry[K, V]{},
		queued:  map[*entry[K, V]]struct{}{},
		zombies: map[K]int{},
		done:    make(chan struct{}),
	}
	if cfg.IdleEvictAfter > 0 {
		v.armSweepLocked()
	}
	return v
}

// Attach wires a change source to Invalidate. It is detached on Close.
func (v *View[K, V]) Attach(src ChangeSource[K]) {
	stop := src.Subscribe(v.Invalidate)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		stop()
		return
	}
	v.stops = append(v.stops, stop)
}

// Invalidate marks a known key dirty. Unknown keys are ignored: entries are
// created lazily by Get and Subscribe, never by events.
func (v *View[K, V]) Invalidate(k K, why Reason) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return
	}
	e, ok := v.entries[k]
	if !ok {
		v.stats.EventsIgnored++
		return
	}
	v.stats.EventsReceived++
	v.markDirtyLocked(e, why.immediate())
}

// Get returns the cached value for k, registering the key on first use. A
// miss, or WaitFresh, queues a compute at top priority and waits for it.
func (v *View[K, V]) Get(ctx context.Context, k K, opt GetOpts) (Versioned[V], error) {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return Versioned[V]{}, ErrClosed
	}
	e := v.entryLocked(k)
	now := v.cfg.Clock.Now()
	e.lastRead = now
	if !opt.WaitFresh && e.has {
		if cur, ok := v.serveLocked(e, now); ok {
			v.mu.Unlock()
			return cur, nil
		}
	}
	// A plain miss is satisfied by any compute, even one already running; only
	// WaitFresh needs one that starts after this call.
	w := &waiter[V]{done: make(chan waitResult[V], 1)}
	if opt.WaitFresh {
		v.seq++
		w.since = v.seq
	}
	e.waiters = append(e.waiters, w)
	v.requestLocked(e, opt.WaitFresh)
	v.mu.Unlock()

	deadline := opt.Deadline
	if deadline == 0 {
		deadline = v.cfg.GetDeadline
	}
	expired := make(chan struct{})
	t := v.cfg.Clock.AfterFunc(deadline, func() { close(expired) })
	defer t.Stop()

	select {
	case r := <-w.done:
		return r.v, r.err
	case <-expired:
		return v.abandon(e, w, ErrTimeout)
	case <-ctx.Done():
		return v.abandon(e, w, ctx.Err())
	case <-v.done:
		return Versioned[V]{}, ErrClosed
	}
}

// abandon drops a waiter that gave up. The current value, if any, is returned
// marked Stale; otherwise the cause is the error.
func (v *View[K, V]) abandon(e *entry[K, V], w *waiter[V], cause error) (Versioned[V], error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	select {
	case r := <-w.done: // completed concurrently
		return r.v, r.err
	default:
	}
	for i, x := range e.waiters {
		if x == w {
			e.waiters = append(e.waiters[:i], e.waiters[i+1:]...)
			break
		}
	}
	if len(e.waiters) == 0 && e.firstDirty.IsZero() {
		e.rerun = false // it was only set on this waiter's behalf
	}
	if len(e.waiters) == 0 && e.pins == 0 {
		v.cancelRunLocked(e) // nobody wants this result any more (#1264)
	}
	if e.has && errors.Is(cause, ErrTimeout) {
		cur := e.cur
		cur.Stale = true
		return cur, nil
	}
	return Versioned[V]{}, cause
}

// cancelRunLocked cancels a running compute and marks its result as discarded,
// so a reader arriving before it returns starts a fresh compute instead of
// joining the cancelled one.
func (v *View[K, V]) cancelRunLocked(e *entry[K, V]) {
	if e.running && !e.cancelled && e.cancel != nil {
		e.cancelled = true
		e.cancel()
	}
}

// served is the entry's value as readers see it.
func (e *entry[K, V]) served() Versioned[V] {
	cur := e.cur
	cur.Stale = cur.Stale || e.failing
	return cur
}

// serveLocked applies the freshness tiers. Watched entries are always served;
// unwatched ones are fresh up to FreshTTL, served while refreshing up to
// StaleTTL, and a miss beyond that.
func (v *View[K, V]) serveLocked(e *entry[K, V], now time.Time) (Versioned[V], bool) {
	if e.watched() {
		return e.served(), true
	}
	age := now.Sub(e.cur.ComputedAt)
	switch {
	case age <= v.cfg.FreshTTL && !e.dirty:
		return e.served(), true
	case age <= v.cfg.StaleTTL:
		if !now.Before(e.retryAfter) { // a failing refresh backs off instead of re-running per read
			v.requestLocked(e, false)
		}
		return e.served(), true
	}
	return Versioned[V]{}, false
}

// requestLocked starts a compute a waiter can trust: one that begins after the
// waiter registered.
func (v *View[K, V]) requestLocked(e *entry[K, V], needsNewer bool) {
	if e.running {
		// A running compute serves a new reader unless it may predate what the
		// reader needs: it is being cancelled, an event landed on it while nobody
		// was interested (dirty), or the caller wants a strictly newer result.
		if needsNewer || e.cancelled || e.dirty {
			e.rerun = true
			e.dirty = false
			if e.firstDirty.IsZero() {
				e.firstDirty = v.cfg.Clock.Now()
			}
		}
		return
	}
	v.enqueueLocked(e)
	v.pumpLocked()
}

func (v *View[K, V]) entryLocked(k K) *entry[K, V] {
	if e, ok := v.entries[k]; ok {
		return e
	}
	v.evictOverCapLocked(1) // make room first so the new entry can never evict itself
	e := &entry[K, V]{key: k, subs: map[*subscriber[K, V]]struct{}{}, lastRead: v.cfg.Clock.Now()}
	v.entries[k] = e
	return e
}

func (v *View[K, V]) markDirtyLocked(e *entry[K, V], immediate bool) {
	if e.pins == 0 && len(e.waiters) == 0 {
		e.dirty = true // nobody is reading: recompute lazily on the next read
		return
	}
	now := v.cfg.Clock.Now()
	if e.firstDirty.IsZero() {
		e.firstDirty = now
	}
	if e.running {
		e.rerun = true
		e.rerunNow = e.rerunNow || immediate
		v.stats.EventsCoalesced++
		return
	}
	if e.queued {
		v.stats.EventsCoalesced++
		return
	}
	if immediate {
		v.stopTimerLocked(e)
		v.enqueueLocked(e)
		v.pumpLocked()
		return
	}
	if e.timer != nil {
		v.stats.EventsCoalesced++
	}
	v.armDebounceLocked(e, now)
}

func (v *View[K, V]) armDebounceLocked(e *entry[K, V], now time.Time) {
	if e.firstDirty.IsZero() {
		e.firstDirty = now
	}
	delay := v.cfg.Quiet
	if remain := e.firstDirty.Add(v.cfg.MaxWait).Sub(now); remain < delay {
		delay = remain
	}
	if delay < 0 {
		delay = 0
	}
	v.armTimerLocked(e, delay)
}

func (v *View[K, V]) armTimerLocked(e *entry[K, V], delay time.Duration) {
	v.stopTimerLocked(e)
	gen := e.gen
	e.timer = v.cfg.Clock.AfterFunc(delay, func() { v.fire(e, gen) })
}

func (v *View[K, V]) stopTimerLocked(e *entry[K, V]) {
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
	e.gen++
}

func (v *View[K, V]) fire(e *entry[K, V], gen uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || e.gen != gen || e.timer == nil {
		return
	}
	e.timer = nil
	v.enqueueLocked(e)
	v.pumpLocked()
}

func (v *View[K, V]) enqueueLocked(e *entry[K, V]) {
	if e.queued {
		return
	}
	if e.running {
		return // the running compute refreshes the entry; rerun is set only by events and WaitFresh
	}
	v.stopTimerLocked(e)
	e.queued = true
	e.queuedAt = v.cfg.Clock.Now()
	v.queued[e] = struct{}{}
}

// better reports whether a should run before b: waiters first, then pinned
// entries, then the longest-queued.
func better[K comparable, V any](a, b *entry[K, V]) bool {
	if (len(a.waiters) > 0) != (len(b.waiters) > 0) {
		return len(a.waiters) > 0
	}
	if (a.pins > 0) != (b.pins > 0) {
		return a.pins > 0
	}
	return a.queuedAt.Before(b.queuedAt)
}

func (v *View[K, V]) pumpLocked() {
	for len(v.queued) > 0 {
		var best *entry[K, V]
		for e := range v.queued {
			if v.zombies[e.key] > 0 {
				continue // an abandoned compute for this key still runs; its exit pumps again
			}
			if best == nil || better(e, best) {
				best = e
			}
		}
		if best == nil {
			return
		}
		if !v.cfg.Limiter.TryAcquire() {
			v.armRetryLocked()
			return
		}
		delete(v.queued, best)
		best.queued = false
		best.running = true
		best.firstDirty = time.Time{}
		best.dirty = false
		v.seq++
		best.startSeq = v.seq
		ctx, cancel := context.WithTimeout(context.Background(), v.cfg.ComputeTimeout)
		best.cancel = cancel
		go v.run(ctx, best, best.startSeq)
	}
}

func (v *View[K, V]) armRetryLocked() {
	if v.retry != nil || v.closed {
		return
	}
	v.retry = v.cfg.Clock.AfterFunc(v.cfg.RetryDelay, func() {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.retry = nil
		if !v.closed {
			v.pumpLocked()
		}
	})
}

func (v *View[K, V]) run(ctx context.Context, e *entry[K, V], startSeq uint64) {
	val, etag, err := v.safeCompute(ctx, e.key)
	e.cancel() // release the timeout; e.cancel is only replaced after running clears

	v.mu.Lock()
	v.finishLocked(e, startSeq, val, etag, err)
	v.mu.Unlock()

	// Release outside the lock: the Limiter may be shared and must never be
	// able to deadlock against the view.
	v.releaseSlot()
	v.mu.Lock()
	v.pumpLocked()
	v.mu.Unlock()
}

func (v *View[K, V]) releaseSlot() {
	defer func() { _ = recover() }()
	v.cfg.Limiter.Release()
}

func (v *View[K, V]) finishLocked(e *entry[K, V], startSeq uint64, val V, etag string, err error) {
	e.running = false
	v.stats.Recomputes++
	if v.entries[e.key] != e || v.closed {
		if v.zombies[e.key]--; v.zombies[e.key] <= 0 {
			delete(v.zombies, e.key)
		}
		return
	}
	if e.cancelled {
		v.finishCancelledLocked(e)
		return
	}
	now := v.cfg.Clock.Now()
	if err != nil {
		v.stats.RecomputeErrors++
		e.failing = e.has
		e.retryAfter = now.Add(v.cfg.ErrorRetry)
		v.finishWaitersLocked(e, startSeq, e.served(), failureOrNil(e, err))
	} else {
		e.failing = false
		e.retryAfter = time.Time{}
		v.publishLocked(e, val, etag)
		v.finishWaitersLocked(e, startSeq, e.cur, nil)
	}
	rerun, rerunNow := e.rerun, e.rerunNow
	e.rerun, e.rerunNow = false, false
	switch {
	case len(e.waiters) > 0 || rerunNow:
		v.enqueueLocked(e)
	case rerun:
		v.armDebounceLocked(e, now)
	case err != nil && e.pins > 0:
		v.armTimerLocked(e, v.cfg.ErrorRetry)
	}
}

// finishCancelledLocked discards the result of a compute abandon cancelled.
// Interest that arrived since (waiters, pins, queued events) gets a new compute;
// otherwise the entry is marked dirty so the next read refreshes it.
func (v *View[K, V]) finishCancelledLocked(e *entry[K, V]) {
	e.cancelled = false
	wanted := len(e.waiters) > 0 || e.pins > 0 || e.rerun
	e.rerun, e.rerunNow = false, false
	if wanted {
		v.enqueueLocked(e)
		return
	}
	e.dirty = true
}

// safeCompute turns a panicking Compute into an error so the limiter slot and
// the entry's running flag are always released.
func (v *View[K, V]) safeCompute(ctx context.Context, k K) (val V, etag string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("matview: compute panicked: %v", r)
		}
	}()
	val, err = v.cfg.Compute(ctx, k)
	if err == nil {
		etag = v.cfg.ETag(val)
	}
	return val, etag, err
}

// failureOrNil hides a compute error behind a previous value: readers keep the
// last good status, marked Stale, rather than an error.
func failureOrNil[K comparable, V any](e *entry[K, V], err error) error {
	if e.has {
		return nil
	}
	return err
}

func (v *View[K, V]) publishLocked(e *entry[K, V], val V, etag string) {
	now := v.cfg.Clock.Now()
	changed := !e.has || e.cur.ETag != etag
	if changed {
		v.rev++
		e.cur = Versioned[V]{Value: val, Rev: v.rev, ETag: etag, ComputedAt: now}
	} else {
		e.cur.ComputedAt = now
	}
	e.has = true
	if changed {
		for s := range e.subs {
			s.put(Update[K, V]{Key: e.key, Versioned: e.cur})
		}
	}
}

func (v *View[K, V]) finishWaitersLocked(e *entry[K, V], startSeq uint64, cur Versioned[V], err error) {
	kept := e.waiters[:0]
	for _, w := range e.waiters {
		if w.since < startSeq {
			w.done <- waitResult[V]{v: cur, err: err}
			continue
		}
		kept = append(kept, w)
	}
	e.waiters = kept
}

// Remove drops a key. Subscribers get a terminal Removed update; waiters get
// ErrRemoved.
func (v *View[K, V]) Remove(k K) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e, ok := v.entries[k]; ok {
		for s := range e.subs {
			s.put(Update[K, V]{Key: k, Removed: true})
		}
		v.dropLocked(e, ErrRemoved)
	}
}

func (v *View[K, V]) dropLocked(e *entry[K, V], cause error) {
	v.stopTimerLocked(e)
	delete(v.queued, e)
	e.queued = false
	for _, w := range e.waiters {
		w.done <- waitResult[V]{err: cause}
	}
	e.waiters = nil
	if e.pins > 0 {
		v.pinned--
	}
	e.pins = 0
	if e.running {
		v.zombies[e.key]++
		if e.cancel != nil {
			e.cancel()
		}
	}
	delete(v.entries, e.key)
}

// SetDegraded marks a key whose change source is unavailable: its entry is
// treated as unwatched, so it falls back to the TTL tiers.
func (v *View[K, V]) SetDegraded(k K, degraded bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e, ok := v.entries[k]; ok {
		e.degraded = degraded
	}
}

// Stats returns a copy of the counters.
func (v *View[K, V]) Stats() Stats {
	v.mu.Lock()
	defer v.mu.Unlock()
	s := v.stats
	s.Entries, s.Pinned, s.Queued = len(v.entries), v.pinned, len(v.queued)
	s.Subscribers = v.streams
	for _, n := range v.zombies {
		s.Running += n
	}
	for _, e := range v.entries {
		if e.running {
			s.Running++
		}
		if e.degraded {
			s.Degraded++
		}
		s.Subscriptions += len(e.subs)
	}
	return s
}

// Close stops timers and sources and fails pending Gets. It is idempotent.
func (v *View[K, V]) Close() {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return
	}
	v.closed = true
	close(v.done)
	stops := v.stops
	v.stops = nil
	for _, e := range v.entries {
		v.stopTimerLocked(e)
		if e.running && e.cancel != nil {
			e.cancel()
		}
		for _, w := range e.waiters {
			w.done <- waitResult[V]{err: ErrClosed}
		}
		e.waiters = nil
	}
	if v.retry != nil {
		v.retry.Stop()
	}
	if v.sweeper != nil {
		v.sweeper.Stop()
	}
	v.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}
