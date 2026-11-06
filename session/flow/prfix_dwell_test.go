package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrFixDwell_should_ReturnCombinedCycleDwellAndExcludeThrash_When_TwoCyclesAndOpenReopen(t *testing.T) {
	t.Parallel()
	t0 := s12Base
	asOf := t0.Add(10 * 24 * time.Hour)
	w := dwellWindow(t, t0.Add(-24*time.Hour), asOf)
	var seq int64
	thrashFlips := flipRun("thrashy", stPRPend, stProg, t0.Add(time.Hour), 30, time.Minute, &seq)
	spans := Collapse(thrashFlips, w, DefaultConfig())
	require.Empty(t, realSpans(spans))
	spans = append(spans,
		s12Real(t, "two", stPRPend, stProg, t0),
		s12Real(t, "two", stProg, stPRPend, t0.Add(6*time.Hour)),
		s12Real(t, "two", stPRPend, stProg, t0.Add(48*time.Hour)),
		s12Real(t, "two", stProg, stReview, t0.Add(51*time.Hour)),
	)

	res := PRFixDwell(spans, w, asOf, DefaultConfig())

	assert.Equal(t, Percentiles{N: 2, P50: 3 * time.Hour, P90: 6 * time.Hour, Max: 6 * time.Hour, LowSample: true}, res.Stats)
	require.Len(t, res.Cycles, 2, "thrash reopens are episodes, not cycles")
	assert.False(t, res.Cycles[0].Open)
	assert.Equal(t, PRFixTriggerUnknown, res.Cycles[0].Trigger)
	assert.Equal(t, []string{"pr_fix_dwell:combined", "pr_fix_cause:unavailable"}, res.Estimated)
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "requirement asks for CI-failure and merge-conflict dwell separately")
	assert.Contains(t, res.Notes[0], "(Story 5.2.3)")
	assert.Equal(t, []ItemID{"two"}, res.Drill[DrillPRFixDwell])
}

func TestPrFixDwell_should_EndOpenReopenAtAsOfAndFlagIt_When_NoExitEvent(t *testing.T) {
	t.Parallel()
	asOf := s12Base.Add(48 * time.Hour)
	w := dwellWindow(t, s12Base.Add(-time.Hour), asOf)
	spans := []Span{s12Real(t, "open", stPRPend, stProg, s12Base.Add(24*time.Hour))}

	res := PRFixDwell(spans, w, asOf, DefaultConfig())

	require.Len(t, res.Cycles, 1)
	assert.True(t, res.Cycles[0].Open)
	assert.Equal(t, 24*time.Hour, res.Cycles[0].Duration)
	assert.Equal(t, 1, res.OpenCount)
}

func TestPrFixDwell_should_EndCycleAtDone_When_ItemMergesDirectly(t *testing.T) {
	t.Parallel()
	asOf := s12Base.Add(48 * time.Hour)
	w := dwellWindow(t, s12Base.Add(-time.Hour), asOf)
	spans := []Span{
		s12Real(t, "d", stPRPend, stProg, s12Base),
		s12Real(t, "d", stPRPend, stDoneStat, s12Base.Add(2*time.Hour)),
	}

	res := PRFixDwell(spans, w, asOf, DefaultConfig())

	require.Len(t, res.Cycles, 1)
	assert.Equal(t, 2*time.Hour, res.Cycles[0].Duration)
}

func TestPrFixDwell_should_ReturnEmptyStatsAndStillLabelEstimated_When_NoReopens(t *testing.T) {
	t.Parallel()
	w := dwellWindow(t, s12Base, s12Base.Add(time.Hour))

	res := PRFixDwell(nil, w, s12Base.Add(time.Hour), DefaultConfig())

	assert.Empty(t, res.Cycles)
	assert.Equal(t, 0, res.Stats.N)
	assert.True(t, res.Stats.LowSample)
	assert.Contains(t, res.Estimated, "pr_fix_dwell:combined")
}
