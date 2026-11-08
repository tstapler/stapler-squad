package services

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type orderLog struct {
	mu sync.Mutex
	v  []string
}

func (l *orderLog) add(s string) {
	l.mu.Lock()
	l.v = append(l.v, s)
	l.mu.Unlock()
}

func (l *orderLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.v...)
}

// fakeAfter records timers instead of arming them so a test fires them by hand.
type fakeAfter struct {
	mu     sync.Mutex
	timers []func()
}

func (f *fakeAfter) after(_ time.Duration, fn func()) {
	f.mu.Lock()
	f.timers = append(f.timers, fn)
	f.mu.Unlock()
}

func (f *fakeAfter) fireAll() {
	f.mu.Lock()
	fns := f.timers
	f.timers = nil
	f.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func newTestGate(log *orderLog, fa *fakeAfter) *inputOrderGate {
	g := newInputOrderGate(func(b []byte) { log.add(string(b)) })
	g.after = fa.after
	return g
}

func TestInputOrderGate_should_DeliverImmediately_When_NoResizeIsPending(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.input([]byte("a"))
	assert.Equal(t, []string{"a"}, log.get())
}

func TestInputOrderGate_should_HoldKeystrokesBehindAResize_AndFlushThemInOrder(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	g.input([]byte("a"))
	g.input([]byte("b"))
	assert.Empty(t, log.get(), "a keystroke typed after a resize waits for the resize")
	g.release()
	assert.Equal(t, []string{"a", "b"}, log.get())
	g.input([]byte("c"))
	assert.Equal(t, []string{"a", "b", "c"}, log.get(), "and later ones are not reordered ahead of the flush")
}

func TestInputOrderGate_should_StayHeld_Until_EveryPendingResizeIsApplied(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	g.begin()
	g.input([]byte("a"))
	g.release()
	assert.Empty(t, log.get())
	g.release()
	assert.Equal(t, []string{"a"}, log.get())
}

func TestInputOrderGate_should_ReleaseOnItsOwn_When_TheResizeNeverReports(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	g.input([]byte("a"))
	fa.fireAll()
	assert.Equal(t, []string{"a"}, log.get(), "a lost resize must never wedge typing")
	g.release() // the late real release is harmless
	g.input([]byte("b"))
	assert.Equal(t, []string{"a", "b"}, log.get())
}

func TestLatestResize_should_DeliverTheNextKeystrokeAfterTheResize_When_Gated(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	lane := newSlowRequestLane(make(chan error, 1), nil)
	defer lane.close()
	var r latestResize
	applied := make(chan struct{})
	r.pushOrdered(lane, g, 100, 30, func(cols, rows int, done func()) {
		log.add("resize")
		done()
		close(applied)
	})
	g.input([]byte("k")) // read loop: the keystroke follows the resize message
	<-applied
	assert.Eventually(t, func() bool { return len(log.get()) == 2 }, 5*time.Second, time.Millisecond)
	assert.Equal(t, []string{"resize", "k"}, log.get())
}

func TestLatestResize_should_ReleaseTheGate_When_ApplyNeverCallsDone(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	lane := newSlowRequestLane(make(chan error, 1), nil)
	defer lane.close()
	var r latestResize
	ran := make(chan struct{})
	r.pushOrdered(lane, g, 100, 30, func(cols, rows int, _ func()) { close(ran) })
	<-ran
	g.input([]byte("k"))
	assert.Eventually(t, func() bool { return len(log.get()) == 1 }, 5*time.Second, time.Millisecond,
		"returning from apply releases the gate even when it skipped the early signal")
}
