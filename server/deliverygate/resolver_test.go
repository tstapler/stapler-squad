package deliverygate

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeLister counts calls and can block until released.
type fakeLister struct {
	calls   atomic.Int64
	started chan struct{} // receives one token per call
	block   chan struct{} // nil = return immediately
	data    []session.InstanceData
	err     error
}

func newFakeLister() *fakeLister { return &fakeLister{started: make(chan struct{}, 64)} }

func (f *fakeLister) ListInstanceData() ([]session.InstanceData, error) {
	f.calls.Add(1)
	f.started <- struct{}{}
	if f.block != nil {
		<-f.block
	}
	return f.data, f.err
}

func gateWithLister(l InstanceDataLister, timeout chan time.Time) (*Gate, *fakeClock) {
	clk := newFakeClock()
	lg, _ := newRecLogger()
	opts := []Option{WithClock(clk.Now), WithLogger(lg), WithFlagLoader((&staticFlags{}).load), WithInstanceLister(l)}
	if timeout != nil {
		opts = append(opts, WithRefreshTimeout(func() <-chan time.Time { return timeout }))
	}
	return NewGate(opts...), clk
}

func TestResolve_ShouldReturnUnresolvedAndNotCache_WhenHookPublishedBeforeUpsert(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false)
	if got := g.Resolver().Resolve("review:abc", nil); got.Visibility != VisibilityUnresolved {
		t.Fatalf("before upsert: %+v", got)
	}
	g.Index().Upsert(hiddenReview)
	if got := g.Resolver().Resolve("review:abc", nil); got.Visibility != VisibilityHidden {
		t.Fatalf("after upsert: %+v (negative result must not be cached)", got)
	}
}

func TestResolve_ShouldNotBlockAndRefreshOnce_When200ConcurrentMissesAgainstBlockedStorage(t *testing.T) {
	t.Parallel()
	l := newFakeLister()
	l.block = make(chan struct{})
	timeout := make(chan time.Time)
	g, _ := gateWithLister(l, timeout)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := g.Resolver().Resolve(fmt.Sprintf("missing-%d", i%50), nil)
			if r.Visibility != VisibilityUnresolved {
				t.Errorf("miss %d: %+v", i, r)
			}
		}(i)
	}
	wg.Wait() // all 200 returned although the lister is blocked: no synchronous I/O
	<-l.started
	if got := l.calls.Load(); got != 1 {
		t.Fatalf("storage listed %d times, want 1 (singleflight)", got)
	}
	close(timeout) // abandon the blocked refresh
	g.Resolver().Wait()
	if got := g.Metrics().Value(CounterIndexRefresh, RefreshTimeout); got != 1 {
		t.Fatalf("refresh{timeout} = %d, want 1", got)
	}
	close(l.block)
}

func TestResolve_ShouldDefineNotASessionPositively_WhenEmptyItemIDOrSystemID(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false)
	r := g.Resolver()
	cases := []struct {
		name string
		id   string
		md   map[string]string
		want Visibility
	}{
		{"empty id", "", nil, VisibilityNotASession},
		{"item id", "item-77", map[string]string{events.MetadataKeyItemID: "item-77"}, VisibilityNotASession},
		{"system id", "fork-pressure", nil, VisibilityNotASession},
		{"prefixed system id", "bulk-reset:all", nil, VisibilityNotASession},
		{"unknown looks like a session", "deadbeef-0000", nil, VisibilityUnresolved},
	}
	for _, c := range cases {
		if got := r.Resolve(c.id, c.md).Visibility; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestResolve_ShouldReturnHiddenNotNotASession_WhenItemIDEqualsSessionUUID(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false, hiddenReview)
	// Notify(inst.UUID, ...) stamps item_id=<session UUID>; the slot itself is also the UUID.
	got := g.Resolver().Resolve("u-h1", map[string]string{events.MetadataKeyItemID: "u-h1"})
	if got.Visibility != VisibilityHidden {
		t.Fatalf("got %+v", got)
	}
	// SessionID unknown but item_id is a known hidden session's UUID: still hidden.
	got = g.Resolver().Resolve("who-knows", map[string]string{events.MetadataKeyItemID: "u-h1"})
	if got.Visibility != VisibilityHidden {
		t.Fatalf("item_id fallback: %+v", got)
	}
}

func TestResolve_ShouldReturnHidden_WhenCapacityMonitorEventHasNilMetadata(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false, hiddenReview)
	if got := g.Resolver().Resolve("review:abc", nil).Visibility; got != VisibilityHidden {
		t.Fatalf("nil metadata: %s", got)
	}
}

func TestRefresh_ShouldCallListAtMostOncePer5s_WhenMissesAndPreviousAttemptFailedOrTimedOut(t *testing.T) {
	t.Parallel()
	l := newFakeLister()
	l.err = errors.New("storage down")
	g, clk := gateWithLister(l, nil)

	// 500 misses over 100 ids across 20s of fake time (one batch per second).
	for sec := 0; sec < 20; sec++ {
		for i := 0; i < 25; i++ {
			g.Resolver().Resolve(fmt.Sprintf("id-%d", (sec*25+i)%100), nil)
		}
		g.Resolver().wg.Wait() // let an in-flight refresh finish before the next second
		clk.Advance(time.Second)
	}
	if got := l.calls.Load(); got > 4 {
		t.Fatalf("ListInstanceData called %d times in 20s, want <= 4", got)
	}
	if g.Metrics().Value(CounterIndexRefresh, RefreshSkippedMinInterval) == 0 {
		t.Error("skipped_min_interval never counted")
	}
	if g.Metrics().Value(CounterIndexRefresh, RefreshError) == 0 {
		t.Error("failed attempts must still count against the interval")
	}
}

// T-MX-12: a key that missed but exists in storage after the refresh is an index defect.
func TestGate_ShouldCountUnresolvedResolvedLater_WhenRefreshFindsSessionInStorage(t *testing.T) {
	t.Parallel()
	l := newFakeLister()
	l.data = []session.InstanceData{{UUID: "u-late", Title: "late-session", Hidden: true, Tags: []string{"backlog:review"}}}
	g, _ := gateWithLister(l, nil)

	if got := g.Resolver().Resolve("late-session", nil).Visibility; got != VisibilityUnresolved {
		t.Fatalf("first lookup: %s", got)
	}
	g.Resolver().wg.Wait()
	if got := g.Metrics().Value(CounterResolvedLater); got != 1 {
		t.Fatalf("resolved_later = %d, want 1", got)
	}
	if got := g.Resolver().Resolve("late-session", nil); got.Visibility != VisibilityHidden || got.Kind != KindReview {
		t.Fatalf("after refresh: %+v", got)
	}
}
