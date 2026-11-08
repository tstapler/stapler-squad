package services

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// Keystrokes are the product's latency-critical traffic, so the code that reads them off the
// socket must never wait on anything slower than the write to the pane. Two things used to put
// it in the way: per-keystroke instance bookkeeping (i.mu, snapshot rebuild) and the
// scrollback/resync requests, which ran capture-pane inline in the same read loop.

// inputBookkeepDelay coalesces keystroke bookkeeping: LastUserResponse and the terminal
// timestamps feed the review queue, which works in seconds, so 250 ms of lag is invisible there.
const inputBookkeepDelay = 250 * time.Millisecond

// humanInputSink is the instance state a keystroke updates.
type humanInputSink interface {
	UpdateTerminalTimestamps(content string, forceUpdate bool)
	MarkUserResponded() time.Time
}

// inputBookkeeper applies keystroke bookkeeping off the typing path: note is O(1) and never
// touches the instance, and one flush per delay window applies the newest chunk.
type inputBookkeeper struct {
	sink    humanInputSink
	delay   time.Duration
	after   func(d time.Duration, f func())
	latency *streamLatency

	mu        sync.Mutex
	latest    string
	scheduled bool
}

func newInputBookkeeper(sink humanInputSink, latency *streamLatency) *inputBookkeeper {
	return &inputBookkeeper{
		sink: sink, delay: inputBookkeepDelay, latency: latency,
		after: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

// note records that data was just typed. It returns without waiting on the instance.
func (b *inputBookkeeper) note(data []byte) {
	b.mu.Lock()
	// Keep the newest chunk, but a blank one (Enter) must not displace typed text: blank content
	// does not count as terminal activity, so the flush would lose the window's signal.
	if chunk := string(data); strings.TrimSpace(chunk) != "" || strings.TrimSpace(b.latest) == "" {
		b.latest = chunk
	}
	schedule := !b.scheduled
	b.scheduled = true
	b.mu.Unlock()
	if schedule {
		b.after(b.delay, b.flush)
	}
}

func (b *inputBookkeeper) flush() {
	b.mu.Lock()
	chunk := b.latest
	b.scheduled = false
	b.mu.Unlock()

	start := time.Now()
	b.sink.UpdateTerminalTimestamps(chunk, true)
	b.sink.MarkUserResponded()
	termlat.Default.Record(termlat.StageBookkeepFlush, b.latency.labels(), time.Since(start))
}

// slowRequestLane runs a stream's scrollback and resync requests on one worker, in order, so
// the input read loop keeps reading keystrokes while a capture-pane is in flight. A request
// error ends the stream the way it did inline: it is reported on errChan.
type slowRequestLane struct {
	jobs    chan func() error
	errChan chan<- error
	onFail  func() // ends the read loop when the loop would otherwise keep running past a failed job
	stop    chan struct{}
	stopped atomic.Bool
	once    sync.Once
}

// slowLaneDepth bounds queued requests; when full, submit blocks the read loop, which is the
// old inline behavior and the only backpressure a flooding client gets.
const slowLaneDepth = 32

func newSlowRequestLane(errChan chan<- error, onFail func()) *slowRequestLane {
	l := &slowRequestLane{jobs: make(chan func() error, slowLaneDepth), errChan: errChan, onFail: onFail, stop: make(chan struct{})}
	go l.run()
	return l
}

func (l *slowRequestLane) run() {
	for {
		select {
		case <-l.stop:
			return
		case job := <-l.jobs:
			if l.stopped.Load() {
				return
			}
			if err := job(); err != nil {
				l.fail(err)
				return
			}
		}
	}
}

func (l *slowRequestLane) fail(err error) {
	select {
	case l.errChan <- err:
	default: // the stream is already ending with another error
	}
	l.close() // no worker is left: unblock any submit parked on a full queue
	if l.onFail != nil {
		l.onFail()
	}
}

// submit queues job; it is dropped once the lane is closed.
func (l *slowRequestLane) submit(job func() error) {
	select {
	case l.jobs <- job:
	case <-l.stop:
	}
}

// close stops the worker; queued jobs that have not started are discarded.
func (l *slowRequestLane) close() {
	l.once.Do(func() {
		l.stopped.Store(true)
		close(l.stop)
	})
}

// latestResize coalesces resize requests to the newest size and applies them on the slow lane:
// hub.RequestResize resizes the pane and captures it (seconds under load), and running it in the
// read loop would hold keystrokes behind every resize a refitting or flapping client sends.
type latestResize struct {
	mu      sync.Mutex
	req     resizeReq
	pending bool
}

// push records the newest size and, unless a run is already queued, queues apply on lane.
// apply always receives the newest size at the time it runs.
func (r *latestResize) push(lane *slowRequestLane, cols, rows int, apply func(cols, rows int)) {
	r.pushOrdered(lane, nil, cols, rows, func(cols, rows int, _ func()) { apply(cols, rows) })
}

// pushOrdered is push for a stream whose keystrokes must not overtake its resizes. It starts a
// hold on gate before returning (so the very next keystroke the read loop sees is held) and
// lifts it when apply calls done, or at the latest when apply returns.
func (r *latestResize) pushOrdered(lane *slowRequestLane, gate *inputOrderGate, cols, rows int, apply func(cols, rows int, done func())) {
	r.mu.Lock()
	r.req = resizeReq{cols, rows}
	queued := r.pending
	r.pending = true
	r.mu.Unlock()
	if queued {
		return // the queued run will apply this newer size, and already holds the gate
	}
	gate.begin()
	lane.submit(func() error {
		r.mu.Lock()
		req := r.req
		r.pending = false
		r.mu.Unlock()
		var once sync.Once
		done := func() { once.Do(gate.release) }
		defer done()
		apply(req.cols, req.rows, done)
		return nil
	})
}

// inputOrderGate keeps a keystroke typed after a resize from reaching the pane before that
// resize does, without making the read loop wait for it: keystrokes arriving while a resize is
// pending queue here in order and flush the moment the pane has been resized (not after the
// repaint capture, which is what used to hold typing for seconds). A nil gate delivers directly.
type inputOrderGate struct {
	deliver func([]byte)
	after   func(d time.Duration, f func())

	mu       sync.Mutex
	pending  int // resizes begun and not yet released
	forgiven int // holds the safety valve already dropped; their late releases are ignored
	epoch    int // bumped each time a hold spell starts; a safety timer only acts on its own spell
	draining bool
	queue    [][]byte
}

// inputOrderGateMaxHold bounds how long a lost resize can hold typing back.
const inputOrderGateMaxHold = 2 * time.Second

func newInputOrderGate(deliver func([]byte)) *inputOrderGate {
	return &inputOrderGate{
		deliver: deliver,
		after:   func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

// begin starts a hold for one pending resize. The safety timer is armed once per hold spell (the
// first begin after the gate was open), so a stream of resizes cannot keep extending it.
func (g *inputOrderGate) begin() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.pending++
	arm := g.pending == 1
	if arm {
		g.epoch++
	}
	epoch := g.epoch
	g.mu.Unlock()
	if arm {
		g.after(inputOrderGateMaxHold, func() { g.expire(epoch) })
	}
}

// release lifts one hold; the last one flushes the queue in arrival order.
func (g *inputOrderGate) release() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.forgiven > 0 {
		g.forgiven--
		return
	}
	if g.pending > 0 {
		g.pending--
	}
	if g.pending == 0 {
		g.startDrainLocked()
	}
}

// expire is the safety valve: a resize that never reported must not wedge typing.
func (g *inputOrderGate) expire(epoch int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.epoch != epoch || g.pending == 0 {
		return
	}
	g.forgiven += g.pending
	g.pending = 0
	g.startDrainLocked()
}

// input delivers data now, or queues it behind a pending resize. The read loop is its only
// caller, so queue order is arrival order; delivery happens under the lock so a concurrent
// flush cannot interleave with it.
func (g *inputOrderGate) input(data []byte) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending > 0 || g.draining {
		g.queue = append(g.queue, append([]byte(nil), data...))
		return
	}
	g.deliver(data)
}

// startDrainLocked flushes the queue on its own goroutine: release runs on the hub's resizing
// goroutine, which must not wait on pane writes. While the drain runs, input keeps queueing
// behind it, so arrival order holds.
func (g *inputOrderGate) startDrainLocked() {
	if g.draining || len(g.queue) == 0 {
		return
	}
	g.draining = true
	go g.drain()
}

func (g *inputOrderGate) drain() {
	for {
		g.mu.Lock()
		if len(g.queue) == 0 || g.pending > 0 {
			g.draining = false
			g.mu.Unlock()
			return
		}
		data := g.queue[0]
		g.queue = g.queue[1:]
		g.mu.Unlock()
		g.deliver(data)
	}
}
