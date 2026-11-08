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
	dirty     bool // a chunk is waiting in latest
	scheduled bool // a flush is queued or running
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
	b.dirty = true
	schedule := !b.scheduled
	b.scheduled = true
	b.mu.Unlock()
	if schedule {
		b.after(b.delay, b.flush)
	}
}

// flush stays "scheduled" until the sink returns, so keystrokes typed while the sink waits on the
// instance lock only update latest instead of each starting a timer; a chunk that arrives in that
// time is flushed in a follow-up window.
func (b *inputBookkeeper) flush() {
	b.mu.Lock()
	chunk := b.latest
	b.latest, b.dirty = "", false
	b.mu.Unlock()

	start := time.Now()
	b.sink.UpdateTerminalTimestamps(chunk, true)
	b.sink.MarkUserResponded()
	termlat.Default.Record(termlat.StageBookkeepFlush, b.latency.labels(), time.Since(start))

	b.mu.Lock()
	again := b.dirty
	b.scheduled = again
	b.mu.Unlock()
	if again {
		b.after(b.delay, b.flush)
	}
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
	// droppedScrollback counts scrollback requests refused because the lane was busy.
	droppedScrollback atomic.Int64
}

// slowLaneDepth bounds queued requests; when full, submit blocks the read loop, which is the
// old inline behavior and the only backpressure a flooding client gets.
const slowLaneDepth = 32

// droppableLaneDepth is how much of the lane scrollback may occupy. Scrollback is the one
// request a client can flood (a fast scroll), so past this it is dropped, never queued, which
// keeps room for resync and resize and keeps submit from blocking the keystroke read loop.
const droppableLaneDepth = slowLaneDepth / 2

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

// submitDroppable queues job unless the lane is already busy enough that the job would crowd out
// resync and resize work; it never blocks. It reports whether the job was queued.
func (l *slowRequestLane) submitDroppable(job func() error) bool {
	if len(l.jobs) >= droppableLaneDepth {
		l.droppedScrollback.Add(1)
		return false
	}
	select {
	case l.jobs <- job:
		return true
	case <-l.stop:
		return false
	default:
		l.droppedScrollback.Add(1)
		return false
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
	r.mu.Lock()
	r.req = resizeReq{cols, rows}
	queued := r.pending
	r.pending = true
	r.mu.Unlock()
	if queued {
		return
	}
	lane.submit(func() error {
		r.mu.Lock()
		req := r.req
		r.pending = false
		r.mu.Unlock()
		apply(req.cols, req.rows)
		return nil
	})
}
