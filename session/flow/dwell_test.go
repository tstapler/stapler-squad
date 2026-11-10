package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dwellWindow(t *testing.T, start, end time.Time) FlowWindow {
	t.Helper()
	w, err := NewFlowWindowBounds(start, end)
	require.NoError(t, err)
	return w
}

func TestDwell_should_ClipToWindowAndSeparateThrashBucket_When_ItemSpansWindowStart(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	thrashFirst := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	thrashLast := thrashFirst.Add(30 * time.Minute)
	seed := Event{ItemID: "i", From: stReady, To: stProg, At: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}

	res := Dwell(DwellInput{
		Spans:  []Span{s12Thrash(t, "i", FlipPair{A: stReady, B: stProg}, thrashFirst, thrashLast, stProg)},
		Seeds:  []Event{seed},
		Window: dwellWindow(t, start, end),
		AsOf:   end,
	})

	prog := res.Stages[stProg]
	assert.Equal(t, []time.Duration{thrashFirst.Sub(start), end.Sub(thrashLast)}, prog.Samples, "dwell starts at window start, thrash cut out")
	assert.Equal(t, 1, prog.Open, "trailing interval is open")
	assert.Equal(t, []time.Duration{30 * time.Minute}, res.Thrash.Samples)
	assert.Empty(t, res.Stages[stReady].Samples, "thrash time never lands in ready")
	assert.Zero(t, res.Quality.NegativeGaps)
}

func TestDwell_should_CountNegativeGapsAndUseAsOfForOpenInterval_When_ClockSkewOrOpenItem(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	w := dwellWindow(t, start, asOf)

	t.Run("open interval ends at as-of and is flagged", func(t *testing.T) {
		t.Parallel()
		enter := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
		res := Dwell(DwellInput{
			Spans:  []Span{s12Real(t, "r", stProg, stReview, enter)},
			Window: w, AsOf: asOf,
		})
		assert.Equal(t, []time.Duration{3 * time.Hour}, res.Stages[stReview].Samples)
		assert.Equal(t, 1, res.Stages[stReview].Open)
	})

	t.Run("a span after as-of clamps to zero and is counted", func(t *testing.T) {
		t.Parallel()
		skewed := time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
		res := Dwell(DwellInput{
			Spans:  []Span{s12Real(t, "k", stProg, stReview, skewed)},
			Window: w, AsOf: asOf,
		})
		assert.Equal(t, []time.Duration{0}, res.Stages[stReview].Samples)
		assert.Equal(t, 1, res.Quality.NegativeGaps)
	})

	t.Run("terminal statuses are not dwell stages", func(t *testing.T) {
		t.Parallel()
		res := Dwell(DwellInput{
			Spans:  []Span{s12Real(t, "d", stPRPend, stDoneStat, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))},
			Window: w, AsOf: asOf,
		})
		assert.Empty(t, res.Stages[stDoneStat].Samples)
	})
}

func TestDwell_should_RecordNoZeroSample_When_SeededStageIsCutByPreWindowThrash(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	first := start.Add(-10 * time.Minute) // the episode began before the window
	last := start.Add(20 * time.Minute)
	ts, err := NewThrashSpan(DefaultConfig(), ThrashSpanParams{
		Item: "i", Pair: FlipPair{A: stReady, B: stProg}, Count: 6, First: first, Last: last,
		NetStatus: stProg, Cause: CausePRFixReopen, StartsBeforeWin: true,
	})
	require.NoError(t, err)
	seed := Event{ItemID: "i", From: stReady, To: stProg, At: start.Add(-time.Hour)}

	res := Dwell(DwellInput{Spans: []Span{ts}, Seeds: []Event{seed}, Window: dwellWindow(t, start, end), AsOf: end})

	assert.Equal(t, []time.Duration{end.Sub(last)}, res.Stages[stProg].Samples, "only the post-thrash interval counts; no phantom 0s sample")
	assert.Equal(t, []time.Duration{last.Sub(start)}, res.Thrash.Samples)
}
