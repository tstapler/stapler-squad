package backlogsvc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func verdictsJSON(t *testing.T, vs ...session.CriterionVerdict) string {
	t.Helper()
	b, err := json.Marshal(vs)
	require.NoError(t, err)
	return string(b)
}

func TestFlaggedForOwner_should_SkipCriteriaAlreadyDoneOrWaived(t *testing.T) {
	t.Parallel()
	criteria := []session.AcCriterion{
		{Index: 0, Status: session.AcStatusWaived},
		{Index: 1, Status: session.AcStatusPending},
		{Index: 2, Status: session.AcStatusPending},
	}
	raw := verdictsJSON(t,
		session.CriterionVerdict{CriterionIndex: 0, Outcome: session.ReviewVerdictFail, FailureClass: domain.FailureClassUnattainable},
		session.CriterionVerdict{CriterionIndex: 1, Outcome: session.ReviewVerdictFail, FailureClass: domain.FailureClassEnvironmental},
		session.CriterionVerdict{CriterionIndex: 2, Outcome: session.ReviewVerdictFail},
	)
	got := flaggedForOwner(raw, criteria)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].CriterionIndex)
}

func TestEscalateVerdictToOwner_should_AskOnce_And_HonourTheAnswer(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()
	raw, err := session.SerializeAcCriteria([]session.AcCriterion{{Index: 0, Text: "c", Status: session.AcStatusPending}})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "t", Status: string(session.BacklogStatusReview), AcceptanceCriteria: raw})
	require.NoError(t, err)
	flagged := []session.CriterionVerdict{{CriterionIndex: 0, Outcome: session.ReviewVerdictFail, FailureClass: domain.FailureClassUnattainable, Evidence: "cannot reproduce"}}

	require.True(t, svc.escalateVerdictToOwner(ctx, item, flagged), "first call parks and raises a request")
	require.True(t, svc.escalateVerdictToOwner(ctx, item, flagged), "a pending request keeps it parked")
	rows, _, _, err := storage.ListGuidanceRequestsForScope(ctx, domain.RequestScopeBacklogItem, item.ID, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the sweep must not raise a duplicate request")
	idx, isWaiver := domain.ParseOwnerActionWaiver(rows[0].QuestionText)
	assert.True(t, isWaiver)
	assert.Equal(t, 0, idx)
	assert.Contains(t, rows[0].Options, domain.OwnerActionWaiveResponse)

	_, err = storage.AnswerGuidanceRequest(ctx, mustParseUUID(t, rows[0].ID), ownerChoiceFixed)
	require.NoError(t, err)
	assert.False(t, svc.escalateVerdictToOwner(ctx, item, flagged), "owner fixed it: rework may proceed")
}

func mustParseUUID(t *testing.T, id string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(id)
	require.NoError(t, err)
	return parsed
}
