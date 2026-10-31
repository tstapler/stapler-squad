// Package scanqueue is the coalescing, CPU-polite work queue behind the
// background transcript scanners (token parsing, artifact extraction).
//
// Two properties replace the old drop-on-full channels: a path queued twice is
// one unit of work (and a path changed while running is re-run, newest wins),
// so nothing is ever dropped and later redone; and startup backfill runs
// delayed and throttled while live file changes stay immediate.
package scanqueue

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/load"
)

// Config tunes a Queue. Zero values select the documented defaults.
type Config struct {
	// Workers bounds concurrent handlers. Default DefaultWorkers().
	Workers int
	// StartDelay holds back backfill (not live) work after Start. Default 0.
	StartDelay time.Duration
	// BackfillDuty in (0,1] is the CPU share a worker may use on backfill
	// items: after an item taking d, it idles d*(1/duty-1). Default 1 (no throttle).
	BackfillDuty float64
	// Busy reports host overload; while true the duty is quartered. Optional.
	Busy func() bool

	// Now and Sleep are injectable for deterministic tests.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration)
}

// DefaultWorkers is a quarter of GOMAXPROCS, between 1 and 2.
func DefaultWorkers() int {
	return min(2, max(1, runtime.GOMAXPROCS(0)/4))
}

type class uint8

const (
	idle class = iota
	backfill
	live
)

// Stats counts queue activity since creation.
type Stats struct {
	Enqueued  uint64 // Enqueue calls
	Coalesced uint64 // calls absorbed by an already-pending or running path
	Processed uint64 // handler invocations
}

// Queue is a coalescing path queue drained by a bounded worker pool.
type Queue struct {
	cfg    Config
	handle func(path string)

	mu      sync.Mutex
	pending map[string]class // path -> class it is queued as
	rerun   map[string]class // path -> class to re-queue after its running pass
	running map[string]struct{}
	lives   []string
	backs   []string

	wake  chan struct{}
	ready chan struct{} // closed once StartDelay has elapsed

	enqueued, coalesced, processed atomic.Uint64
}

// New builds a Queue that calls handle(path) for each coalesced path.
func New(cfg Config, handle func(path string)) *Queue {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers()
	}
	if cfg.BackfillDuty <= 0 || cfg.BackfillDuty > 1 {
		cfg.BackfillDuty = 1
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = realSleep
	}
	return &Queue{
		cfg: cfg, handle: handle,
		pending: map[string]class{}, rerun: map[string]class{}, running: map[string]struct{}{},
		wake: make(chan struct{}, 1), ready: make(chan struct{}),
	}
}

func realSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Start launches the workers; they stop when ctx is done.
func (q *Queue) Start(ctx context.Context) {
	go func() {
		if q.cfg.StartDelay > 0 {
			q.cfg.Sleep(ctx, q.cfg.StartDelay)
		}
		close(q.ready)
		q.signal()
	}()
	for i := 0; i < q.cfg.Workers; i++ {
		go q.worker(ctx)
	}
}

// Enqueue schedules a live change: processed ahead of backfill, with no start
// delay or throttling.
func (q *Queue) Enqueue(path string) { q.add(path, live) }

// EnqueueBackfill schedules startup catch-up work, subject to StartDelay and
// BackfillDuty. A path already queued as live stays live.
func (q *Queue) EnqueueBackfill(path string) { q.add(path, backfill) }

// Stats returns the activity counters.
func (q *Queue) Stats() Stats {
	return Stats{q.enqueued.Load(), q.coalesced.Load(), q.processed.Load()}
}

// Pending returns the number of paths waiting (not counting those running).
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *Queue) add(path string, c class) {
	q.enqueued.Add(1)
	q.mu.Lock()
	switch {
	case q.isRunning(path):
		q.coalesced.Add(1)
		q.rerun[path] = max(q.rerun[path], c)
	case q.pending[path] != idle:
		q.coalesced.Add(1)
		if c > q.pending[path] { // promote backfill -> live
			q.pending[path] = c
			q.lives = append(q.lives, path)
		}
	default:
		q.pending[path] = c
		q.push(path, c)
	}
	q.mu.Unlock()
	q.signal()
}

func (q *Queue) isRunning(path string) bool { _, ok := q.running[path]; return ok }

func (q *Queue) push(path string, c class) {
	if c == live {
		q.lives = append(q.lives, path)
	} else {
		q.backs = append(q.backs, path)
	}
}

func (q *Queue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// pop returns the next runnable path, live first; backfill only once ready.
func (q *Queue) pop() (path string, c class, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.lives) > 0 {
		p := q.lives[0]
		q.lives = q.lives[1:]
		if q.pending[p] == live { // skip entries promoted-and-already-taken
			return q.take(p), live, true
		}
	}
	select {
	case <-q.ready:
	default:
		return "", idle, false
	}
	for len(q.backs) > 0 {
		p := q.backs[0]
		q.backs = q.backs[1:]
		if q.pending[p] == backfill {
			return q.take(p), backfill, true
		}
	}
	return "", idle, false
}

func (q *Queue) take(p string) string {
	delete(q.pending, p)
	q.running[p] = struct{}{}
	return p
}

func (q *Queue) finish(p string) {
	q.mu.Lock()
	delete(q.running, p)
	if c, again := q.rerun[p]; again {
		delete(q.rerun, p)
		q.pending[p] = c
		q.push(p, c)
	}
	more := len(q.lives)+len(q.backs) > 0
	q.mu.Unlock()
	if more {
		q.signal()
	}
}

func (q *Queue) worker(ctx context.Context) {
	for {
		path, c, ok := q.pop()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
				continue
			}
		}
		// Another worker may find more to do; chain the wake-up.
		q.signal()
		start := q.cfg.Now()
		q.handle(path)
		q.processed.Add(1)
		q.finish(path)
		if c == backfill {
			q.throttle(ctx, q.cfg.Now().Sub(start))
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (q *Queue) throttle(ctx context.Context, worked time.Duration) {
	duty := q.cfg.BackfillDuty
	if q.cfg.Busy != nil && q.cfg.Busy() {
		duty /= 4
	}
	if duty >= 1 || worked <= 0 {
		runtime.Gosched()
		return
	}
	q.cfg.Sleep(ctx, time.Duration(float64(worked)*(1/duty-1)))
}

// HostBusy reports whether the 1-minute load average exceeds twice the CPU
// count, cached for 10s so it is cheap to call per item.
func HostBusy() func() bool {
	var (
		mu      sync.Mutex
		checked time.Time
		busy    bool
	)
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(checked) > 10*time.Second {
			checked = time.Now()
			if avg, err := load.Avg(); err == nil {
				busy = avg.Load1 > 2*float64(runtime.NumCPU())
			}
		}
		return busy
	}
}

// BackfillConfig is the production profile for startup transcript backfill:
// a bounded pool, a delay so the burst does not compete with startup traffic,
// and a 30% per-worker duty cycle (quartered when the host is overloaded).
func BackfillConfig(startDelay time.Duration) Config {
	return Config{StartDelay: startDelay, BackfillDuty: 0.3, Busy: HostBusy()}
}
