package scanqueue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the queue")
		panic("unreachable")
	}
}

// recorder collects handled paths in order and signals each.
type recorder struct {
	mu    sync.Mutex
	paths []string
	ch    chan string
}

func newRecorder() *recorder { return &recorder{ch: make(chan string, 1024)} }

func (r *recorder) handle(p string) {
	r.mu.Lock()
	r.paths = append(r.paths, p)
	r.mu.Unlock()
	r.ch <- p
}

func start(t *testing.T, q *Queue) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	q.Start(ctx)
}

func TestQueue_RepeatedEnqueueOfPendingPathRunsOnce(t *testing.T) {
	t.Parallel()
	rec := newRecorder()
	q := New(Config{Workers: 1}, rec.handle)
	for i := 0; i < 500; i++ {
		q.Enqueue("a")
		q.EnqueueBackfill("a")
	}
	assert.Equal(t, 1, q.Pending())
	start(t, q)

	assert.Equal(t, "a", recv(t, rec.ch))
	assert.Equal(t, uint64(999), q.Stats().Coalesced)
	assert.Equal(t, uint64(1000), q.Stats().Enqueued)
}

func TestQueue_ChangeWhileRunningReRunsOnceAfterward(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	runs := 0
	done := make(chan struct{}, 4)
	q := New(Config{Workers: 2}, func(string) {
		mu.Lock()
		runs++
		first := runs == 1
		mu.Unlock()
		if first {
			entered <- struct{}{}
			<-release
		}
		done <- struct{}{}
	})
	start(t, q)

	q.Enqueue("a")
	recv(t, entered)
	for i := 0; i < 10; i++ { // changes while the first pass is mid-parse
		q.Enqueue("a")
	}
	close(release)
	recv(t, done)
	recv(t, done)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, runs, "ten mid-run changes collapse into one follow-up pass (newest wins, none lost)")
}

func TestQueue_LiveRunsBeforeBackfillAndPromotesPendingBackfill(t *testing.T) {
	t.Parallel()
	rec := newRecorder()
	q := New(Config{Workers: 1}, rec.handle)
	q.EnqueueBackfill("b1")
	q.EnqueueBackfill("b2")
	q.EnqueueBackfill("b3")
	q.Enqueue("live")
	q.Enqueue("b3") // promoted ahead of the remaining backfill
	start(t, q)

	got := []string{recv(t, rec.ch), recv(t, rec.ch), recv(t, rec.ch), recv(t, rec.ch)}
	assert.Equal(t, []string{"live", "b3", "b1", "b2"}, got)
}

func TestQueue_StartDelayHoldsBackfillButNotLive(t *testing.T) {
	t.Parallel()
	rec := newRecorder()
	gate := make(chan struct{})
	slept := make(chan time.Duration, 1)
	q := New(Config{
		Workers: 1, StartDelay: 30 * time.Second,
		Sleep: func(ctx context.Context, d time.Duration) {
			slept <- d
			select {
			case <-gate:
			case <-ctx.Done():
			}
		},
	}, rec.handle)
	q.EnqueueBackfill("backfill")
	q.Enqueue("live")
	start(t, q)

	assert.Equal(t, 30*time.Second, recv(t, slept))
	assert.Equal(t, "live", recv(t, rec.ch), "live work does not wait for the start delay")
	select {
	case p := <-rec.ch:
		t.Fatalf("backfill %q ran before the start delay elapsed", p)
	default:
	}
	close(gate)
	assert.Equal(t, "backfill", recv(t, rec.ch))
}

func TestQueue_BackfillIsThrottledToDutyAndLiveIsNot(t *testing.T) {
	t.Parallel()
	var clock struct {
		sync.Mutex
		now time.Time
	}
	advance := func(d time.Duration) { clock.Lock(); clock.now = clock.now.Add(d); clock.Unlock() }
	sleeps := make(chan time.Duration, 8)
	busy := false
	var busyMu sync.Mutex
	rec := newRecorder()
	q := New(Config{
		Workers: 1, BackfillDuty: 0.25,
		Busy: func() bool { busyMu.Lock(); defer busyMu.Unlock(); return busy },
		Now:  func() time.Time { clock.Lock(); defer clock.Unlock(); return clock.now },
		Sleep: func(_ context.Context, d time.Duration) {
			sleeps <- d
		},
	}, func(p string) { advance(100 * time.Millisecond); rec.handle(p) })

	q.EnqueueBackfill("b1")
	q.Enqueue("l1")
	q.EnqueueBackfill("b2")
	start(t, q)
	for i := 0; i < 3; i++ {
		recv(t, rec.ch)
	}

	// 100ms of work at 25% duty idles 300ms; the live item adds no sleep.
	require.Equal(t, 300*time.Millisecond, recv(t, sleeps))
	require.Equal(t, 300*time.Millisecond, recv(t, sleeps))
	select {
	case d := <-sleeps:
		t.Fatalf("unexpected extra sleep %v (live item must not be throttled)", d)
	default:
	}

	busyMu.Lock()
	busy = true
	busyMu.Unlock()
	q.EnqueueBackfill("b3")
	recv(t, rec.ch)
	assert.Equal(t, 1500*time.Millisecond, recv(t, sleeps), "a busy host quarters the duty")
}
