package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

// recency is a fake "when did the user last touch this session" source.
type recency map[string]time.Time

func (r recency) lastInteraction(key string) time.Time { return r[key] }

func TestAttachGate_should_LimitConcurrentAttaches(t *testing.T) {
	t.Parallel()
	g := newAttachGate(2, recency{}.lastInteraction)
	r1, _, err := g.acquire(context.Background(), "a")
	require.NoError(t, err)
	r2, _, err := g.acquire(context.Background(), "b")
	require.NoError(t, err)

	got := make(chan struct{})
	go func() {
		release, _, err := g.acquire(context.Background(), "c")
		if assert.NoError(t, err) {
			release()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("a third attach ran while two were in flight")
	case <-time.After(50 * time.Millisecond):
	}
	r1()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued attach never started after a slot freed")
	}
	r2()
}

func TestAttachGate_should_StartTheMostRecentlyUsedSessionFirst(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	g := newAttachGate(1, recency{"fg": now, "old": now.Add(-time.Hour)}.lastInteraction)
	hold, _, err := g.acquire(context.Background(), "first")
	require.NoError(t, err)

	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	for _, key := range []string{"never", "old", "fg"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, _, err := g.acquire(context.Background(), key)
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, key)
			mu.Unlock()
			release()
		}()
		require.Eventually(t, func() bool { return g.queued() == len(order)+queuedAfter(key) }, 5*time.Second, time.Millisecond)
	}
	hold()
	wg.Wait()

	assert.Equal(t, []string{"fg", "old", "never"}, order)
}

// queuedAfter is how many waiters are queued once key has joined (1, 2, 3 for the loop above).
func queuedAfter(key string) int {
	return map[string]int{"never": 1, "old": 2, "fg": 3}[key]
}

func TestAttachGate_should_KeepArrivalOrder_ForSessionsNeverTouched(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	hold, _, err := g.acquire(context.Background(), "first")
	require.NoError(t, err)

	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	for i, key := range []string{"x", "y", "z"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, _, err := g.acquire(context.Background(), key)
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, key)
			mu.Unlock()
			release()
		}()
		require.Eventually(t, func() bool { return g.queued() == i+1 }, 5*time.Second, time.Millisecond)
	}
	hold()
	wg.Wait()

	assert.Equal(t, []string{"x", "y", "z"}, order)
}

func TestAttachGate_should_DropAWaiter_When_ItsContextEnds(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	hold, _, err := g.acquire(context.Background(), "a")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := g.acquire(ctx, "b")
		done <- err
	}()
	require.Eventually(t, func() bool { return g.queued() == 1 }, 5*time.Second, time.Millisecond)
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	assert.Equal(t, 0, g.queued())
	hold()
	release, _, err := g.acquire(context.Background(), "c")
	require.NoError(t, err, "the abandoned waiter must not keep a slot")
	release()
}

func TestGatedAttach_should_RunOpenUnderTheLimit_AndReturnItsResult(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	var running, peak int32
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			alive, err := gatedAttach(context.Background(), g, termlat.Labels{Backend: "tymux"}, "s", func() (bool, error) {
				mu.Lock()
				running++
				peak = max(peak, running)
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				running--
				mu.Unlock()
				return true, nil
			})
			assert.NoError(t, err)
			assert.True(t, alive)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, peak, "never more than the limit at once")
}

func TestGatedAttach_should_PropagateTheOpenError(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	boom := errors.New("daemon down")
	_, err := gatedAttach(context.Background(), g, termlat.Labels{}, "s", func() (bool, error) { return false, boom })
	require.ErrorIs(t, err, boom)
	release, _, acqErr := g.acquire(context.Background(), "next")
	require.NoError(t, acqErr, "a failed attach frees its slot")
	release()
}

func TestGatedAttach_should_FailWhileQueued_When_TheConnectionEnds(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	hold, _, err := g.acquire(context.Background(), "a")
	require.NoError(t, err)
	defer hold()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := gatedAttach(ctx, g, termlat.Labels{}, "b", func() (bool, error) { return true, nil })
		done <- err
	}()
	require.Eventually(t, func() bool { return g.queued() == 1 }, 5*time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestAttachGate_should_ReportHowLongAnAttachWaited(t *testing.T) {
	t.Parallel()
	g := newAttachGate(1, recency{}.lastInteraction)
	_, waited, err := g.acquire(context.Background(), "a")
	require.NoError(t, err)
	assert.Zero(t, waited, "an uncontended attach does not wait")
}
