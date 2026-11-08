package services

import (
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
	b.latest = string(data)
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
