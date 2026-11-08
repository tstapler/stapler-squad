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

func eventuallyLogged(t *testing.T, log *orderLog, want ...string) {
	t.Helper()
	assert.Eventually(t, func() bool { return len(log.get()) == len(want) }, 5*time.Second, time.Millisecond)
	assert.Equal(t, want, log.get())
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
	g.input([]byte("c")) // may race the drain goroutine: it must still land last
	eventuallyLogged(t, log, "a", "b", "c")
}

func TestInputOrderGate_should_NotDeliverOnTheReleasingGoroutine(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	delivering := make(chan struct{})
	unblock := make(chan struct{})
	g := newInputOrderGate(func(b []byte) {
		log.add(string(b))
		close(delivering)
		<-unblock
	})
	g.after = fa.after
	g.begin()
	g.input([]byte("a"))

	released := make(chan struct{})
	go func() {
		g.release() // stands in for the hub's resize goroutine
		close(released)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("release blocked on a slow pane write")
	}
	<-delivering
	close(unblock)
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
	eventuallyLogged(t, log, "a")
}

func TestInputOrderGate_should_BoundTheHold_EvenWhenResizesKeepArriving(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	g.input([]byte("a"))
	g.begin() // a drag keeps queueing resizes behind a stuck one
	g.begin()
	assert.Len(t, fa.timers, 1, "one safety timer per hold spell, not one per resize")
	fa.fireAll()
	eventuallyLogged(t, log, "a")
}

func TestInputOrderGate_should_IgnoreALateRelease_FromAHoldTheSafetyValveDropped(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	fa.fireAll() // the first resize is given up on
	g.begin()    // a newer resize starts a new hold
	g.input([]byte("b"))
	g.release() // the stale resize finally reports: it must not lift the newer hold
	time.Sleep(20 * time.Millisecond)
	assert.Empty(t, log.get())
	g.release() // the real one
	eventuallyLogged(t, log, "b")
}

func TestInputOrderGate_should_ReleaseOnItsOwn_When_TheResizeNeverReports(t *testing.T) {
	t.Parallel()
	log, fa := &orderLog{}, &fakeAfter{}
	g := newTestGate(log, fa)
	g.begin()
	g.input([]byte("a"))
	fa.fireAll()
	eventuallyLogged(t, log, "a")
	g.release() // the late real release is harmless
	g.input([]byte("b"))
	eventuallyLogged(t, log, "a", "b")
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
	eventuallyLogged(t, log, "resize", "k")
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
