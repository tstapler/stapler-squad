package instance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/telemetry"
)

// WorktreeEnricher runs DetectWorktree, whose git-ref open() can block on an
// unreadable (e.g. TCC-protected) path, off the LoadInstances path. Workers only
// fill an in-memory result cache and never touch an Instance; instanceFromData
// reads the cache (pure memory) and applies a ready result to the instance it is
// still building. See docs/adr/ADR-038-post-start-worktree-enrichment.md.
//
// A bounded scanqueue pool drains one task per path; singleflight keeps a blocked
// path to one goroutine. A probe that outlives Deadline releases its worker and is
// retried with backoff (joining the blocked call) until it returns.
type WorktreeEnricher struct {
	cfg   WorktreeEnricherConfig
	queue *scanqueue.Queue
	group singleflight.Group

	mu      sync.Mutex
	results map[string]worktreeResult
	stuck   map[string]*stuckPath
	gens    map[string]uint64 // per-path generation; Recheck bumps it to invalidate in-flight probes

	cancel  context.CancelFunc
	stopped atomic.Bool

	// retrySlots bounds how many workers retries of already-stuck paths may hold
	// at once (half the pool), so a few blocked paths cannot starve the rest.
	retrySlots chan struct{}

	running                                       atomic.Int64
	timedOut, completed, errored, retriesDeferred atomic.Uint64
}

// WorktreeEnricherConfig tunes a WorktreeEnricher; zero values pick defaults.
type WorktreeEnricherConfig struct {
	Workers    int           // concurrent probes; default 4
	Deadline   time.Duration // wait per attempt before a path counts as stuck; default 10s
	BackoffMin time.Duration // first retry delay for a stuck path; default 5s
	BackoffMax time.Duration // retry delay ceiling; default 5m
	ResultTTL  time.Duration // how long a result is served from cache; default 5m
	MaxResults int           // cached-result cap; oldest evicted beyond it; default 1024

	// Ready gates a path before probing (extension point for a TCC consent gate).
	// Returning false parks the path: nothing runs until Unpark(path). Nil means
	// always ready.
	Ready func(path string) bool

	// OnResult observes every finished probe (not parked ones), outside locks.
	OnResult func(path string, err error)

	// Probe, AfterFunc and Now are injectable for deterministic tests.
	Probe     func(path string) (*WorktreeInfo, error)
	Timer     func(d time.Duration) (fired <-chan time.Time, stop func())
	AfterFunc func(d time.Duration, f func())
	Now       func() time.Time
}

type worktreeResult struct {
	info *WorktreeInfo
	err  error
	at   time.Time
}

type stuckPath struct {
	since        time.Time
	nextTry      time.Time
	backoff      time.Duration
	retryPending bool // a retry timer is already scheduled; never chain a second
}

// NewWorktreeEnricher builds an enricher. Call Start, and SetWorktreeEnricher to
// make LoadInstances use it.
func NewWorktreeEnricher(cfg WorktreeEnricherConfig) *WorktreeEnricher {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Deadline <= 0 {
		cfg.Deadline = 10 * time.Second
	}
	if cfg.BackoffMin <= 0 {
		cfg.BackoffMin = 5 * time.Second
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = 5 * time.Minute
	}
	if cfg.ResultTTL <= 0 {
		cfg.ResultTTL = 5 * time.Minute
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 1024
	}
	if cfg.Probe == nil {
		cfg.Probe = DetectWorktree
	}
	if cfg.Timer == nil {
		cfg.Timer = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTimer(d)
			return t.C, func() { t.Stop() }
		}
	}
	if cfg.AfterFunc == nil {
		cfg.AfterFunc = func(d time.Duration, f func()) { time.AfterFunc(d, f) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	e := &WorktreeEnricher{cfg: cfg, results: map[string]worktreeResult{}, stuck: map[string]*stuckPath{}, gens: map[string]uint64{}}
	e.retrySlots = make(chan struct{}, max(1, cfg.Workers/2))
	e.queue = scanqueue.New(scanqueue.Config{Workers: cfg.Workers}, e.handle)
	return e
}

// Start launches the worker pool; it stops on ctx cancellation or Stop.
func (e *WorktreeEnricher) Start(ctx context.Context) {
	ctx, e.cancel = context.WithCancel(ctx)
	e.queue.Start(ctx)
}

// Stop halts the workers; pending retry timers fire as no-ops. In-flight probes
// blocked in the kernel cannot be interrupted and are abandoned.
func (e *WorktreeEnricher) Stop() {
	e.stopped.Store(true)
	if e.cancel != nil {
		e.cancel()
	}
	activeWorktreeEnricher.CompareAndSwap(e, nil)
}

// SetWorktreeEnricher registers e as the process-wide enricher consulted by
// instance loading; nil restores synchronous detection.
func SetWorktreeEnricher(e *WorktreeEnricher) { activeWorktreeEnricher.Store(e) }

// ActiveWorktreeEnricher returns the registered enricher, or nil.
func ActiveWorktreeEnricher() *WorktreeEnricher { return activeWorktreeEnricher.Load() }

// StopWorktreeEnricher stops the registered enricher, if any (server shutdown hook).
func StopWorktreeEnricher() {
	if e := activeWorktreeEnricher.Load(); e != nil {
		e.Stop()
	}
}

// WorktreeLookup is a cached detection outcome: Info on success, Err on failure.
type WorktreeLookup struct {
	Info *WorktreeInfo
	Err  error
}

// Result returns the cached detection for path, if one is fresh. Memory only.
func (e *WorktreeEnricher) Result(path string) (WorktreeLookup, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.results[path]
	if !ok || e.cfg.Now().Sub(r.at) > e.cfg.ResultTTL {
		return WorktreeLookup{}, false
	}
	return WorktreeLookup{Info: r.info, Err: r.err}, true
}

// Request queues a background probe for path (coalesced; no I/O on the caller).
func (e *WorktreeEnricher) Request(path string) {
	if !e.stopped.Load() {
		e.queue.Enqueue(path)
	}
}

// Unpark requeues a path parked by Config.Ready.
func (e *WorktreeEnricher) Unpark(path string) { e.Request(path) }

// Recheck drops the cached result for path, invalidates any in-flight probe
// (its late result is discarded) and probes it again.
func (e *WorktreeEnricher) Recheck(path string) {
	e.mu.Lock()
	e.gens[path]++
	delete(e.results, path)
	delete(e.stuck, path)
	e.mu.Unlock()
	e.Request(path)
}

func (e *WorktreeEnricher) handle(path string) {
	if e.stopped.Load() {
		return
	}
	e.mu.Lock()
	sp := e.stuck[path]
	if sp != nil && e.cfg.Now().Before(sp.nextTry) {
		e.mu.Unlock() // backing off; the scheduled retry will come back
		return
	}
	gen := e.gens[path]
	e.mu.Unlock()

	start := e.cfg.Now()
	if sp != nil {
		select {
		case e.retrySlots <- struct{}{}:
			defer func() { <-e.retrySlots }()
		default:
			// Too many workers already wait on stuck paths: back off without waiting.
			e.retriesDeferred.Add(1)
			e.markStuck(path, gen, start)
			return
		}
	}
	e.running.Add(1)
	defer e.running.Add(-1)
	// The result is cached by the flight itself, so it lands even when this
	// handler already gave up on a stuck call.
	// Keyed by generation so a probe started after Recheck never joins a pre-Recheck flight.
	ch := e.group.DoChan(fmt.Sprintf("%s\x00%d", path, gen), func() (any, error) {
		info, err := e.safeProbe(path)
		if errors.Is(err, ErrEnrichmentParked) {
			e.mu.Lock()
			delete(e.stuck, path)
			e.mu.Unlock()
			return struct{}{}, nil
		}
		e.finish(path, gen, worktreeResult{info: info, err: err}, e.cfg.Now().Sub(start))
		return struct{}{}, nil
	})
	deadline, stopTimer := e.cfg.Timer(e.cfg.Deadline)
	defer stopTimer()
	select {
	case <-ch:
	case <-deadline:
		e.markStuck(path, gen, start)
	}
}

// safeProbe converts a panic in the probe into an error result, so a panicking
// probe cannot crash the process from a detached singleflight goroutine.
func (e *WorktreeEnricher) safeProbe(path string) (info *WorktreeInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			info, err = nil, fmt.Errorf("worktree probe panicked: %v", r)
		}
	}()
	if e.cfg.Ready != nil && !e.cfg.Ready(path) {
		return nil, ErrEnrichmentParked
	}
	if e.stopped.Load() {
		return nil, ErrEnrichmentParked
	}
	return e.cfg.Probe(path)
}

var ErrEnrichmentParked = errors.New("enrichment awaiting app consent")

// Outcome describes enrichment only; it does not infer an OS prompt or grant.
func (e *WorktreeEnricher) Outcome(path string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stuck[path] != nil {
		return "unresolved/slow"
	}
	if r, ok := e.results[path]; ok {
		if r.err != nil {
			return "error"
		}
		return "completed"
	}
	return "pending"
}

func (e *WorktreeEnricher) markStuck(path string, gen uint64, attemptStart time.Time) {
	e.mu.Lock()
	if e.gens[path] != gen {
		e.mu.Unlock() // superseded by Recheck: do not back the path off
		return
	}
	if r, ok := e.results[path]; ok && !r.at.Before(attemptStart) {
		e.mu.Unlock() // the probe finished just as the deadline fired
		return
	}
	now := e.cfg.Now()
	sp := e.stuck[path]
	if sp == nil {
		sp = &stuckPath{since: now, backoff: e.cfg.BackoffMin}
		e.stuck[path] = sp
	} else {
		sp.backoff = min(sp.backoff*2, e.cfg.BackoffMax)
	}
	sp.nextTry = now.Add(sp.backoff)
	schedule := !sp.retryPending
	sp.retryPending = true
	backoff, since := sp.backoff, sp.since
	e.mu.Unlock()

	log.Warn("worktree enrichment: path stuck, will retry", "path", path, "stuck_for", now.Sub(since), "retry_in", backoff)
	if schedule {
		e.cfg.AfterFunc(backoff, func() { e.retry(path) })
	}
	// Counted last so an observer of TimedOut sees the retry already scheduled.
	enrichmentResults.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", "timeout")))
	e.timedOut.Add(1)
}

func (e *WorktreeEnricher) retry(path string) {
	e.mu.Lock()
	if sp := e.stuck[path]; sp != nil {
		sp.retryPending = false
	}
	e.mu.Unlock()
	e.Request(path)
}

func (e *WorktreeEnricher) finish(path string, gen uint64, res worktreeResult, took time.Duration) {
	res.at = e.cfg.Now()
	e.mu.Lock()
	if e.gens[path] != gen {
		e.mu.Unlock() // a Recheck superseded this probe: drop its result entirely
		return
	}
	late := e.stuck[path] != nil
	if late {
		delete(e.stuck, path)
	}
	e.results[path] = res
	e.evictLocked(res.at)
	e.mu.Unlock()

	result := "ok"
	switch {
	case res.err != nil:
		result = "error"
	case late:
		result = "late"
	}
	if late {
		log.Info("worktree enrichment: stuck path finished (slow, not stuck)", "path", path, "took", took)
	}
	if e.cfg.OnResult != nil {
		e.cfg.OnResult(path, res.err)
	}
	e.completed.Add(1)
	if res.err != nil {
		e.errored.Add(1)
	}
	enrichmentResults.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", result)))
}

// WorktreeEnricherStats reports completed and timed-out probe counts.
type WorktreeEnricherStats struct{ Completed, TimedOut, Errors, RetriesDeferred uint64 }

// Stats returns probe counters since creation.
func (e *WorktreeEnricher) Stats() WorktreeEnricherStats {
	return WorktreeEnricherStats{Completed: e.completed.Load(), TimedOut: e.timedOut.Load(), Errors: e.errored.Load(), RetriesDeferred: e.retriesDeferred.Load()}
}

// evictLocked drops expired results, then the oldest ones beyond MaxResults.
func (e *WorktreeEnricher) evictLocked(now time.Time) {
	if len(e.results) <= e.cfg.MaxResults {
		return
	}
	for p, r := range e.results {
		if now.Sub(r.at) > e.cfg.ResultTTL {
			delete(e.results, p)
		}
	}
	for len(e.results) > e.cfg.MaxResults {
		var oldest string
		var oldestAt time.Time
		for p, r := range e.results {
			if oldest == "" || r.at.Before(oldestAt) {
				oldest, oldestAt = p, r.at
			}
		}
		delete(e.results, oldest)
	}
}

// StuckPaths is the number of paths currently past their deadline and unresolved.
func (e *WorktreeEnricher) StuckPaths() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.stuck)
}

// Gauges read the registered enricher, so they hold no cross-enricher state.
var (
	activeWorktreeEnricher atomic.Pointer[WorktreeEnricher]

	enrichmentResults = mustEnrichmentCounter()
)

func mustEnrichmentCounter() metric.Int64Counter {
	m := telemetry.GetMeter()
	c, err := m.Int64Counter("worktree_enrichment_total",
		metric.WithDescription("Background worktree detection probes by result (ok, late, error, timeout)"))
	if err != nil {
		panic(err)
	}
	of := func(read func(*WorktreeEnricher) int64) func() int64 {
		return func() int64 {
			if e := activeWorktreeEnricher.Load(); e != nil {
				return read(e)
			}
			return 0
		}
	}
	gauges := map[string]func() int64{
		"worktree_enrichment_running":     of(func(e *WorktreeEnricher) int64 { return e.running.Load() }),
		"worktree_enrichment_stuck_paths": of(func(e *WorktreeEnricher) int64 { return int64(e.StuckPaths()) }),
		"worktree_enrichment_queued":      of(func(e *WorktreeEnricher) int64 { return int64(e.queue.Pending()) }),
	}
	for name, read := range gauges {
		if _, err := m.Int64ObservableGauge(name, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(read())
			return nil
		})); err != nil {
			panic(err)
		}
	}
	return c
}
