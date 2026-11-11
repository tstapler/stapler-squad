package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func newWaiverItem(t *testing.T, repo *session.EntRepository) string {
	t.Helper()
	ac, err := session.SerializeAcCriteria([]session.AcCriterion{
		{Index: 0, Text: "needs macOS proof", Status: session.AcStatusPending},
		{Index: 1, Text: "other", Status: session.AcStatusDone},
	})
	require.NoError(t, err)
	item, err := repo.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:              "waiver item",
		Status:             string(session.BacklogStatusInProgress),
		AcceptanceCriteria: ac,
	})
	require.NoError(t, err)
	return item.ID
}

func loadCriteria(t *testing.T, repo *session.EntRepository, id string) []session.AcCriterion {
	t.Helper()
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	got, err := session.ParseAcCriteria(item.AcceptanceCriteria)
	require.NoError(t, err)
	return got
}

func TestWaiveAcCriterion_should_RecordWaivedStatusAndReason(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	require.NoError(t, repo.WaiveAcCriterion(context.Background(), id, 0, "  macOS waived by owner  "))

	got := loadCriteria(t, repo, id)
	assert.Equal(t, session.AcStatusWaived, got[0].Status)
	assert.Equal(t, "macOS waived by owner", got[0].WaiverReason)
	assert.True(t, got[0].Status.IsSatisfied(), "waived must satisfy the request_review gate")
	assert.Equal(t, session.AcStatusDone, got[1].Status, "other criteria untouched")
}

func TestWaiveAcCriterion_should_Reject_When_ReasonBlankOrIndexOutOfRange(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	assert.ErrorIs(t, repo.WaiveAcCriterion(context.Background(), id, 0, "   "), session.ErrWaiverReasonRequired)
	assert.ErrorIs(t, repo.WaiveAcCriterion(context.Background(), id, 9, "why"), session.ErrAcCriterionOutOfRange)
	assert.Equal(t, session.AcStatusPending, loadCriteria(t, repo, id)[0].Status)
}

func TestUpdateAcCriterionStatus_should_NotOverwrite_When_CriterionWaived(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	require.NoError(t, repo.WaiveAcCriterion(context.Background(), id, 0, "owner call"))

	err := repo.UpdateAcCriterionStatus(context.Background(), id, 0, string(session.AcStatusFail), "")

	assert.ErrorIs(t, err, session.ErrAcCriterionWaived)
	assert.Equal(t, session.AcStatusWaived, loadCriteria(t, repo, id)[0].Status)
}

func TestWaiveAcCriterion_should_ReturnErrNotFound_When_ItemMissing(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)

	err := repo.WaiveAcCriterion(context.Background(), "bl_01HXYZABCDEFGHJKMNPQRSTVWX", 0, "why")

	assert.ErrorIs(t, err, domain.ErrNotFound)
}
