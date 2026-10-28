package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// countingStore counts GetAll calls, i.e. real aggregate computations.
type countingStore struct {
	fakeTokenStore
	getAllCalls atomic.Int32
}

func (c *countingStore) GetAll() []*tokens.ParseResult {
	c.getAllCalls.Add(1)
	return c.results
}

func newCachedFixture(t *testing.T) (*InsightsService, *countingStore, *fakeClock) {
	t.Helper()
	store := &countingStore{fakeTokenStore: fakeTokenStore{results: []*tokens.ParseResult{
		newResult("u1", "claude-sonnet-4-5", "/proj", 1000, 500, 0, time.Now().UTC()),
	}}}
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil, nil)
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)} // pi_extension_health_test.go
	svc.summaries.now = clk.Now
	return svc, store, clk
}

func summaryReq(orphans bool) *connect.Request[sessionv1.GetInsightsSummaryRequest] {
	return connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: orphans})
}

func notLoading() bool { return false }

func TestGetInsightsSummary_WhenCalledWithinTTL_ExpectCacheHit(t *testing.T) {
	t.Parallel()
	svc, store, clk := newCachedFixture(t)

	first, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	clk.Advance(insightsSummaryTTL - time.Millisecond)
	second, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)

	assert.EqualValues(t, 1, store.getAllCalls.Load())
	assert.Same(t, first.Msg, second.Msg)
}

func TestGetInsightsSummary_WhenRequestDiffers_ExpectSeparateComputation(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCachedFixture(t)

	_, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	_, err = svc.GetInsightsSummary(context.Background(), summaryReq(false))
	require.NoError(t, err)

	assert.EqualValues(t, 2, store.getAllCalls.Load())
}

func TestGetInsightsSummary_WhenTTLExpired_ExpectRecompute(t *testing.T) {
	t.Parallel()
	svc, store, clk := newCachedFixture(t)

	_, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	clk.Advance(insightsSummaryTTL)
	_, err = svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)

	assert.EqualValues(t, 2, store.getAllCalls.Load())
}

func TestGetInsightsSummary_WhenStoreStillLoading_ExpectNotCached(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCachedFixture(t)
	store.isLoading = true

	_, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	_, err = svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)

	assert.EqualValues(t, 2, store.getAllCalls.Load())
}

func TestSummaryCache_WhenInvalidated_ExpectRecompute(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCachedFixture(t)

	_, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	svc.summaries.invalidate()
	_, err = svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)

	assert.EqualValues(t, 2, store.getAllCalls.Load())
}

func TestSummaryCache_WhenInvalidatedDuringCompute_ExpectResultNotStored(t *testing.T) {
	t.Parallel()
	var c summaryCache
	req := &sessionv1.GetInsightsSummaryRequest{}
	var computes int
	compute := func(context.Context) (*sessionv1.GetInsightsSummaryResponse, error) {
		computes++
		if computes == 1 {
			c.invalidate() // e.g. DismissFinding lands mid-aggregate
		}
		return &sessionv1.GetInsightsSummaryResponse{}, nil
	}

	_, err := c.get(context.Background(), req, notLoading, compute)
	require.NoError(t, err)
	_, err = c.get(context.Background(), req, notLoading, compute)
	require.NoError(t, err)

	assert.Equal(t, 2, computes, "result computed before invalidation must not be cached")
}

func TestSummaryCache_WhenConcurrentIdenticalCallers_ExpectSingleCompute(t *testing.T) {
	t.Parallel()
	var c summaryCache
	req := &sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}
	const callers = 8
	var computes atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, callers)
	compute := func(context.Context) (*sessionv1.GetInsightsSummaryResponse, error) {
		computes.Add(1)
		started <- struct{}{}
		<-release
		return &sessionv1.GetInsightsSummaryResponse{TotalCostUsd: 1}, nil
	}

	var wg sync.WaitGroup
	results := make([]*sessionv1.GetInsightsSummaryResponse, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.get(context.Background(), req, notLoading, compute)
			assert.NoError(t, err)
			results[i] = r
		}()
	}
	<-started // leader is inside compute; the rest join its flight or hit the cache
	close(release)
	wg.Wait()

	assert.EqualValues(t, 1, computes.Load())
	for _, r := range results {
		assert.Same(t, results[0], r)
	}
}

func TestSummaryCache_WhenLeaderContextCanceled_ExpectFollowerStillServed(t *testing.T) {
	t.Parallel()
	var c summaryCache
	req := &sessionv1.GetInsightsSummaryRequest{}
	release := make(chan struct{})
	entered := make(chan struct{})
	compute := func(ctx context.Context) (*sessionv1.GetInsightsSummaryResponse, error) {
		close(entered)
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return &sessionv1.GetInsightsSummaryResponse{}, nil
	}

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := c.get(leaderCtx, req, notLoading, compute)
		leaderErr <- err
	}()
	<-entered
	follower := make(chan error, 1)
	go func() {
		_, err := c.get(context.Background(), req, notLoading, compute)
		follower <- err
	}()
	cancel()
	require.ErrorIs(t, <-leaderErr, context.Canceled)
	close(release)
	require.NoError(t, <-follower)
}

func TestSummaryCache_WhenComputeFails_ExpectErrorNotCached(t *testing.T) {
	t.Parallel()
	var c summaryCache
	req := &sessionv1.GetInsightsSummaryRequest{}
	boom := errors.New("boom")
	fail := func(context.Context) (*sessionv1.GetInsightsSummaryResponse, error) { return nil, boom }
	ok := func(context.Context) (*sessionv1.GetInsightsSummaryResponse, error) {
		return &sessionv1.GetInsightsSummaryResponse{}, nil
	}

	_, err := c.get(context.Background(), req, notLoading, fail)
	require.ErrorIs(t, err, boom)
	_, err = c.get(context.Background(), req, notLoading, ok)
	require.NoError(t, err)
}

func TestDismissFinding_WhenSummaryCached_ExpectNextSummaryRecomputed(t *testing.T) {
	t.Parallel()
	svc, store, _ := newCachedFixture(t)
	svc.SetDismissedFindingsStore(&fakeDismissedFindingsStore{})

	_, err := svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)
	_, err = svc.DismissFinding(context.Background(), connect.NewRequest(&sessionv1.DismissFindingRequest{FindingId: "f1"}))
	require.NoError(t, err)
	_, err = svc.GetInsightsSummary(context.Background(), summaryReq(true))
	require.NoError(t, err)

	assert.EqualValues(t, 2, store.getAllCalls.Load())
}
