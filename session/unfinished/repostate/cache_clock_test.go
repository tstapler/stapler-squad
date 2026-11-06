package repostate

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func newBlobCache(budget int64) *RepoCache[string, []byte] {
	return NewRepoCache[string, []byte](budget, 0, sizeOfLen, Options{})
}

// liveBytes sums the sizes of the entries the index holds; at quiesce it must
// equal Bytes() (exactly-once accounting).
func liveBytes[K comparable, V any](c *RepoCache[K, V]) (sum int64, n int) {
	c.core.Load().index.Range(func(_ K, e *clockEntry[K, V]) bool {
		if e.state.Load()&entryEvicted != 0 {
			return true // retired but not yet unindexed: not live
		}
		sum += e.size
		n++
		return true
	})
	return sum, n
}

func assertQuiesced[K comparable, V any](t *testing.T, label string, c *RepoCache[K, V], budget int64) {
	t.Helper()
	sum, n := liveBytes(c)
	if got := c.Bytes(); got != sum {
		t.Errorf("%s: Bytes()=%d, sum of %d live entries=%d", label, got, n, sum)
	}
	if c.Bytes() > budget {
		t.Errorf("%s: Bytes()=%d over budget %d at quiesce", label, c.Bytes(), budget)
	}
	if c.Bytes() < 0 {
		t.Errorf("%s: negative Bytes()=%d", label, c.Bytes())
	}
}

// lruModel is the retained reference: the previous mutex-guarded LRU's
// observable behaviour for a scripted sequence.
type lruModel struct {
	budget int64
	order  []string // most recently used last
	sizes  map[string]int64
	evicts int
}

func (m *lruModel) get(k string) bool {
	for i, o := range m.order {
		if o == k {
			m.order = append(append(m.order[:i:i], m.order[i+1:]...), k)
			return true
		}
	}
	return false
}

func (m *lruModel) put(k string, size int64) {
	m.sizes[k] = size
	m.order = append(m.order, k)
	var total int64
	for _, o := range m.order {
		total += m.sizes[o]
	}
	for total > m.budget {
		victim := m.order[0]
		m.order = m.order[1:]
		total -= m.sizes[victim]
		m.evicts++
	}
}

func TestClockEviction_ShouldMatchLRUObservableResult_AndNeverDeleteReinsertedKey(t *testing.T) {
	// Same observable result as Story 1.1.2's LRU test: K1 re-read before K3.
	c := newBlobCache(1000)
	model := &lruModel{budget: 1000, sizes: map[string]int64{}}
	c.Put("K1", make([]byte, 400))
	model.put("K1", 400)
	c.Put("K2", make([]byte, 400))
	model.put("K2", 400)
	if _, ok := c.Get("K1"); !ok || !model.get("K1") {
		t.Fatal("K1 missing before eviction")
	}
	c.Put("K3", make([]byte, 400))
	model.put("K3", 400)
	for _, k := range []string{"K1", "K2", "K3"} {
		_, got := c.Get(k)
		if want := model.get(k); got != want {
			t.Errorf("%s retained=%v, LRU model %v", k, got, want)
		}
	}
	if st := c.Stats(); st.Evictions != 1 || st.Bytes != 800 || int(st.Evictions) != model.evicts {
		t.Errorf("Evictions=%d Bytes=%d, want 1 and 800 (model evictions %d)", st.Evictions, st.Bytes, model.evicts)
	}

	// Compare-and-delete: a key re-inserted between the evicting CAS and the
	// index removal must survive.
	d := newBlobCache(1000)
	d.Put("K1", make([]byte, 300))
	d.Put("K2", make([]byte, 300))
	d.Put("K3", make([]byte, 300))
	armed := true
	d.counters.beforeUnindex = func() {
		if !armed {
			return
		}
		armed = false
		d.Put("K1", []byte("fresh")) // same key, new entry, while the evicted one is still indexed
	}
	d.Put("K4", make([]byte, 300)) // 1200 > 1000: the hand evicts K1 (oldest, unreferenced)
	got, ok := d.Get("K1")
	if !ok || string(got) != "fresh" {
		t.Fatalf("re-inserted K1 = %q, %v; compare-and-delete must not remove it", got, ok)
	}
	assertQuiesced(t, "reinsert", d, 1000)
	if st := d.Stats(); st.Entries != 4 {
		t.Errorf("Entries = %d, want 4 (K1', K2, K3, K4)", st.Entries)
	}
}

func TestClockEviction_ShouldGiveSecondChance_WhenHandMeetsReferencedEntry(t *testing.T) {
	c := newBlobCache(900)
	for _, k := range []string{"a", "b", "c"} {
		c.Put(k, make([]byte, 300))
	}
	for _, k := range []string{"a", "b", "c"} {
		if _, ok := c.Get(k); !ok {
			t.Fatal("missing before eviction")
		}
	}
	c.Put("d", make([]byte, 300)) // all referenced: hand clears a,b,c, spares d (being inserted), evicts a
	if st := c.Stats(); st.Evictions != 1 || st.Bytes != 900 {
		t.Fatalf("Evictions=%d Bytes=%d, want 1 and 900", st.Evictions, st.Bytes)
	}
	if got := c.ClockStats().SecondChances; got != 3 {
		t.Errorf("SecondChances = %d, want 3", got)
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a survived: hand order is insertion order after the clear lap")
	}
	if _, ok := c.Get("d"); !ok {
		t.Error("d, the entry being inserted, was evicted by its own insert")
	}
	assertQuiesced(t, "second chance", c, 900)
}

func TestClockEviction_ShouldKeepEvictedValueReadable_WhenReaderHoldsIt(t *testing.T) {
	c := newBlobCache(500)
	payload := []byte("held-by-reader")
	c.Put("k", append([]byte(nil), payload...))
	core := c.core.Load()
	held, ok := core.index.Load("k")
	if !ok {
		t.Fatal("entry missing")
	}
	c.Put("x", make([]byte, 495)) // evicts k
	if _, ok := c.Get("k"); ok {
		t.Fatal("k not evicted")
	}
	runtime.GC()
	if string(held.val) != string(payload) || held.state.Load()&entryEvicted == 0 {
		t.Errorf("held entry = %q state %b, want intact value marked Evicted", held.val, held.state.Load())
	}
}

func TestClockEviction_ShouldBoundRing_WhenSameKeyReplacedManyTimes(t *testing.T) {
	c := newBlobCache(1 << 20)
	for range 20_000 {
		c.Put("hot", make([]byte, 10))
	}
	if got, limit := c.core.Load().ring.length(), uint64(2*1+clockChunkSlots+8); got > limit {
		t.Errorf("ring length %d after 20,000 replacements of one key, want <= %d", got, limit)
	}
	assertQuiesced(t, "replace", c, 1<<20)
	if st := c.Stats(); st.Entries != 1 || st.Bytes != 10 {
		t.Errorf("Entries=%d Bytes=%d, want 1 and 10", st.Entries, st.Bytes)
	}
}

func TestClockEviction_ShouldReleaseChunks_WhenHandAdvancesPastThem(t *testing.T) {
	c := newBlobCache(1000)
	for i := range 5 * clockChunkSlots {
		c.Put(fmt.Sprint("k", i), make([]byte, 100))
	}
	core := c.core.Load()
	if base := core.ring.head.Load().base; base == 0 {
		t.Error("head chunk never advanced: consumed chunks are retained")
	}
	assertQuiesced(t, "chunks", c, 1000)
}

func TestClockCache_ShouldPadHandAndTailToACacheLine_WhenLaidOut(t *testing.T) {
	var r clockRing[string, int]
	if sz := unsafe.Sizeof(r.hand); sz < 128 {
		t.Errorf("hand occupies %d bytes, want it isolated on its own cache lines (>= 128)", sz)
	}
	if unsafe.Offsetof(r.hand) == unsafe.Offsetof(r.tail) {
		t.Error("hand and tail share a slot")
	}
}

// stressCache runs 8 goroutines inserting, hitting and evicting one hot key and
// a rotating key set with the yield hook, then checks accounting at quiesce.
func stressCache(t *testing.T, seed int64, procs int) {
	const (
		workers = 8
		ops     = 4000
		maxSize = 64
		budget  = 1000
	)
	c := NewRepoCache[string, []byte](budget, 0, sizeOfLen, Options{})
	c.yield = yieldHook(seed)
	var overshoot atomic.Int64
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed*100 + int64(w)))
			for range ops {
				key := "hot"
				if rng.Intn(2) == 0 {
					key = fmt.Sprint("k", rng.Intn(40))
				}
				switch rng.Intn(10) {
				case 0, 1, 2, 3:
					c.Put(key, make([]byte, 1+rng.Intn(maxSize)))
				case 4:
					c.Delete(key)
				default:
					c.Get(key)
				}
				if over := c.Bytes() - budget; over > overshoot.Load() {
					overshoot.Store(over)
				}
			}
		}()
	}
	wg.Wait()
	assertQuiesced(t, fmt.Sprintf("seed %d procs %d", seed, procs), c, budget)
	if got, limit := overshoot.Load(), int64(workers*maxSize); got > limit {
		t.Errorf("seed %d: transient overshoot %d bytes, want <= %d (one in-flight entry per goroutine)", seed, got, limit)
	}
	t.Logf("seed %d procs %d: evictions=%d casRetries=%d secondChances=%d overshoot=%dB", seed, procs, c.Stats().Evictions, c.ClockStats().CASRetries, c.ClockStats().SecondChances, overshoot.Load())
}

func TestClockCache_ShouldKeepExactBytesAndBudget_WhenEightGoroutinesStressUnderYieldHook(t *testing.T) {
	for _, procs := range []int{1, 2, 8} {
		prev := runtime.GOMAXPROCS(procs)
		for _, seed := range stressSeeds {
			stressCache(t, seed, procs)
		}
		runtime.GOMAXPROCS(prev)
	}
}

func TestClockCache_ShouldNeverLoseLiveHotKey_WhenReplacedConcurrentlyWithinBudget(t *testing.T) {
	c := NewRepoCache[string, int](1<<20, 0, func(int) int { return 8 }, Options{})
	c.yield = yieldHook(3)
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 3000 {
				c.Put("hot", w*10_000+i)
				if _, ok := c.Get("hot"); !ok {
					t.Error("hot key missing after its Put completed with no eviction pressure")
					return
				}
			}
		}()
	}
	wg.Wait()
	assertQuiesced(t, "hot replace", c, 1<<20)
	if st := c.Stats(); st.Entries != 1 || st.Bytes != 8 {
		t.Errorf("Entries=%d Bytes=%d, want 1 and 8", st.Entries, st.Bytes)
	}
}

func TestClockCache_ShouldStayConsistent_WhenClearRacesWithUse(t *testing.T) {
	c := newBlobCache(2000)
	c.yield = yieldHook(5)
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 2000 {
				c.Put(fmt.Sprint("k", (w*7+i)%25), make([]byte, 50))
				c.Get(fmt.Sprint("k", i%25))
				if i%500 == 0 {
					c.Clear()
				}
			}
		}()
	}
	wg.Wait()
	assertQuiesced(t, "clear race", c, 2000)
}

// BenchmarkRepoCacheParallel reports hit/insert throughput with the hand
// sweeping (advisory: wall-clock is never a gate).
func BenchmarkRepoCacheParallel(b *testing.B) {
	c := NewRepoCache[int, [8]byte](64<<10, 0, func([8]byte) int { return 64 }, Options{})
	for i := range 512 {
		c.Put(i, [8]byte{})
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(rand.Int63()))
		for pb.Next() {
			k := rng.Intn(2048)
			if rng.Intn(10) == 0 {
				c.Put(k, [8]byte{})
			} else {
				c.Get(k)
			}
		}
	})
	b.ReportMetric(float64(c.ClockStats().CASRetries), "cas_retries")
}

func TestClockCore_ShouldReleaseHeldCount_WhenPopYieldPanicsInSweepOrCompact(t *testing.T) {
	newCore := func() *clockCore[string, int] {
		k := newClockCore[string, int]()
		e := &clockEntry[string, int]{key: "a", size: 10}
		k.index.Store("a", e)
		k.bytes.Add(e.size)
		k.ring.push(e)
		return k
	}
	boom := func() { panic("injected") }
	run := func(f func()) (recovered any) {
		defer func() { recovered = recover() }()
		f()
		return nil
	}

	k := newCore()
	if run(func() { k.sweep(0, nil, boom, &clockCounters{}) }) == nil {
		t.Fatal("sweep did not panic through the injected yield")
	}
	if got := k.held.Load(); got != 0 {
		t.Errorf("held=%d after a panicking sweep step, want 0", got)
	}

	k = newCore()
	if run(func() { k.compactStep(boom, &clockCounters{}) }) == nil {
		t.Fatal("compactStep did not panic through the injected yield")
	}
	if got := k.held.Load(); got != 0 {
		t.Errorf("held=%d after a panicking compact step, want 0", got)
	}
}
