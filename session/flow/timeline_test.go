package flow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestItemTimeline_should_CollapseThrashIntoTwoEpisodes_When_ThrashAFixture(t *testing.T) {
	t.Parallel()
	agg := NewAggregator(fixtureSrc(t).sources(), fixtureClock)
	tl, err := agg.ItemTimeline(context.Background(), ItemTimelineQuery{ItemID: "thrash-a"})
	require.NoError(t, err)

	assert.LessOrEqual(t, len(tl.Entries), 50)
	assert.Equal(t, 2, tl.ThrashEpisodes)
	counts := make([]int, 0, len(tl.Entries))
	for _, e := range tl.Entries {
		assert.Equal(t, "thrash_episode", e.Kind, "no raw flip events remain")
		counts = append(counts, e.Count)
	}
	assert.Equal(t, []int{217, 62}, counts, "279 flips as 2 episodes")
	assert.NotEmpty(t, tl.Title)
	assert.False(t, tl.Truncated)
}

func TestItemTimeline_should_KeepNewest50AndReportTotals_When_ItemHasManyTransitions(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f.items = []ItemMeta{{ID: "busy", Title: "busy item", Status: domain.BacklogStatusReview}}
	for i := 0; i < 120; i++ {
		from, to := domain.BacklogStatusInProgress, domain.BacklogStatusReview
		if i%2 == 1 {
			from, to = to, from
		}
		f.events = append(f.events, Event{ItemID: "busy", From: from, To: to, At: start.Add(time.Duration(i) * time.Hour), Seq: int64(i + 1), Note: fmt.Sprintf("n%d", i)})
	}
	tl, err := NewAggregator(f.sources(), fixtureClock).ItemTimeline(context.Background(), ItemTimelineQuery{ItemID: "busy"})
	require.NoError(t, err)

	assert.Len(t, tl.Entries, 50)
	assert.Equal(t, 120, tl.TotalEntries)
	assert.True(t, tl.Truncated)
	assert.Equal(t, "n119", tl.Entries[49].Note, "the newest entry is last")
	assert.Equal(t, "n70", tl.Entries[0].Note)
	assert.Equal(t, "review", tl.Status)
}

func TestItemTimeline_should_RejectEmptyItemID_When_QueryHasNone(t *testing.T) {
	t.Parallel()
	_, err := NewAggregator(newFakeSrc().sources(), fixtureClock).ItemTimeline(context.Background(), ItemTimelineQuery{})
	require.Error(t, err)
}
