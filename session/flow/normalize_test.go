package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func rawFromFixture(f fixtureEvents) []RawEvent {
	out := make([]RawEvent, 0, len(f.Events))
	for _, e := range f.Events {
		out = append(out, RawEvent{
			ItemID: ItemID(e.ItemID), From: domain.BacklogStatus(e.From), To: domain.BacklogStatus(e.To),
			At: e.At, Note: e.Note, TriggeredBy: e.TriggeredBy, Seq: int64(e.Seq),
		})
	}
	return out
}

func TestNormalize_should_SortByUtcAndDropSelfLoops_When_MixedOffsetsAndTwoSelfLoops(t *testing.T) {
	t.Parallel()
	f := loadFixture[fixtureEvents](t, "events_thrash.json")

	events, dq := Normalize(rawFromFixture(f))

	assert.Equal(t, 2, dq.SelfLoops)
	assert.Equal(t, 0, dq.Unparseable)
	assert.Len(t, events, len(f.Events)-2)
	for _, e := range events {
		assert.Equal(t, time.UTC, e.At.Location())
		assert.NotEqual(t, e.From, e.To)
	}
	for i := 1; i < len(events); i++ {
		assert.False(t, events[i].At.Before(events[i-1].At), "event %d out of order", i)
	}
}

func TestNormalize_should_OrderByInstantNotWallClock_When_OffsetsDiffer(t *testing.T) {
	t.Parallel()
	raw := []RawEvent{
		{ItemID: "i", From: "ready", To: "in_progress", At: "2026-08-10 01:00:00.000000-07:00", Seq: 1},
		{ItemID: "i", From: "in_progress", To: "review", At: "2026-08-10 07:30:00.000000+00:00", Seq: 2},
	}

	events, dq := Normalize(raw)

	require.Len(t, events, 2)
	assert.Equal(t, int64(2), events[0].Seq, "07:30Z sorts before 08:00Z")
	assert.Equal(t, time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC), events[1].At)
	assert.Zero(t, dq.SelfLoops)
}

func TestNormalize_should_BreakTiesBySeqAndCountUnparseable_When_EqualInstantsOrBadTimes(t *testing.T) {
	t.Parallel()
	raw := []RawEvent{
		{ItemID: "i", From: "b", To: "c", At: "2026-10-01T10:00:00Z", Seq: 9},
		{ItemID: "i", From: "a", To: "b", At: "2026-10-01 10:00:00+00:00", Seq: 3},
		{ItemID: "i", From: "c", To: "d", At: "garbage", Seq: 1},
	}

	events, dq := Normalize(raw)

	require.Len(t, events, 2)
	assert.Equal(t, int64(3), events[0].Seq)
	assert.Equal(t, 1, dq.Unparseable)
}

func TestPrefilterFloor_should_BeSupersetOfPaddedWindow_When_OffsetsWithinFourteenHours(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	start := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	floor := PrefilterFloor(start, cfg)
	wallClock := func(at time.Time) time.Time { // stored text is local wall-clock; compare it as naive
		y, m, d := at.Date()
		h, mi, s := at.Clock()
		return time.Date(y, m, d, h, mi, s, 0, time.UTC)
	}

	for offsetHours := -14; offsetHours <= 14; offsetHours++ {
		loc := time.FixedZone("o", offsetHours*3600)
		edge := start.Add(-cfg.CollapsePad()) // earliest instant the collapse needs
		assert.False(t, wallClock(edge.In(loc)).Before(floor), "offset %dh would be dropped by the SQL prefilter", offsetHours)
	}
}
