package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func newWaiverItem(t *testing.T, repo *session.EntRepository) string {
	t.Helper()
	ac, err := domain.SerializeAcCriteria([]domain.AcCriterion{
		{Index: 0, Text: "needs macOS proof", Status: domain.AcStatusPending},
		{Index: 1, Text: "other", Status: domain.AcStatusDone},
	})
	require.NoError(t, err)
	item, err := repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
		Title:              "waiver item",
		Status:             string(domain.BacklogStatusInProgress),
		AcceptanceCriteria: ac,
	})
	require.NoError(t, err)
	return item.ID
}

func loadCriteria(t *testing.T, repo *session.EntRepository, id string) []domain.AcCriterion {
	t.Helper()
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	got, err := domain.ParseAcCriteria(item.AcceptanceCriteria)
	require.NoError(t, err)
	return got
}

func TestWaiveAcCriterion_should_RecordWaivedStatusAndReason(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	require.NoError(t, repo.WaiveAcCriterion(context.Background(), id, 0, "  macOS waived by owner  "))

	got := loadCriteria(t, repo, id)
	assert.Equal(t, domain.AcStatusWaived, got[0].Status)
	assert.Equal(t, "macOS waived by owner", got[0].WaiverReason)
	assert.True(t, got[0].Status.IsSatisfied(), "waived must satisfy the request_review gate")
	assert.Equal(t, domain.AcStatusDone, got[1].Status, "other criteria untouched")
}

func TestWaiveAcCriterion_should_Reject_When_ReasonBlankOrIndexOutOfRange(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	assert.ErrorIs(t, repo.WaiveAcCriterion(context.Background(), id, 0, "   "), backlogdto.ErrWaiverReasonRequired)
	assert.ErrorIs(t, repo.WaiveAcCriterion(context.Background(), id, 9, "why"), backlogdto.ErrAcCriterionOutOfRange)
	assert.Equal(t, domain.AcStatusPending, loadCriteria(t, repo, id)[0].Status)
}

func TestUpdateAcCriterionStatus_should_NotOverwrite_When_CriterionWaived(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	require.NoError(t, repo.WaiveAcCriterion(context.Background(), id, 0, "owner call"))

	err := repo.UpdateAcCriterionStatus(context.Background(), id, 0, string(domain.AcStatusFail), "")

	assert.ErrorIs(t, err, backlogdto.ErrAcCriterionWaived)
	assert.Equal(t, domain.AcStatusWaived, loadCriteria(t, repo, id)[0].Status)
}

func TestWaiveAcCriterion_should_ReturnErrNotFound_When_ItemMissing(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)

	err := repo.WaiveAcCriterion(context.Background(), "bl_01HXYZABCDEFGHJKMNPQRSTVWX", 0, "why")

	assert.ErrorIs(t, err, domain.ErrNotFound)
}
