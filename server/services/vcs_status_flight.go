package services

// vcs_status_flight.go coalesces concurrent GetVCSStatus cache misses for one
// workdir into a single git computation, bounds how many computations run at
// once across workdirs, and ties the computation's lifetime to its waiters.
//
// Why: the 15s TTL cache only helps callers that arrive after it is filled.
// Every poller that arrives while a computation is running (or just after the
// entry expires) used to start its own `git status` + numstat spawns, all
// contending on syscall.ForkLock and the index, so a burst slowed itself down.
// The computation also ran on context.Background() with a 30s timeout, so a
// browser-cancelled request kept burning CPU for up to 30s after the caller left.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/telemetry"
)

const (
	// maxConcurrentVCSComputations caps distinct-workdir git computations.
	// Each spawns several git processes; more than a handful in parallel just
	// queues on the runtime's fork lock.
	maxConcurrentVCSComputations = 4
	// vcsComputeTimeout bounds one shared computation regardless of waiters.
	vcsComputeTimeout = 30 * time.Second
)

// Outcomes recorded on vcs.status.requests. Bounded set: safe as a metric label.
const (
	vcsOutcomeCacheHit  = "cache_hit"
	vcsOutcomeStaleHit  = "stale_hit"
	vcsOutcomeComputed  = "computed"
	vcsOutcomeCoalesced = "coalesced"
	vcsOutcomeError     = "error"
	vcsOutcomeCanceled  = "canceled"
)

type vcsStatusMetrics struct {
	requests metric.Int64Counter
	compute  metric.Float64Histogram
	lockWait metric.Float64Histogram
	inFlight metric.Int64UpDownCounter
}

var vcsMetrics = sync.OnceValue(func() *vcsStatusMetrics {
	meter := telemetry.GetMeter()
	m := &vcsStatusMetrics{}
	var err error
	if m.requests, err = meter.Int64Counter("vcs.status.requests",
		metric.WithDescription("GetVCSStatus calls by outcome: cache_hit, stale_hit (stale entry served, refresh in background), computed (leader), coalesced (joined an in-flight computation), error, canceled")); err != nil {
		log.Error("services: register vcs.status.requests", "error", err)
	}
	if m.compute, err = meter.Float64Histogram("vcs.status.compute.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Wall time of one shared git status computation (excludes cache hits), labeled by outcome"),
		metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000)); err != nil {
		log.Error("services: register vcs.status.compute.duration", "error", err)
	}
	if m.lockWait, err = meter.Float64Histogram("vcs.status.queue_wait.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Time a computation waited for one of the concurrent-computation slots"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 50, 100, 500, 1000, 5000, 30000)); err != nil {
		log.Error("services: register vcs.status.queue_wait.duration", "error", err)
	}
	if m.inFlight, err = meter.Int64UpDownCounter("vcs.status.in_flight",
		metric.WithDescription("Git status computations currently running or queued for a slot")); err != nil {
		log.Error("services: register vcs.status.in_flight", "error", err)
	}
	return m
})

// vcsFlight is one in-progress computation shared by all callers of a workdir.
type vcsFlight struct {
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int // guarded by vcsFlights.mu
	status  *vc.VCSStatus
	err     error
}

// vcsFlights is a singleflight whose work is cancelled when the last waiter
// leaves, and not before: one cancelled caller must not fail the others.
// The zero value is ready to use.
type vcsFlights struct {
	initOnce sync.Once
	mu       sync.Mutex
	flights  map[string]*vcsFlight
	slots    chan struct{}
}

func (f *vcsFlights) init() {
	f.initOnce.Do(func() {
		f.flights = make(map[string]*vcsFlight)
		f.slots = make(chan struct{}, maxConcurrentVCSComputations)
	})
}

// do returns the result of compute for key, running it at most once for all
// concurrent callers. shared is true for callers that joined an existing run.
// If ctx ends first, do returns ctx's error; the computation is cancelled only
// if no other caller is still waiting.
func (f *vcsFlights) do(ctx context.Context, key string, compute func(context.Context) (*vc.VCSStatus, error)) (st *vc.VCSStatus, shared bool, err error) {
	f.init()
	f.mu.Lock()
	fl, ok := f.flights[key]
	if ok {
		fl.waiters++
		shared = true
	} else {
		// Detached from the leader's cancellation (it may leave while others wait) but
		// keeping its values, so the computation's spans stay in the leader's trace.
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), vcsComputeTimeout)
		fl = &vcsFlight{cancel: cancel, done: make(chan struct{}), waiters: 1}
		f.flights[key] = fl
		go f.run(workCtx, key, fl, compute)
	}
	f.mu.Unlock()

	select {
	case <-fl.done:
		return fl.status, shared, fl.err
	case <-ctx.Done():
		f.leave(key, fl)
		return nil, shared, ctx.Err()
	}
}

// forget detaches the running flight for key so the next caller starts a fresh one. The
// detached run finishes for its existing waiters.
func (f *vcsFlights) forget(key string) {
	f.init()
	f.mu.Lock()
	delete(f.flights, key)
	f.mu.Unlock()
}

func (f *vcsFlights) leave(key string, fl *vcsFlight) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl.waiters--
	if fl.waiters == 0 {
		fl.cancel()
		if f.flights[key] == fl {
			delete(f.flights, key) // a new caller starts fresh rather than joining a dying run
		}
	}
}

func (f *vcsFlights) run(ctx context.Context, key string, fl *vcsFlight, compute func(context.Context) (*vc.VCSStatus, error)) {
	m := vcsMetrics()
	start := time.Now()
	m.inFlight.Add(ctx, 1)
	defer m.inFlight.Add(context.WithoutCancel(ctx), -1)

	ctx, span := telemetry.GetTracer().Start(ctx, "vcs.status.compute")
	defer span.End()

	outcome := "ok"
	defer func() {
		// This goroutine is not covered by net/http's per-request recover.
		if r := recover(); r != nil {
			outcome = vcsOutcomeError
			fl.status, fl.err = nil, fmt.Errorf("failed to get VCS status: panic: %v", r)
			log.Error("vcs status computation panicked", "key", key, "panic", r)
			span.SetStatus(codes.Error, "panic")
		}
		m.compute.Record(context.WithoutCancel(ctx), float64(time.Since(start).Microseconds())/1000,
			metric.WithAttributes(attribute.String("result", outcome)))
		// Unlink before signalling so a caller arriving now starts fresh instead of joining a finished flight.
		f.mu.Lock()
		if f.flights[key] == fl {
			delete(f.flights, key)
		}
		f.mu.Unlock()
		close(fl.done)
		fl.cancel()
	}()

	waited, err := f.acquireSlot(ctx, m)
	if err != nil {
		outcome = vcsOutcomeCanceled
		fl.err = err
		span.AddEvent("canceled", trace.WithAttributes(attribute.String("stage", "queue_wait")))
		return
	}
	defer func() { <-f.slots }()

	fl.status, fl.err = compute(ctx)
	outcome = classifyVCSResult(ctx, fl.err, span)
	span.SetAttributes(attribute.Int64("queue_wait_ms", waited.Milliseconds()))
}

// acquireSlot blocks for a computation slot and records the wait.
func (f *vcsFlights) acquireSlot(ctx context.Context, m *vcsStatusMetrics) (time.Duration, error) {
	start := time.Now()
	_, span := telemetry.GetTracer().Start(ctx, "vcs.status.queue_wait")
	defer span.End()
	select {
	case f.slots <- struct{}{}:
	case <-ctx.Done():
		span.AddEvent("canceled_while_queued")
		return 0, ctx.Err()
	}
	waited := time.Since(start)
	m.lockWait.Record(ctx, float64(waited.Microseconds())/1000)
	return waited, nil
}

// classifyVCSResult maps a computation's result onto the bounded "result" label and marks span.
func classifyVCSResult(ctx context.Context, err error, span trace.Span) string {
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		span.AddEvent("canceled", trace.WithAttributes(attribute.String("stage", "git")))
		return vcsOutcomeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		span.SetStatus(codes.Error, "compute timeout")
		return "timeout"
	case err != nil:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return vcsOutcomeError
	}
	return "ok"
}

func recordVCSOutcome(ctx context.Context, outcome string) {
	vcsMetrics().requests.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}
