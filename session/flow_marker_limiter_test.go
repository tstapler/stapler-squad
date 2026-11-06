package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeMarkerClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeMarkerClock() *fakeMarkerClock {
	return &fakeMarkerClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeMarkerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeMarkerClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// The interval is inclusive: a repeat exactly 30m later is allowed, so the
// closed hour [t0, t0+60m] admits t0, t0+30m and t0+60m.
func TestMarkerLimiter_should_StoreAtMostThreeRecords_When_10000SameDecisionsInOneHour(t *testing.T) {
	t.Parallel()
	clk := newFakeMarkerClock()
	l := NewMarkerLimiter(clk.Now)
	// 10,001 decisions at 360ms land exactly on t0, t0+30m and t0+60m.
	const n = 10001
	const step = 360 * time.Millisecond
	allowed := 0
	for i := 0; i < n; i++ {
		if l.Allow("pr_fix_trigger|item|7", "ci") {
			allowed++
		}
		clk.Advance(step)
	}
	assert.Equal(t, 3, allowed)
	assert.Zero(t, l.Dropped(), "dedupe is not a cap drop")
}

func TestMarkerLimiter_should_DedupeJustUnderIntervalAndAllowAtInterval(t *testing.T) {
	t.Parallel()
	clk := newFakeMarkerClock()
	l := NewMarkerLimiter(clk.Now)
	assert.True(t, l.Allow("k", "ci"))
	clk.Advance(TriggerRecordMinInterval - time.Nanosecond)
	assert.False(t, l.Allow("k", "ci"))
	clk.Advance(time.Nanosecond)
	assert.True(t, l.Allow("k", "ci"))
}

func TestMarkerLimiter_should_AllowChangedValueImmediately_When_ValueDiffers(t *testing.T) {
	t.Parallel()
	l := NewMarkerLimiter(newFakeMarkerClock().Now)
	assert.True(t, l.Allow("k", "ci"))
	assert.True(t, l.Allow("k", "conflict"))
	assert.False(t, l.Allow("k", "conflict"))
}

func TestMarkerLimiter_should_DropAndCountOverGlobalCap_When_ManyKeys(t *testing.T) {
	t.Parallel()
	clk := newFakeMarkerClock()
	l := NewMarkerLimiter(clk.Now)
	for i := 0; i < MarkerMaxWritesPerHour; i++ {
		assert.True(t, l.Allow("k"+string(rune('a'+i%26))+time.Duration(i).String(), "v"))
	}
	assert.False(t, l.Allow("overflow", "v"))
	assert.False(t, l.Allow("overflow2", "v"))
	assert.Equal(t, 2, l.Dropped())
	// A changed value does not bypass the global cap.
	assert.False(t, l.Allow("k"+string(rune('a'))+time.Duration(0).String(), "changed"))
	// Next window resets the cap.
	clk.Advance(time.Hour)
	assert.True(t, l.Allow("overflow", "v"))
}

func TestMarkerLimiter_should_SweepStaleEntries_When_MapGrowsPastThreshold(t *testing.T) {
	t.Parallel()
	clk := newFakeMarkerClock()
	l := NewMarkerLimiter(clk.Now)
	for i := 0; i < markerSweepAbove+1; i++ {
		if i%MarkerMaxWritesPerHour == 0 {
			clk.Advance(time.Hour) // fresh cap window
		}
		l.Allow(time.Duration(i).String(), "v")
	}
	clk.Advance(TriggerRecordMinInterval)
	l.Allow("trigger-sweep", "v")
	l.Allow("trigger-sweep-2", "v")
	l.mu.Lock()
	size := len(l.last)
	l.mu.Unlock()
	assert.LessOrEqual(t, size, markerSweepAbove)
}

func TestPrFixTrigger_should_ClassifyTotalTable_When_FlagCombinations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		ci, conflict, rev, fb bool
		want                  string
	}{
		{"ci", true, false, false, false, PRFixTriggerCI},
		{"conflict", false, true, false, false, PRFixTriggerConflict},
		{"mixed", true, true, false, false, PRFixTriggerMixed},
		{"review", false, false, true, false, PRFixTriggerReview},
		{"feedback", false, false, false, true, PRFixTriggerFeedback},
		{"none", false, false, false, false, PRFixTriggerUnknown},
		{"ci+review", true, false, true, false, PRFixTriggerMixed},
		{"ci+feedback", true, false, false, true, PRFixTriggerMixed},
		{"conflict+review", false, true, true, false, PRFixTriggerMixed},
		{"review+feedback", false, false, true, true, PRFixTriggerMixed},
		{"ci+conflict+review", true, true, true, false, PRFixTriggerMixed},
		{"all", true, true, true, true, PRFixTriggerMixed},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, ClassifyPRFixTrigger(tc.ci, tc.conflict, tc.rev, tc.fb), tc.name)
	}
	// Exhaustive: every one of 16 combinations yields a known value.
	for m := 0; m < 16; m++ {
		got := ClassifyPRFixTrigger(m&1 != 0, m&2 != 0, m&4 != 0, m&8 != 0)
		assert.Contains(t, []string{PRFixTriggerCI, PRFixTriggerConflict, PRFixTriggerMixed,
			PRFixTriggerReview, PRFixTriggerFeedback, PRFixTriggerUnknown}, got)
	}
}
