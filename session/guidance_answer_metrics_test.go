package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

func TestObserveGuidanceAnswer_should_ReportMilliseconds_When_AnsweredSevenAndAHalfSecondsAfterCreation(t *testing.T) {
	created := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	answered := created.Add(7500 * time.Millisecond)
	row := &ent.GuidanceRequest{ID: uuid.New(), Source: string(domain.QuestionSourceAgentMCP), CreatedAt: created, AnsweredAt: &answered}

	before := GuidanceAnswerLatencyCount(domain.QuestionSourceAgentMCP, "")
	ms := observeGuidanceAnswer(context.Background(), row)

	assert.EqualValues(t, 7500, ms)
	assert.Equal(t, before+1, GuidanceAnswerLatencyCount(domain.QuestionSourceAgentMCP, ""))
}

func TestGuidanceRepository_should_ObserveLatencyAndDeliveryState_When_AnAnswerIsApplied(t *testing.T) {
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	row, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_metrics", "Which color?"), 0)
	require.NoError(t, err)
	id := uuid.MustParse(row.ID)

	latencyBefore := GuidanceAnswerLatencyCount(domain.QuestionSourceHook, "")
	applied, err := repo.AnswerGuidanceRequest(ctx, id, "Red")
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, latencyBefore+1, GuidanceAnswerLatencyCount(domain.QuestionSourceHook, ""))

	again, err := repo.AnswerGuidanceRequest(ctx, id, "Blue")
	require.NoError(t, err)
	assert.False(t, again)
	assert.Equal(t, latencyBefore+1, GuidanceAnswerLatencyCount(domain.QuestionSourceHook, ""), "a lost race observes nothing")

	deliveredBefore := GuidanceAnswerDeliveryTotal(domain.DeliveryRouteLiveKeystroke, domain.DeliveryStatePending)
	ok, err := repo.SetDelivery(ctx, id, domain.DeliveryRouteLiveKeystroke, domain.DeliveryStatePending, 0)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, deliveredBefore+1, GuidanceAnswerDeliveryTotal(domain.DeliveryRouteLiveKeystroke, domain.DeliveryStatePending))
}
