package flow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingSrc overrides CountEvents with a fixed answer.
type countingSrc struct {
	*fakeSrc
	rows int
}

func (c countingSrc) CountEvents(context.Context, EventRange) (int, error) {
	c.record("CountEvents")
	return c.rows, nil
}

func TestAggregator_should_ChooseReadShapeByRowCeiling_When_10kAnd258kAnd1200kRows(t *testing.T) {
	t.Parallel()
	limits := ReaderLimits{MaxWindowRows: 60_000, HardRowCeiling: 1_000_000, ChunkItems: 25}
	cases := []struct {
		name       string
		rows       int
		wantEvents int
		wantStream int
		wantErr    bool
		wantSlow   bool
	}{
		{"30-day 10138 rows reads in one call", 10_138, 1, 0, false, false},
		{"all-time 258k rows streams", 258_000, 0, 1, false, true},
		{"1.2M rows is refused", 1_200_000, 0, 0, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := fixtureSrc(t)
			src := f.sources()
			src.Events = countingSrc{fakeSrc: f, rows: tc.rows}
			agg := NewAggregator(src, fixtureClock, WithReaderLimits(limits))
			s, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}})
			if tc.wantErr {
				var tooLarge *WindowTooLargeError
				require.ErrorIs(t, err, ErrWindowTooLarge)
				require.ErrorAs(t, err, &tooLarge)
				assert.Equal(t, tc.rows, tooLarge.Rows)
				assert.Equal(t, 1_000_000, tooLarge.Limit)
				assert.Zero(t, f.called("Events")+f.called("StreamEvents"), "a refused window reads nothing")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantEvents, f.called("Events"))
			assert.Equal(t, tc.wantStream, f.called("StreamEvents"))
			assert.Equal(t, tc.wantSlow, s.Meta.SlowPath)
		})
	}
}

func summarizeChunked(t *testing.T, chunk int) *FlowSummary {
	t.Helper()
	agg := NewAggregator(fixtureSrc(t).sources(), fixtureClock, WithReaderLimits(ReaderLimits{MaxWindowRows: 1, ChunkItems: chunk}))
	s, err := agg.Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}})
	require.NoError(t, err)
	return s
}

func TestSummary_should_EqualUnchunked_When_ChunkItemsOneAndOneThousand(t *testing.T) {
	t.Parallel()
	one, thousand := summarizeChunked(t, 1), summarizeChunked(t, 1000)
	assert.True(t, one.Meta.SlowPath, "forcing MaxWindowRows=1 must take the streamed path")
	assert.Equal(t, thousand, one)

	unchunked := summarize30(t, fixtureSrc(t))
	unchunked.Meta.SlowPath = true
	assert.Equal(t, unchunked, one, "chunked must equal the single-read summary")
}

// gatedSrc blocks CountEvents until release closes, tracking concurrency.
type gatedSrc struct {
	*fakeSrc
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	maxSeen int
	starts  int
}

func (g *gatedSrc) CountEvents(ctx context.Context, rng EventRange) (int, error) {
	g.mu.Lock()
	g.active++
	g.starts++
	g.maxSeen = max(g.maxSeen, g.active)
	g.mu.Unlock()
	g.entered <- struct{}{}
	<-g.release
	g.mu.Lock()
	g.active--
	g.mu.Unlock()
	return g.fakeSrc.CountEvents(ctx, rng)
}

func TestAggregator_should_RunOneUncachedComputeAtATime_When_TwoAllTimeRequestsWithDifferentFilters(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	g := &gatedSrc{fakeSrc: f, entered: make(chan struct{}, 4), release: make(chan struct{})}
	src := f.sources()
	src.Events = g
	agg := NewAggregator(src, fixtureClock)
	allTime := WindowSpec{AllTime: true}

	first := make(chan error, 1)
	go func() {
		_, err := agg.Summary(context.Background(), FlowQuery{Window: allTime, ItemIDs: []ItemID{"thrash-a"}})
		first <- err
	}()
	<-g.entered // the first compute holds the slot, parked inside CountEvents

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := agg.Summary(ctx, FlowQuery{Window: allTime, ItemIDs: []ItemID{"thrash-b"}})
	require.ErrorIs(t, err, ErrBusy, "the second request cannot take the slot while the first runs")

	g.mu.Lock()
	assert.Equal(t, 1, g.starts, "the second compute never reached the source")
	assert.Equal(t, 1, g.maxSeen)
	g.mu.Unlock()

	close(g.release)
	require.NoError(t, <-first)

	_, err = agg.Summary(context.Background(), FlowQuery{Window: allTime, ItemIDs: []ItemID{"thrash-b"}})
	require.NoError(t, err, "the slot is free once the first compute ends")
	assert.False(t, errors.Is(err, ErrBusy))
}
