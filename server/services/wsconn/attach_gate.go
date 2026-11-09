package wsconn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/qos"
	"github.com/tstapler/stapler-squad/internal/termlat"
)

// A page reload reconnects every open terminal at once (about ten), and each attach does daemon
// RPCs. Run together they queue behind each other and all hit their deadline, so the client
// reconnects into the same pile-up. The gate lets a few through at a time, most recently used
// session first, so the terminal the user is looking at is ready while the rest queue.
const defaultAttachConcurrency = 3

// attachLimiter is the process-wide gate for terminal attaches.
var attachLimiter = newAttachGate(defaultAttachConcurrency, qos.Default.LastInteraction) //nolint:gochecknoglobals // one limit across every stream

// attachQueueTimeout caps how long one attach may wait for a slot before failing, so a wedged
// attach ahead of it cannot hold the queue (and the client's patience) forever.
const attachQueueTimeout = 30 * time.Second

// gatedAttach runs open under g's concurrency limit and records how long it queued and how long
// it took, so /actuator/terminal-latency shows where a slow attach spent its time.
func gatedAttach[T any](ctx context.Context, g *attachGate, labels termlat.Labels, key string, open func() (T, error)) (T, error) {
	var zero T
	waitCtx, cancel := context.WithTimeout(ctx, attachQueueTimeout)
	defer cancel()
	release, waited, err := g.acquire(waitCtx, key)
	termlat.Default.Record(termlat.StageAttachQueue, labels, waited)
	if err != nil {
		return zero, fmt.Errorf("terminal attach queue: %w", err)
	}
	defer release()
	start := time.Now()
	out, err := open()
	termlat.Default.Record(termlat.StageAttachOpen, labels, time.Since(start))
	return out, err
}

// attachGate is a counting semaphore whose waiters are served most-recently-used first (ties,
// and sessions never touched, in arrival order).
type attachGate struct {
	limit   int
	lastUse func(key string) time.Time

	mu      sync.Mutex
	active  int
	waiters []*attachWaiter
	seq     uint64
}

type attachWaiter struct {
	key   string
	order uint64
	ready chan struct{}
	// granted is set under the gate's mutex when a slot was handed to this waiter, so a waiter
	// that gave up concurrently can tell whether it must return the slot.
	granted bool
}

func newAttachGate(limit int, lastUse func(string) time.Time) *attachGate {
	return &attachGate{limit: max(limit, 1), lastUse: lastUse}
}

// acquire blocks until a slot is free or ctx ends. waited is how long it queued (zero when a
// slot was free). The returned release must be called exactly once.
func (g *attachGate) acquire(ctx context.Context, key string) (release func(), waited time.Duration, err error) {
	g.mu.Lock()
	if g.active < g.limit {
		g.active++
		g.mu.Unlock()
		return g.release, 0, nil
	}
	g.seq++
	w := &attachWaiter{key: key, order: g.seq, ready: make(chan struct{})}
	g.waiters = append(g.waiters, w)
	g.mu.Unlock()

	start := time.Now()
	select {
	case <-w.ready:
		return g.release, time.Since(start), nil
	case <-ctx.Done():
		g.mu.Lock()
		defer g.mu.Unlock()
		if w.granted {
			g.releaseLocked() // the slot arrived as we gave up: pass it on
		} else {
			g.removeLocked(w)
		}
		return nil, time.Since(start), ctx.Err()
	}
}

func (g *attachGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseLocked()
}

// releaseLocked frees a slot, or hands it straight to the best waiter.
func (g *attachGate) releaseLocked() {
	next := g.pickLocked()
	if next == nil {
		g.active--
		return
	}
	next.granted = true
	close(next.ready)
}

// pickLocked removes and returns the waiter to serve next: the one whose session was used most
// recently, earliest arrival among equals.
func (g *attachGate) pickLocked() *attachWaiter {
	best := -1
	var bestUse time.Time
	for i, w := range g.waiters {
		use := g.lastUse(w.key)
		if best < 0 || use.After(bestUse) || (use.Equal(bestUse) && w.order < g.waiters[best].order) {
			best, bestUse = i, use
		}
	}
	if best < 0 {
		return nil
	}
	w := g.waiters[best]
	g.waiters = append(g.waiters[:best], g.waiters[best+1:]...)
	return w
}

func (g *attachGate) removeLocked(w *attachWaiter) {
	for i, cand := range g.waiters {
		if cand == w {
			g.waiters = append(g.waiters[:i], g.waiters[i+1:]...)
			return
		}
	}
}

// queued is the number of attaches waiting for a slot.
func (g *attachGate) queued() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.waiters)
}
