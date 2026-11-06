package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func funnelStage(t *testing.T, res FunnelResult, st domain.BacklogStatus) FunnelStage {
	t.Helper()
	for _, s := range res.Stages {
		if s.Status == st {
			return s
		}
	}
	require.Failf(t, "stage missing", "stage %s not in funnel", st)
	return FunnelStage{}
}

func TestFunnel_should_PlaceUnknownStatusInOtherBucket_When_CustomStageNotInStageOrder(t *testing.T) {
	t.Parallel()
	order := []domain.BacklogStatus{
		domain.BacklogStatusIdea, domain.BacklogStatusRefining, stReady, stProg, stReview, stPRPend, stDoneStat,
	}
	spans := []Span{
		s12Real(t, "a", stProg, "qa", s12Base),
		s12Real(t, "a", "qa", stReview, s12Base.Add(time.Hour)),
		s12Real(t, "b", domain.BacklogStatusIdea, domain.BacklogStatusRefining, s12Base),
		s12Real(t, "b", stDoneStat, domain.BacklogStatusArchived, s12Base.Add(time.Hour)),
	}

	res := Funnel(spans, order, DwellResult{}, DefaultConfig())

	assert.Equal(t, 1, res.Other, "qa entry lands in other with its count")
	assert.Equal(t, 1, funnelStage(t, res, domain.BacklogStatusRefining).Entered, "refining is a stage because it is in StageOrder")
	assert.Len(t, res.Stages, len(order))
	assert.Equal(t, 1, res.Archived, "archived counts toward throughput, not stages")
	for _, s := range res.Stages {
		assert.NotEqual(t, domain.BacklogStatusArchived, s.Status)
	}
}

func TestFunnel_should_CountBackEdgesAndAdvances_When_ItemsMoveBothWays(t *testing.T) {
	t.Parallel()
	order := []domain.BacklogStatus{stReady, stProg, stReview, stPRPend, stDoneStat}
	var spans []Span
	for i := 0; i < 3; i++ {
		item := s12Item(i)
		spans = append(spans,
			s12Real(t, item, stProg, stReview, s12Base),
			s12Real(t, item, stReview, stProg, s12Base.Add(time.Hour)),
			s12Real(t, item, stProg, stReview, s12Base.Add(2*time.Hour)),
			s12Real(t, item, stReview, stPRPend, s12Base.Add(3*time.Hour)))
	}

	res := Funnel(spans, order, DwellResult{}, DefaultConfig())

	review := funnelStage(t, res, stReview)
	assert.Equal(t, 3, review.Entered, "distinct items, not visits")
	assert.Equal(t, 3, review.BackEdges)
	assert.Equal(t, 3, review.Advanced)
	assert.Equal(t, 3, funnelStage(t, res, stProg).Advanced)
	assert.Contains(t, res.Drill, DrillKey("funnel.stage.review"))
}
