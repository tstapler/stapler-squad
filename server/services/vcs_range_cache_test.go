package services

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
)

type rangeTestClock struct{ t atomic.Pointer[time.Time] }

func newRangeTestClock() *rangeTestClock {
	c := &rangeTestClock{}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.t.Store(&now)
	return c
}
func (c *rangeTestClock) now() time.Time { return *c.t.Load() }
func (c *rangeTestClock) advance(d time.Duration) {
	n := c.now().Add(d)
	c.t.Store(&n)
}

var testRange = vcsRangeKey{workDir: "/w", baseSHA: "base", headSHA: "head"}

func TestRangeStages_should_NotRecompute_When_RangeUnchanged(t *testing.T) {
	t.Parallel()
	var listCalls, diffCalls atomic.Int32
	s := &vcsRangeStages{
		now: newRangeTestClock().now,
		listShipped: func(context.Context, string, string, string) ([]git.ShippedCommit, bool, error) {
			listCalls.Add(1)
			return []git.ShippedCommit{{SHA: "head"}}, true, nil
		},
		diffStat: func(context.Context, string, string, string) (git.AggregateDiffStat, error) {
			diffCalls.Add(1)
			return git.AggregateDiffStat{FilesChanged: 2}, nil
		},
	}
	for i := 0; i < 3; i++ {
		res, err := s.shippedCommits(context.Background(), testRange)
		require.NoError(t, err)
		assert.True(t, res.truncated)
		ds, err := s.aggregateDiffStat(context.Background(), testRange)
		require.NoError(t, err)
		assert.Equal(t, 2, ds.FilesChanged)
	}
	assert.EqualValues(t, 1, listCalls.Load())
	assert.EqualValues(t, 1, diffCalls.Load())

	moved := testRange
	moved.headSHA = "head2"
	_, err := s.shippedCommits(context.Background(), moved)
	require.NoError(t, err)
	assert.EqualValues(t, 2, listCalls.Load(), "a moved HEAD is a new range")
}

func TestRangeStages_should_BoundStageAndRememberFailure_When_BudgetExceeded(t *testing.T) {
	clock := newRangeTestClock()
	var calls atomic.Int32
	s := &vcsRangeStages{
		now: clock.now,
		listShipped: func(ctx context.Context, _, _, _ string) ([]git.ShippedCommit, bool, error) {
			calls.Add(1)
			<-ctx.Done() // a walk that only stops when its budget ends
			return nil, false, ctx.Err()
		},
	}
	old := vcsShippedCommitsBudget
	vcsShippedCommitsBudget = time.Millisecond
	t.Cleanup(func() { vcsShippedCommitsBudget = old })

	_, err := s.shippedCommits(context.Background(), testRange)
	require.ErrorIs(t, err, errVCSStageBudget)
	_, err = s.shippedCommits(context.Background(), testRange)
	require.ErrorIs(t, err, errVCSStageBudget)
	assert.EqualValues(t, 1, calls.Load(), "the failure is remembered, not retried on every cache miss")

	clock.advance(vcsRangeFailureTTL + time.Second)
	_, err = s.shippedCommits(context.Background(), testRange)
	require.ErrorIs(t, err, errVCSStageBudget)
	assert.EqualValues(t, 2, calls.Load(), "retried once the failure TTL passed")
}

func TestRangeStages_should_NotCache_When_CallerCanceled(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	s := &vcsRangeStages{
		now: newRangeTestClock().now,
		listShipped: func(ctx context.Context, _, _, _ string) ([]git.ShippedCommit, bool, error) {
			calls.Add(1)
			<-ctx.Done()
			return nil, false, ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.shippedCommits(ctx, testRange)
	require.Error(t, err)
	_, err = s.shippedCommits(ctx, testRange)
	require.Error(t, err)
	assert.EqualValues(t, 2, calls.Load(), "a canceled caller says nothing about the range")
}

func TestRangeCache_should_EvictLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	c := newRangeCache[int](2, newRangeTestClock().now)
	k := func(h string) vcsRangeKey { return vcsRangeKey{headSHA: h} }
	c.store(k("a"), 1, time.Time{})
	c.store(k("b"), 2, time.Time{})
	_, r := c.lookup(k("a")) // a is now most recent
	assert.Equal(t, rangeCacheHit, r)
	c.store(k("c"), 3, time.Time{})
	_, r = c.lookup(k("b"))
	assert.Equal(t, rangeCacheMiss, r, "b was least recently used")
	_, r = c.lookup(k("a"))
	assert.Equal(t, rangeCacheHit, r)
}

func TestRangeCache_should_KeepCachedSuccess_When_SlowerComputationFailsLater(t *testing.T) {
	t.Parallel()
	clock := newRangeTestClock()
	c := newRangeCache[int](4, clock.now)
	c.store(testRange, 7, time.Time{})
	c.store(testRange, 0, clock.now().Add(time.Minute)) // a budget failure from an earlier-started run
	v, r := c.lookup(testRange)
	assert.Equal(t, rangeCacheHit, r)
	assert.Equal(t, 7, v)
}
