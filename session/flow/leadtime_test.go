package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeadTime_should_ReturnPercentilesAndExcludeItemsWithoutInProgress_When_ThreeMergedItems(t *testing.T) {
	t.Parallel()
	mergeAt := s12Base.Add(10 * 24 * time.Hour)
	spans := make([]Span, 0, 7)
	for i, days := range []int{1, 3, 5} {
		item := s12Item(i)
		spans = append(spans,
			s12Real(t, item, stReady, stProg, mergeAt.Add(-time.Duration(days)*24*time.Hour)),
			s12Real(t, item, stPRPend, stDoneStat, mergeAt))
	}
	spans = append(spans, s12Real(t, "no-start", stPRPend, stDoneStat, mergeAt))

	res := LeadTime(spans, DefaultConfig())

	day := 24 * time.Hour
	assert.Equal(t, Percentiles{N: 3, P50: 3 * day, P90: 5 * day, Max: 5 * day, LowSample: true}, res.Stats)
	assert.Equal(t, 1, res.Excluded)
	assert.Equal(t, []ItemID{"item-000", "item-001", "item-002"}, res.Drill[DrillLeadTime])
}

func TestLeadTime_should_UseFirstInProgressOnly_When_ItemReentersInProgress(t *testing.T) {
	t.Parallel()
	mergeAt := s12Base.Add(10 * 24 * time.Hour)
	spans := []Span{
		s12Real(t, "x", stReady, stProg, mergeAt.Add(-4*24*time.Hour)),
		s12Real(t, "x", stReview, stProg, mergeAt.Add(-2*24*time.Hour)),
		s12Real(t, "x", stPRPend, stDoneStat, mergeAt),
	}

	res := LeadTime(spans, DefaultConfig())

	assert.Equal(t, 4*24*time.Hour, res.Stats.Max)
}
