package github

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nudgeTestPR(t *testing.T, number int, failing int) (UserPR, PRKey) {
	t.Helper()
	key, err := NewPRKey("", "acme", "api", number)
	require.NoError(t, err)
	pr := UserPR{Host: "github.com", Owner: "acme", Repo: "api", Number: number}
	for i := 0; i < failing; i++ {
		pr.FailingChecks = append(pr.FailingChecks, FailingCheck{Name: "lint"})
	}
	return pr, key
}

func TestRecordNudge_should_LogResolvedClosedOrExpired_When_LaterPollsClearReasonsDropPRsOrWait24h(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c := NewUserPRCache()
	failing, key42 := nudgeTestPR(t, 42, 1)
	_, key43 := nudgeTestPR(t, 43, 0)
	_, key44 := nudgeTestPR(t, 44, 0)
	reasons := []NudgeReasonKind{NudgeReasonFailingChecks}
	c.RecordNudge(key42, reasons, t0)
	c.RecordNudge(key43, reasons, t0)
	c.RecordNudge(key44, reasons, t0)
	pr43, _ := nudgeTestPR(t, 43, 1)
	pr44, _ := nudgeTestPR(t, 44, 1)

	// Still failing shortly after: nothing settles, 42 stays pending.
	require.Empty(t, c.nudges.evaluate([]UserPR{failing, pr43, pr44}, t0.Add(time.Minute)))

	// +40 min: 42 cleared (resolved), 43 gone from the open list (closed), 44 still failing.
	cleared, _ := nudgeTestPR(t, 42, 0)
	got := c.nudges.evaluate([]UserPR{cleared, pr44}, t0.Add(40*time.Minute))
	byKey := map[PRKey]NudgeFollowup{}
	for _, ev := range got {
		byKey[ev.Key] = ev
	}
	require.Len(t, got, 2)
	assert.Equal(t, NudgeFollowupResolved, byKey[key42].State)
	assert.EqualValues(t, 2400, int64(byKey[key42].ResolvedAfter/time.Second))
	assert.Equal(t, NudgeFollowupClosed, byKey[key43].State)

	// 24 h later 44 is still failing: expired, then the map is empty.
	got = c.nudges.evaluate([]UserPR{pr44}, t0.Add(24*time.Hour))
	require.Len(t, got, 1)
	assert.Equal(t, NudgeFollowupExpired, got[0].State)
	assert.Empty(t, c.nudges.evaluate([]UserPR{pr44}, t0.Add(25*time.Hour)))
}

func TestRecordNudge_should_KeepNewest256_When_MoreRecorded(t *testing.T) {
	c := NewUserPRCache()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var first PRKey
	for i := 1; i <= maxRecordedNudges+5; i++ {
		_, key := nudgeTestPR(t, i, 0)
		if i == 1 {
			first = key
		}
		c.RecordNudge(key, []NudgeReasonKind{NudgeReasonFailingChecks}, t0)
	}
	assert.Len(t, c.nudges.recorded, maxRecordedNudges)
	for _, r := range c.nudges.recorded {
		assert.NotEqual(t, first, r.key, "oldest entry must be dropped first")
	}
}

func TestFirstSeenAttention_should_StickUntilPRStopsNeedingAttention_When_PollsRepeat(t *testing.T) {
	c := NewUserPRCache()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	failing, key := nudgeTestPR(t, 42, 1)
	c.nudges.evaluate([]UserPR{failing}, t0)
	c.nudges.evaluate([]UserPR{failing}, t0.Add(time.Hour))
	at, ok := c.FirstSeenAttention(key)
	require.True(t, ok)
	assert.Equal(t, t0, at, "first sighting must not move on later polls")

	green, _ := nudgeTestPR(t, 42, 0)
	c.nudges.evaluate([]UserPR{green}, t0.Add(2*time.Hour))
	_, ok = c.FirstSeenAttention(key)
	assert.False(t, ok)
}
