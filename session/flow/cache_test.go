package flow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cacheTestClock is an advanceable Clock so TTL tests never sleep.
type cacheTestClock struct {
	mu sync.Mutex
	t  time.Time
}

func newCacheTestClock() *cacheTestClock {
	return &cacheTestClock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *cacheTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *cacheTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type cacheTestValue struct {
	QueryMs int64
	AsOf    time.Time
}

func newTestCache(clock Clock) *Cache[*cacheTestValue] {
	return NewCache[*cacheTestValue](clock, CacheOptions{DefaultConfig: DefaultConfig()})
}

func TestCache_should_CoalesceToOneSourceCall_When_10ConcurrentSummaries(t *testing.T) {
	t.Parallel()
	c := newTestCache(newCacheTestClock())
	var calls atomic.Int32
	entered := make(chan struct{}, 10)
	release := make(chan struct{})
	compute := func(context.Context) (*cacheTestValue, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return &cacheTestValue{QueryMs: 7}, nil
	}
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	var wg sync.WaitGroup
	results := make([]*cacheTestValue, 10)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Do(context.Background(), q, compute)
			assert.NoError(t, err)
			results[i] = v
		}()
	}
	<-entered // leader is inside compute; followers coalesce behind it
	close(release)
	wg.Wait()

	assert.EqualValues(t, 1, calls.Load())
	for _, r := range results {
		assert.Same(t, results[0], r)
	}
}

func TestCache_should_ShareEntryAndBoundTo64_When_WindowSpecKeyAnd200DistinctKeys(t *testing.T) {
	t.Parallel()
	clock := newCacheTestClock()
	c := newTestCache(clock)
	var calls atomic.Int32
	compute := func(context.Context) (*cacheTestValue, error) {
		calls.Add(1)
		return &cacheTestValue{}, nil
	}
	ctx := context.Background()

	q30 := FlowQuery{Window: WindowSpec{Days: 30}}
	_, err := c.Do(ctx, q30, compute)
	require.NoError(t, err)
	clock.Advance(time.Second)
	_, err = c.Do(ctx, q30, compute)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load(), "calls 1s apart share one entry")

	for d := 1; d <= 200; d++ {
		_, err := c.Do(ctx, FlowQuery{Window: WindowSpec{Days: d, OffsetDays: 1}}, compute)
		require.NoError(t, err)
	}
	assert.LessOrEqual(t, c.Len(), 64)
	assert.Equal(t, 64, c.Len())

	// Least recently used is evicted: the earliest key is gone, the latest stays.
	before := calls.Load()
	_, err = c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 200, OffsetDays: 1}}, compute)
	require.NoError(t, err)
	assert.Equal(t, before, calls.Load(), "most recent key still cached")
	_, err = c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 1, OffsetDays: 1}}, compute)
	require.NoError(t, err)
	assert.Equal(t, before+1, calls.Load(), "oldest key was evicted")
}

func TestCache_should_BypassCache_When_NonDefaultConfigOrItemIdsFilter(t *testing.T) {
	t.Parallel()
	c := newTestCache(newCacheTestClock())
	var calls atomic.Int32
	compute := func(context.Context) (*cacheTestValue, error) {
		calls.Add(1)
		return &cacheTestValue{}, nil
	}
	ctx := context.Background()

	for i := 1; i <= 20; i++ {
		cfg := DefaultConfig()
		cfg.ThrashMinFlips += i
		_, err := c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 30}, Config: cfg}, compute)
		require.NoError(t, err)
	}
	_, err := c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 30}, ItemIDs: []ItemID{"a"}}, compute)
	require.NoError(t, err)
	assert.Zero(t, c.Len(), "bypassing queries never grow the map")

	// Repeating a bypass query recomputes (not cached).
	cfg := DefaultConfig()
	cfg.LowSampleN = 99
	q := FlowQuery{Window: WindowSpec{Days: 30}, Config: cfg}
	before := calls.Load()
	_, _ = c.Do(ctx, q, compute)
	_, _ = c.Do(ctx, q, compute)
	assert.EqualValues(t, before+2, calls.Load())

	// An unset (zero) Config means defaults and is cacheable.
	_, err = c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 7}}, compute)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Len())
}

func TestCache_should_ExpireAt31sNotAt29s_When_InjectedClockAdvances(t *testing.T) {
	t.Parallel()
	clock := newCacheTestClock()
	c := newTestCache(clock)
	var calls atomic.Int32
	compute := func(context.Context) (*cacheTestValue, error) {
		n := calls.Add(1)
		return &cacheTestValue{QueryMs: int64(n) * 10, AsOf: clock.Now()}, nil
	}
	ctx := context.Background()
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	first, err := c.Do(ctx, q, compute)
	require.NoError(t, err)

	clock.Advance(29 * time.Second)
	hit, err := c.Do(ctx, q, compute)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.Equal(t, first.QueryMs, hit.QueryMs, "hit reports original QueryMs")
	assert.Equal(t, first.AsOf, hit.AsOf, "hit reports original AsOf")

	clock.Advance(2 * time.Second) // 31s since compute
	fresh, err := c.Do(ctx, q, compute)
	require.NoError(t, err)
	assert.EqualValues(t, 2, calls.Load())
	assert.Equal(t, int64(20), fresh.QueryMs)
}

func TestCache_should_RunOneUncachedComputeAtATime_When_FiveDistinctConfigsConcurrent(t *testing.T) {
	t.Parallel()
	c := newTestCache(newCacheTestClock())
	var inflight, maxInflight atomic.Int32
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	compute := func(context.Context) (*cacheTestValue, error) {
		n := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if n <= m || maxInflight.CompareAndSwap(m, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		inflight.Add(-1)
		return &cacheTestValue{}, nil
	}

	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		cfg := DefaultConfig()
		cfg.DeadlockMinRun += i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Do(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}, Config: cfg}, compute)
			assert.NoError(t, err)
		}()
	}
	for i := 0; i < 5; i++ {
		<-entered
		release <- struct{}{}
	}
	wg.Wait()
	assert.EqualValues(t, 1, maxInflight.Load(), "exactly one source read in flight at a time")
}

func TestCache_should_ReturnErrBusyAndSkipSlotForHits_When_SlotHeldAndWaiterExpires(t *testing.T) {
	t.Parallel()
	c := newTestCache(newCacheTestClock())
	ctx := context.Background()
	hitQ := FlowQuery{Window: WindowSpec{Days: 30}}
	_, err := c.Do(ctx, hitQ, func(context.Context) (*cacheTestValue, error) { return &cacheTestValue{QueryMs: 5}, nil })
	require.NoError(t, err)

	holderCfg := DefaultConfig()
	holderCfg.LowSampleN = 11
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Do(ctx, FlowQuery{Window: WindowSpec{Days: 90}, Config: holderCfg}, func(context.Context) (*cacheTestValue, error) {
			close(entered)
			<-release
			return &cacheTestValue{}, nil
		})
	}()
	<-entered

	hit, err := c.Do(ctx, hitQ, func(context.Context) (*cacheTestValue, error) {
		t.Error("cache hit must not compute")
		return &cacheTestValue{}, nil
	})
	require.NoError(t, err)
	assert.EqualValues(t, 5, hit.QueryMs, "hit served while the slot is held")

	waiterCfg := DefaultConfig()
	waiterCfg.LowSampleN = 12
	expired, cancel := context.WithCancel(ctx)
	cancel()
	_, err = c.Do(expired, FlowQuery{Window: WindowSpec{Days: 90}, Config: waiterCfg}, func(context.Context) (*cacheTestValue, error) {
		t.Error("expired waiter must not compute")
		return &cacheTestValue{}, nil
	})
	assert.ErrorIs(t, err, ErrBusy)

	close(release)
	<-done
}

func TestCache_should_NotCacheErrors_When_ComputeFails(t *testing.T) {
	t.Parallel()
	c := newTestCache(newCacheTestClock())
	boom := fmt.Errorf("boom")
	q := FlowQuery{Window: WindowSpec{Days: 30}}
	_, err := c.Do(context.Background(), q, func(context.Context) (*cacheTestValue, error) { return nil, boom })
	assert.ErrorIs(t, err, boom)
	assert.Zero(t, c.Len())
	v, err := c.Do(context.Background(), q, func(context.Context) (*cacheTestValue, error) { return &cacheTestValue{QueryMs: 1}, nil })
	require.NoError(t, err)
	assert.EqualValues(t, 1, v.QueryMs)
}
