package streamhub_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// redrawController is a fakeSessionController whose backend also provides a full-state redraw.
type redrawController struct {
	*fakeSessionController
	snap  streamhub.FullStateSnapshot
	ok    bool
	err   error
	calls atomic.Int32
	// failNext makes every later redraw capture fail, after earlier ones succeeded.
	failNext atomic.Bool
}

func (r *redrawController) CapturePaneRedraw(context.Context) (streamhub.FullStateSnapshot, bool, error) {
	r.calls.Add(1)
	if r.failNext.Load() {
		return streamhub.FullStateSnapshot{}, false, errors.New("daemon gone")
	}
	return r.snap, r.ok, r.err
}

func attachAndFrames(t *testing.T, c streamhub.SessionController) [][]byte {
	t.Helper()
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	tr := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{CanResize: true})
	frames := tr.ReceivedFrames()
	hub.DetachSubscriber(id)
	return frames
}

func TestAttachSubscriber_should_SendRedrawVerbatimAndFresh_When_BackendProvidesOne(t *testing.T) {
	defer goleak.VerifyNone(t)
	const redraw = "\x1b[?1049h\x1b]0;t\x07\x1b[2;3Hx"
	c := &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: redraw, AltScreen: true}, ok: true}
	c.captureContent = "legacy text"

	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	first, second := streamhub.NewMemoryTransport(), streamhub.NewMemoryTransport()
	id1 := hub.AttachSubscriber(first, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id1)
	id2 := hub.AttachSubscriber(second, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id2)

	for i, tr := range []*streamhub.MemoryTransport{first, second} {
		frames := tr.ReceivedFrames()
		if len(frames) != 1 || string(frames[0]) != redraw {
			t.Fatalf("subscriber %d: want exactly the redraw verbatim, got %q", i, frames)
		}
	}
	if got := c.captureCalls.Load(); got != 0 {
		t.Fatalf("text capture ran %d times; the redraw must replace it", got)
	}
	if got := c.calls.Load(); got != 2 {
		t.Fatalf("redraw captured %d times for 2 attaches; a redraw must never be served from the cache", got)
	}
}

func TestAttachSubscriber_should_FallBackToTextSnapshot_When_RedrawUnavailable(t *testing.T) {
	cases := map[string]*redrawController{
		"older daemon declines": {ok: false},
		"capture errors":        {ok: true, err: errors.New("daemon gone")},
	}
	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			defer goleak.VerifyNone(t)
			c.fakeSessionController = newFakeSessionController()
			c.captureContent = "legacy text"
			frames := attachAndFrames(t, c)
			if len(frames) != 1 || !strings.Contains(string(frames[0]), "legacy text") {
				t.Fatalf("want the legacy text snapshot, got %q", frames)
			}
		})
	}
}

func TestAttachSubscriber_should_NotServeStaleRedrawAsText_When_LaterCaptureFails(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := &redrawController{fakeSessionController: newFakeSessionController(),
		snap: streamhub.FullStateSnapshot{Data: "REDRAW"}, ok: true}
	c.captureContent = "legacy text"
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	first, second := streamhub.NewMemoryTransport(), streamhub.NewMemoryTransport()
	id1 := hub.AttachSubscriber(first, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id1)

	c.failNext.Store(true)
	id2 := hub.AttachSubscriber(second, streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(id2)

	frames := second.ReceivedFrames()
	if len(frames) != 1 || strings.Contains(string(frames[0]), "REDRAW") || !strings.Contains(string(frames[0]), "legacy text") {
		t.Fatalf("a failed redraw must fall back to text, never to the earlier redraw; got %q", frames)
	}
}

// orderedTransport appends every frame it sends to a log shared with the test body, so the test
// can assert the interleaving of direct paints and queued frames.
type orderedTransport struct {
	log *orderLog
}

func (o *orderedTransport) Send(data []byte) error { o.log.add("frame:" + string(data)); return nil }
func (o *orderedTransport) Close() error           { return nil }

type orderLog struct {
	mu     sync.Mutex
	events []string
	added  chan struct{} // one token per event
}

func newOrderLog() *orderLog { return &orderLog{added: make(chan struct{}, 64)} }

func (l *orderLog) add(e string) {
	l.mu.Lock()
	l.events = append(l.events, e)
	l.mu.Unlock()
	l.added <- struct{}{}
}
func (l *orderLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func (l *orderLog) waitFor(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-l.added:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %d events, have %v", n, l.snapshot())
		}
	}
}

func TestFirstPaint_should_DiscardQueuedFramesAndOrderPaintBeforeLiveOutput(t *testing.T) {
	defer goleak.VerifyNone(t)
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	log := newOrderLog()
	id := hub.AttachSubscriber(&orderedTransport{log}, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	hub.Broadcast([]byte("stale-before-capture"))
	select {
	case <-log.added:
		t.Fatalf("a ManualFirstPaint subscriber must send nothing before FirstPaint, got %v", log.snapshot())
	case <-time.After(200 * time.Millisecond): // a running writer would have sent it by now
	}

	err := hub.FirstPaint(id, func() error {
		log.add("paint")
		hub.Broadcast([]byte("live-during-paint"))
		return nil
	})
	require.NoError(t, err)
	hub.Broadcast([]byte("live-after"))

	log.waitFor(t, 3)
	require.Equal(t, []string{"paint", "frame:live-during-paint", "frame:live-after"}, log.snapshot())
}

func TestAttachSubscriber_should_MakeNoCapture_When_ManualFirstPaint(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := &redrawController{fakeSessionController: newFakeSessionController(), snap: streamhub.FullStateSnapshot{Data: "R"}, ok: true}
	c.captureContent = "legacy"
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour))
	tr := streamhub.NewMemoryTransport()

	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	if c.calls.Load() != 0 || c.captureCalls.Load() != 0 {
		t.Fatalf("attach captured (redraw %d, text %d) for a snapshot FirstPaint would replace", c.calls.Load(), c.captureCalls.Load())
	}
	if frames := tr.ReceivedFrames(); len(frames) != 0 {
		t.Fatalf("no catch-up expected, got %q", frames)
	}
}

func TestFirstPaint_should_ReturnErrUnknownSubscriber_When_AlreadyDetached(t *testing.T) {
	defer goleak.VerifyNone(t)
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{ManualFirstPaint: true})
	hub.DetachSubscriber(id)

	err := hub.FirstPaint(id, func() error { t.Fatal("paint must not run"); return nil })

	require.ErrorIs(t, err, streamhub.ErrUnknownSubscriber)
}

// A resize must neither capture nor broadcast between FirstPaint's capture and its write: the
// broadcast would be older than the paint yet could be written after it.
func TestFirstPaint_should_BlockResizeUntilPaintReturns(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := newFakeSessionController()
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(5*time.Millisecond), streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	painter := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(painter)
	voter := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true})
	defer hub.DetachSubscriber(voter)

	inPaint, release := make(chan struct{}), make(chan struct{})
	painted := make(chan error, 1)
	go func() {
		painted <- hub.FirstPaint(painter, func() error { close(inPaint); <-release; return nil })
	}()
	<-inPaint

	resized := make(chan struct{})
	go func() { hub.RequestResize(context.Background(), voter, mustSize(t, 100, 30)); close(resized) }()

	select {
	case <-resized:
		t.Fatal("resize completed while a first paint was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	if got := c.setWindowSizeCalls.Load(); got != 0 {
		t.Fatalf("SetWindowSize ran %d times during the first paint", got)
	}

	close(release)
	require.NoError(t, <-painted)
	<-resized
	if got := c.setWindowSizeCalls.Load(); got == 0 {
		t.Fatal("resize never ran after the paint returned")
	}
}

func TestApplyNegotiatedSize_should_SkipCapture_When_EverySubscriberAwaitsFirstPaint(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := &redrawController{fakeSessionController: newFakeSessionController(), snap: streamhub.FullStateSnapshot{Data: "R"}, ok: true}
	hub := streamhub.NewStreamHub("s", c, streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(5*time.Millisecond), streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	id := hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{CanResize: true, ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	hub.RequestResize(context.Background(), id, mustSize(t, 100, 30))

	if c.setWindowSizeCalls.Load() == 0 {
		t.Fatal("the resize itself must still be applied")
	}
	if got := c.calls.Load(); got != 0 {
		t.Fatalf("resize captured %d redraws that the pending first paint would discard", got)
	}

	require.NoError(t, hub.FirstPaint(id, func() error { return nil }))
	hub.RequestResize(context.Background(), id, mustSize(t, 90, 20))
	if c.calls.Load() == 0 {
		t.Fatal("after the first paint a resize must broadcast a fresh redraw")
	}
}

// A subscriber waiting for FirstPaint has no writer, so a burst plus a delayed FirstPaint (blocked
// on resizeApplyMu or a slow capture) must not trip the slow-subscriber eviction.
func TestFirstPaint_should_NotEvictAwaitingSubscriber_When_BurstOutlastsSlowGrace(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithClock(clock),
		streamhub.WithTeardownGrace(time.Hour), streamhub.WithSlowSubscriberGrace(20*time.Millisecond))
	log := newOrderLog()
	id := hub.AttachSubscriber(&orderedTransport{log}, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	for i := 0; i < 300; i++ { // more than the 100-deep queue
		hub.Broadcast([]byte("burst"))
	}
	clock.Advance(200 * time.Millisecond) // fires the slow-grace timer, which must spare an awaiting subscriber

	ran := false
	err := hub.FirstPaint(id, func() error { ran = true; log.add("paint"); return nil })
	require.NoError(t, err)
	require.True(t, ran)
	require.Zero(t, hub.SlowSubscriberDropsTotal())
}

func TestFirstPaint_should_EvictSubscriber_When_NeverPaintedWithinMaxAwait(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithClock(clock),
		streamhub.WithTeardownGrace(time.Hour), streamhub.WithFirstPaintMaxAwait(30*time.Millisecond))
	tr := streamhub.NewMemoryTransport()
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{ManualFirstPaint: true})
	clock.Advance(30 * time.Millisecond)
	require.Eventually(t, tr.IsClosed, 5*time.Second, time.Millisecond, "max-await eviction must close the transport")
	require.ErrorIs(t, hub.FirstPaint(id, func() error { return nil }), streamhub.ErrUnknownSubscriber)
}

// Pins the residual window: a frame broadcast during the capture is delivered after the paint
// even though the capture may already contain it (at-least-once; see ADR-032).
func TestFirstPaint_should_DeliverFrameBroadcastDuringPaintAfterPaint(t *testing.T) {
	defer goleak.VerifyNone(t)
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	log := newOrderLog()
	id := hub.AttachSubscriber(&orderedTransport{log}, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)
	require.NoError(t, hub.FirstPaint(id, func() error {
		hub.Broadcast([]byte("F"))
		log.add("paint")
		return nil
	}))
	log.waitFor(t, 2)
	require.Equal(t, []string{"paint", "frame:F"}, log.snapshot())
}

// seqTransport records every Send, in order, without back-pressure.
type seqTransport struct {
	mu     sync.Mutex
	events []string
}

func (s *seqTransport) Send(d []byte) error {
	s.mu.Lock()
	s.events = append(s.events, string(d))
	s.mu.Unlock()
	return nil
}
func (s *seqTransport) Close() error { return nil }
func (s *seqTransport) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}
func (s *seqTransport) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}

// More than the 100-deep queue arrives during a paint that outlasts the slow-subscriber grace:
// nothing may be evicted or lost, and every frame follows the paint in order.
func TestFirstPaint_should_HoldEveryFrameInOrder_When_LongPaintOutlastsSlowGrace(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithClock(clock),
		streamhub.WithTeardownGrace(time.Hour), streamhub.WithSlowSubscriberGrace(20*time.Millisecond))
	tr := &seqTransport{}
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	const n = 300
	require.NoError(t, hub.FirstPaint(id, func() error {
		for i := 0; i < n; i++ {
			hub.Broadcast([]byte(fmt.Sprintf("f%03d", i)))
		}
		clock.Advance(150 * time.Millisecond) // the paint outlasts the slow-subscriber grace
		_ = tr.Send([]byte("paint"))
		return nil
	}))
	require.Eventually(t, func() bool { return len(tr.snapshot()) >= n+1 }, 5*time.Second, 5*time.Millisecond)
	ev := tr.snapshot()
	require.Equal(t, "paint", ev[0])
	for i := 0; i < n; i++ {
		require.Equal(t, fmt.Sprintf("f%03d", i), ev[i+1])
	}
	require.Zero(t, hub.SlowSubscriberDropsTotal())
}

func TestFirstPaint_should_NotEvict_When_MaxAwaitExpiresMidPaint(t *testing.T) {
	defer goleak.VerifyNone(t)
	clock := streamhub.NewFakeClock(time.Unix(0, 0))
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithClock(clock),
		streamhub.WithTeardownGrace(time.Hour), streamhub.WithFirstPaintMaxAwait(20*time.Millisecond))
	tr := &seqTransport{}
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	require.NoError(t, hub.FirstPaint(id, func() error {
		clock.Advance(150 * time.Millisecond) // the paint outlasts the max-await window
		return nil
	}))
	hub.Broadcast([]byte("live"))
	require.Eventually(t, func() bool { return len(tr.snapshot()) == 1 }, 2*time.Second, 5*time.Millisecond)
}

func TestFirstPaint_should_Fail_When_PaintBacklogOverflows(t *testing.T) {
	defer goleak.VerifyNone(t)
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	id := hub.AttachSubscriber(&seqTransport{}, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	err := hub.FirstPaint(id, func() error {
		for i := 0; i < 5000; i++ { // past the hold limit
			hub.Broadcast([]byte("x"))
		}
		return nil
	})
	require.Error(t, err, "a gap in the held frames must fail the paint, not pass silently")
}

// Producer-vs-capture stress: frames numbered above the capture's sequence must all follow the
// paint contiguously (duplicates of lower ones are allowed, gaps are not).
func TestFirstPaint_should_LeaveNoGap_When_FramesRaceTheCapture(t *testing.T) {
	iterations := 300
	if testing.Short() {
		iterations = 30
	}
	for it := 0; it < iterations; it++ {
		runFirstPaintRace(t, it)
	}
}

func runFirstPaintRace(t *testing.T, it int) {
	t.Helper()
	hub := streamhub.NewStreamHub("s", newFakeSessionController(), streamhub.WithTeardownGrace(time.Hour))
	tr := &seqTransport{}
	id := hub.AttachSubscriber(tr, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	var seq atomic.Int64
	var painted atomic.Bool
	var paintSeq atomic.Int64 // frames up to this are in the capture, never delivered after the paint
	stop := make(chan struct{})
	prodDone := make(chan struct{})
	go func() {
		defer close(prodDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n := seq.Add(1)
			hub.Broadcast([]byte(strconv.FormatInt(n, 10)))
			runtime.Gosched()
			for !painted.Load() && seq.Load() >= 1500 { // stay under the paint-backlog limit while the capture runs
				runtime.Gosched()
			}
			// Once painting is over, stay within the 100-deep queue so the stress cannot trip slow eviction.
			for painted.Load() && seq.Load()-paintSeq.Load()-int64(tr.count()) > 60 {
				runtime.Gosched()
			}
		}
	}()
	for seq.Load() < int64(it%5)*20 { // head start for the producer
		runtime.Gosched()
	}

	var snapSeq int64
	require.NoError(t, hub.FirstPaint(id, func() error {
		snapSeq = seq.Load()
		for i := 0; i < it%7*50; i++ { // capture/write latency
			runtime.Gosched()
		}
		_ = tr.Send([]byte("paint"))
		return nil
	}))
	paintSeq.Store(snapSeq)
	painted.Store(true)
	for seq.Load() < snapSeq+20 { // let the producer run past the capture
		runtime.Gosched()
	}
	close(stop)
	<-prodDone
	last := seq.Load()

	delivered := func() map[int64]bool {
		got := map[int64]bool{}
		for _, e := range tr.snapshot() {
			if n, err := strconv.ParseInt(e, 10, 64); err == nil {
				got[n] = true
			}
		}
		return got
	}
	missing := func() []int64 {
		got := delivered()
		var m []int64
		for n := snapSeq + 1; n <= last; n++ { // lower ones are in the capture
			if !got[n] {
				m = append(m, n)
			}
		}
		return m
	}
	require.Eventually(t, func() bool { return len(missing()) == 0 }, 5*time.Second, time.Millisecond,
		"iteration %d: frames after the capture (saw %d, last %d) were lost: missing %v, events %d, first %v", it, snapSeq, last, missing(), len(tr.snapshot()), tr.snapshot()[:min(6, len(tr.snapshot()))])
	require.Equal(t, "paint", tr.snapshot()[0], "iteration %d", it)
}

// A socket that stalls right after the paint must still be evicted: frames held during the paint
// and replayed by the writer may not bypass the slow-subscriber grace.
func TestFirstPaint_should_EvictSubscriber_When_TransportStallsDuringReplay(t *testing.T) {
	defer goleak.VerifyNone(t)
	hub := streamhub.NewStreamHub("s", newFakeSessionController(),
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithSubscriberBufferSize(4),
		streamhub.WithSlowSubscriberGrace(20*time.Millisecond))
	blocked := streamhub.NewMemoryTransport(streamhub.WithBlockingSend())
	id := hub.AttachSubscriber(blocked, streamhub.SubscriberCapability{ManualFirstPaint: true})
	defer hub.DetachSubscriber(id)

	require.NoError(t, hub.FirstPaint(id, func() error {
		hub.Broadcast([]byte("held")) // lands in the paint backlog, so the writer's first Send blocks
		return nil
	}))
	require.Eventually(t, func() bool {
		hub.Broadcast([]byte("live"))
		return hub.SubscriberCount() == 0
	}, 5*time.Second, time.Millisecond, "a stalled replay must trip the slow-subscriber eviction")
	require.Equal(t, int64(1), hub.SlowSubscriberDropsTotal())
}
