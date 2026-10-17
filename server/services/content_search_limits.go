package services

import (
	"net"
	"sync"
	"time"
)

// Proposed split (ADR-030 decision 6, INFERRED and adjustable): 3 of a browser's 6
// HTTP/1.1 connections leaves at least 3 for unary RPCs.
const (
	maxSearchStreamsPerOrigin = 3
	maxSearchStreamsTotal     = 8
	maxQueueWait              = 20 * time.Second
	maxConcurrentWalks        = 2
)

// SearchBusyReason says which cap refused a stream; it travels in SearchBusyDetail.
type SearchBusyReason string

const (
	busyPerIP        SearchBusyReason = "per_ip"
	busyTotal        SearchBusyReason = "total"
	busyQueueTimeout SearchBusyReason = "queue_timeout"
)

// ipLimiter counts open streams per remote host and in total. It is the per-IP
// accounting seam the range limiter (Task 1.6.2c) will share; the key function is
// remoteHost, the one place to change if a trusted proxy is ever introduced.
type ipLimiter struct {
	mu       sync.Mutex
	perHost  map[string]int
	total    int
	hostCap  int
	totalCap int
}

// ipCaps are the stream caps of an ipLimiter.
type ipCaps struct{ PerHost, Total int }

func newIPLimiter(c ipCaps) *ipLimiter {
	return &ipLimiter{perHost: map[string]int{}, hostCap: c.PerHost, totalCap: c.Total}
}

// remoteHost keys on the connection's peer address only: forwarded headers are
// ignored because the unauthenticated local listener cannot trust them.
func remoteHost(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// Acquire takes a slot for remoteAddr or says which cap refused it. The returned
// release is idempotent.
func (l *ipLimiter) Acquire(remoteAddr string) (release func(), refused SearchBusyReason) {
	host := remoteHost(remoteAddr)
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.total >= l.totalCap:
		return nil, busyTotal
	case l.perHost[host] >= l.hostCap:
		return nil, busyPerIP
	}
	l.total++
	l.perHost[host]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			if l.perHost[host]--; l.perHost[host] == 0 {
				delete(l.perHost, host)
			}
		})
	}, ""
}

// walkGate bounds concurrent walks process-wide with a FIFO queue. It replaces a plain
// semaphore because waiters must learn their position (Queued{ahead}) as it changes.
type walkGate struct {
	mu      sync.Mutex
	limit   int
	running int
	waiters []*gateWaiter
}

type gateWaiter struct {
	granted chan struct{}
	moved   chan struct{} // buffered 1: the position changed, re-read Ahead
}

func newWalkGate(limit int) *walkGate { return &walkGate{limit: limit} }

// Enter returns (nil, 0) when a slot was free, else a waiter and how many searches are ahead.
func (g *walkGate) Enter() (*gateWaiter, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running < g.limit && len(g.waiters) == 0 {
		g.running++
		return nil, 0
	}
	w := &gateWaiter{granted: make(chan struct{}), moved: make(chan struct{}, 1)}
	g.waiters = append(g.waiters, w)
	return w, g.running + len(g.waiters) - 1
}

// Ahead is the number of searches in front of w (running plus queued before it).
func (g *walkGate) Ahead(w *gateWaiter) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, x := range g.waiters {
		if x == w {
			return g.running + i
		}
	}
	return 0
}

// Release gives a slot back and grants it to the head of the queue.
func (g *walkGate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running--
	if len(g.waiters) > 0 {
		head := g.waiters[0]
		g.waiters = g.waiters[1:]
		g.running++
		close(head.granted)
	}
	g.notifyLocked()
}

// Abandon removes a waiter that stopped waiting. If the slot was granted in the
// meantime it is released again, so a cancelled search never leaks a walk slot.
func (g *walkGate) Abandon(w *gateWaiter) {
	g.mu.Lock()
	for i, x := range g.waiters {
		if x == w {
			g.waiters = append(g.waiters[:i], g.waiters[i+1:]...)
			g.notifyLocked()
			g.mu.Unlock()
			return
		}
	}
	g.mu.Unlock()
	select {
	case <-w.granted:
		g.Release()
	default:
	}
}

func (g *walkGate) notifyLocked() {
	for _, w := range g.waiters {
		select {
		case w.moved <- struct{}{}:
		default:
		}
	}
}
