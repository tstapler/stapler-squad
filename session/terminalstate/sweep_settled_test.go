package terminalstate

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSettledTracker_should_SkipUntilDeadline_When_Settled(t *testing.T) {
	t.Parallel()
	var s SettledTracker
	now := time.Unix(1_000, 0)

	assert.False(t, s.IsSettled("k", now), "the zero value tracks nothing")
	s.Settle("k", now, time.Minute)

	assert.True(t, s.IsSettled("k", now.Add(59*time.Second)))
	assert.False(t, s.IsSettled("k", now.Add(time.Minute)), "the deadline itself is no longer settled")
	assert.False(t, s.IsSettled("other", now))
}

func TestSettledTracker_should_DropOnlyExpiredKeys_When_AtCapacity(t *testing.T) {
	t.Parallel()
	var s SettledTracker
	now := time.Unix(1_000, 0)
	for i := 0; i < terminalSweepSettledMax-1; i++ {
		s.Settle(fmt.Sprintf("old-%d", i), now, time.Second)
	}
	s.Settle("live", now, time.Hour)

	later := now.Add(time.Minute)
	s.Settle("fresh", later, time.Minute)

	assert.True(t, s.IsSettled("live", later), "an unexpired key survives the eviction")
	assert.True(t, s.IsSettled("fresh", later))
	assert.Len(t, s.until, 2, "every expired key is evicted")
}

func TestSettledTracker_should_ClearEverything_When_AtCapacityAndNothingExpired(t *testing.T) {
	t.Parallel()
	var s SettledTracker
	now := time.Unix(1_000, 0)
	for i := 0; i < terminalSweepSettledMax; i++ {
		s.Settle(fmt.Sprintf("live-%d", i), now, time.Hour)
	}

	s.Settle("fresh", now, time.Hour)

	assert.True(t, s.IsSettled("fresh", now))
	assert.False(t, s.IsSettled("live-0", now), "the tracker is bounded: a full table of live keys is reset")
	assert.Len(t, s.until, 1)
}
