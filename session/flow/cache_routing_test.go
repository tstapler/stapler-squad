package flow

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routingSrc() *fakeSrc {
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)
	return f
}

func TestCompare_should_HitCache_When_SameArgsRepeated(t *testing.T) {
	t.Parallel()
	for name, q := range map[string]CompareQuery{
		"offsets": cmpQ,
		"explicit": {
			AStart: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), AEnd: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
			BStart: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), BEnd: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := routingSrc()
			agg := NewAggregator(f.sources(), fixtureClock)
			first, err := agg.Compare(context.Background(), q)
			require.NoError(t, err)
			events := f.called("Events") + f.called("StreamEvents")
			stuck := f.called("StuckIntervals")
			require.Positive(t, events+stuck)

			second, err := agg.Compare(context.Background(), q)
			require.NoError(t, err)
			assert.Equal(t, events, f.called("Events")+f.called("StreamEvents"), "second compare must not read events")
			assert.Equal(t, stuck, f.called("StuckIntervals"))
			assert.Equal(t, first.Metrics, second.Metrics)
		})
	}
}

func TestCompare_should_ShareSummaryCache_When_WindowsOverlapAcrossCalls(t *testing.T) {
	t.Parallel()
	f := routingSrc()
	agg := NewAggregator(f.sources(), fixtureClock)
	_, err := agg.Compare(context.Background(), CompareQuery{ADays: 14, BDays: 14})
	require.NoError(t, err)
	before := f.called("Events") + f.called("StreamEvents")
	// Window B (the last 14 days) is already cached; only the new A window computes.
	_, err = agg.Compare(context.Background(), CompareQuery{ADays: 7, BDays: 14})
	require.NoError(t, err)
	grew := f.called("Events") + f.called("StreamEvents") - before
	assert.Equal(t, 1, grew, "only window A is new")
}

func TestDrilldown_should_HitCache_When_SameArgsRepeated(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	q := DrilldownQuery{Key: DrillLeadTime, Query: FlowQuery{Window: WindowSpec{Days: 30}}}
	first, err := agg.Drilldown(context.Background(), q)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, m := range []string{"Events", "StreamEvents", "Items", "Verdicts", "StuckIntervals", "Stages"} {
		counts[m] = f.called(m)
	}
	second, err := agg.Drilldown(context.Background(), q)
	require.NoError(t, err)
	for m, n := range counts {
		assert.Equal(t, n, f.called(m), "source %s must not be re-read", m)
	}
	assert.Equal(t, first.Items, second.Items)
}

func TestDrilldown_should_BuildOnlyNeededFamily_When_KeyIsStuck(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	_, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: DrillStuckReason("review_deadlock"), Query: FlowQuery{Window: WindowSpec{Days: 30}}})
	if err != nil {
		require.ErrorContains(t, err, "unknown drill key") // family built; key just absent in this fixture
	}
	assert.Positive(t, f.called("StuckIntervals"))
	assert.Zero(t, f.called("Verdicts"), "review family must not run")
	assert.Zero(t, f.called("Stages"), "funnel family must not run")
	assert.Zero(t, f.called("Events")+f.called("StreamEvents"), "events family must not run")
}

// The review family reads stuck intervals itself (deadlock sets), so only the funnel family is asserted absent.
func TestDrilldown_should_BuildOnlyReviewFamily_When_KeyIsReview(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	_, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: DrillVerdicts, Query: FlowQuery{Window: WindowSpec{Days: 30}}})
	require.NoError(t, err)
	assert.Positive(t, f.called("Verdicts"))
	assert.Zero(t, f.called("Stages"), "funnel family must not run")
}

func TestDrilldown_should_CoalesceToOneCompute_When_ConcurrentSameArgs(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	q := DrilldownQuery{Key: DrillVerdicts, Query: FlowQuery{Window: WindowSpec{Days: 30}}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := agg.Drilldown(context.Background(), q)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, f.called("Verdicts"), "concurrent drills share one compute")
}

func TestAggregator_should_ShareOneSlot_When_ComputesDifferInKind(t *testing.T) {
	t.Parallel()
	agg := NewAggregator(fixtureSrc(t).sources(), fixtureClock)
	assert.True(t, agg.cache.slot == agg.drillCache.slot, "drill cache must share the summary slot")
	assert.True(t, agg.cache.slot == agg.timelineCache.slot, "timeline cache must share the summary slot")
}
