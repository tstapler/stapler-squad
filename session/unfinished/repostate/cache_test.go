package repostate

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func sizeOfLen(v []byte) int { return len(v) }

func newByteCache(budget int64, ttl time.Duration, opts Options) *RepoCache[string, []byte] {
	return NewRepoCache[string, []byte](budget, ttl, sizeOfLen, opts)
}

func TestRepoCache_ShouldEvictLRUAndCountEvictions_WhenBudgetExceeded(t *testing.T) {
	c := newByteCache(1000, 0, Options{})
	c.Put("K1", make([]byte, 400))
	c.Put("K2", make([]byte, 400))
	if _, ok := c.Get("K1"); !ok {
		t.Fatal("K1 missing before eviction")
	}
	c.Put("K3", make([]byte, 400))

	if _, ok := c.Get("K2"); ok {
		t.Error("K2 survived, want evicted as least recently used")
	}
	for _, k := range []string{"K1", "K3"} {
		if _, ok := c.Get(k); !ok {
			t.Errorf("%s evicted, want retained", k)
		}
	}
	st := c.Stats()
	if st.Evictions != 1 || st.Bytes != 800 {
		t.Errorf("Evictions=%d Bytes=%d, want 1 and 800", st.Evictions, st.Bytes)
	}
}

func TestRepoCache_ShouldCoalesceToOneCompute_WhenEightGoroutinesRequestSameKey(t *testing.T) {
	const callers = 8
	c := newByteCache(1<<20, 0, Options{})
	var (
		computes  int
		computeMu sync.Mutex
		release   = make(chan struct{})
		entered   = make(chan struct{}, callers)
		wg        sync.WaitGroup
	)
	fn := func() ([]byte, error) {
		computeMu.Lock()
		computes++
		computeMu.Unlock()
		<-release
		return []byte("v"), nil
	}
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entered <- struct{}{}
			if _, err := c.GetOrCompute("k", fn); err != nil {
				t.Error(err)
			}
		}()
	}
	for range callers {
		<-entered
	}
	// The compute stays blocked until every caller is registered in the flight.
	deadline := time.Now().Add(10 * time.Second)
	for c.joined.Load() < callers {
		if time.Now().After(deadline) {
			t.Fatal("callers did not coalesce")
		}
		runtime.Gosched()
	}
	close(release)
	wg.Wait()

	if computes != 1 {
		t.Errorf("fn ran %d times, want 1", computes)
	}
	if got := c.Stats().Coalesced; got != callers-1 {
		t.Errorf("Coalesced = %d, want %d", got, callers-1)
	}
}

func TestRepoCache_ShouldNotStore_WhenComputeReturnsError(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	calls := 0
	fn := func() ([]byte, error) { calls++; return nil, ErrNotFound }
	for range 2 {
		if _, err := c.GetOrCompute("k", fn); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if calls != 2 {
		t.Errorf("fn ran %d times, want 2 (negatives are not cached)", calls)
	}
}

func TestRepoCache_ShouldRecoverPanic_WhenComputePanics(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	_, err := c.GetOrCompute("k", func() ([]byte, error) { panic("boom") })
	if err == nil {
		t.Fatal("want an error from a panicking compute")
	}
}

func TestRepoCache_ShouldExpireEntry_WhenFakeClockPassesTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newByteCache(1<<20, time.Minute, Options{Clock: func() time.Time { return now }})
	c.Put("k", []byte("v"))
	if _, ok := c.Get("k"); !ok {
		t.Fatal("fresh entry missing")
	}
	now = now.Add(time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Error("entry survived past its TTL")
	}
	if st := c.Stats(); st.Entries != 0 || st.Bytes != 0 {
		t.Errorf("Entries=%d Bytes=%d after expiry, want 0 and 0", st.Entries, st.Bytes)
	}
}

func TestRepoCache_ShouldSkipStore_WhenUnderPressure(t *testing.T) {
	pressure := true
	c := newByteCache(1<<20, 0, Options{PressureFunc: func() bool { return pressure }})
	c.Put("k", []byte("v"))
	if _, ok := c.Get("k"); ok {
		t.Error("stored under pressure")
	}
	pressure = false
	c.Put("k", []byte("v"))
	if _, ok := c.Get("k"); !ok {
		t.Error("not stored once pressure cleared")
	}
}

func TestRepoCache_ShouldDropEverything_WhenCleared(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	c.Put("a", []byte("x"))
	c.Put("b", []byte("y"))
	c.Delete("a")
	c.Clear()
	if st := c.Stats(); st.Entries != 0 || st.Bytes != 0 {
		t.Errorf("Entries=%d Bytes=%d after Clear, want 0 and 0", st.Entries, st.Bytes)
	}
}

func TestComputeMiss_ShouldNotRecompute_WhenPreviousFlightAlreadyPublished(t *testing.T) {
	c := NewRepoCache[string, []byte](1<<20, 0, sizeOfLen, Options{})
	c.Put("k", []byte("old")) // the previous flight finished between this caller's Get miss and its DoChan
	calls := 0
	v, err := c.computeMissKeep("k", func() ([]byte, error) { calls++; return []byte("new"), nil }, nil)
	if err != nil || string(v) != "old" || calls != 0 {
		t.Fatalf("got v=%q err=%v calls=%d, want old nil 0", v, err, calls)
	}
}
