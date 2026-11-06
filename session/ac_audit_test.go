package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func ownerCtx() context.Context {
	return session.WithAcActor(context.Background(), session.AcActor{
		Class: session.AcActorOperatorAuthenticated, Source: "ui", RemoteAddr: "127.0.0.1:1", UserAgent: "ua",
	})
}

func TestAcAudit_should_RecordPreviousStatusAndActor_When_WaiveApplied(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "owner call"))

	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "pending", rows[0].PreviousStatus)
	assert.Equal(t, "waived", rows[0].NewStatus)
	assert.Equal(t, "applied", rows[0].Outcome)
	assert.Equal(t, "owner call", rows[0].Reason)
	assert.Equal(t, string(session.AcActorOperatorAuthenticated), rows[0].ActorClass)
	assert.Equal(t, "ui", rows[0].Source)
	assert.Equal(t, "127.0.0.1:1", rows[0].RemoteAddr)
	assert.Equal(t, "ua", rows[0].UserAgent)
	got := loadCriteria(t, repo, id)[0]
	assert.Equal(t, "operator (authenticated)", got.WaivedBy)
	assert.NotEmpty(t, got.WaivedAt)
}

func TestAcAudit_should_RecordRejection_When_WaivingDoneCriterion(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)

	err := repo.WaiveAcCriterion(ownerCtx(), id, 1, "why") // criterion 1 is done

	require.ErrorIs(t, err, session.ErrAcCriterionAlreadyDone)
	assert.Equal(t, session.AcStatusDone, loadCriteria(t, repo, id)[1].Status)
	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "done", rows[0].PreviousStatus)
	assert.Contains(t, rows[0].Outcome, "rejected:")
}

func TestAcAudit_should_RecordOneRowPerMutation_When_AgentChangesStatusAndHitsWaiver(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	agent := session.WithAcActor(context.Background(), session.AcActor{
		Class: session.AcActorMCPSession, SessionUUID: "sess-1", SessionVerified: true, Source: "mcp",
	})

	require.NoError(t, repo.UpdateAcCriterionStatus(agent, id, 0, "in_progress", "working"))
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "owner call"))
	require.ErrorIs(t, repo.UpdateAcCriterionStatus(agent, id, 0, "done", ""), session.ErrAcCriterionWaived)
	require.ErrorIs(t, repo.UpdateAcCriterionStatus(agent, id, 9, "done", ""), session.ErrAcCriterionOutOfRange)

	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, []string{"pending", "in_progress", "waived", ""}, []string{
		rows[0].PreviousStatus, rows[1].PreviousStatus, rows[2].PreviousStatus, rows[3].PreviousStatus})
	assert.Equal(t, "applied", rows[0].Outcome)
	assert.Equal(t, "mcp", rows[0].Source)
	assert.True(t, rows[0].SessionVerified)
	assert.Equal(t, "sess-1", rows[0].ActorSessionUUID)
	assert.Contains(t, rows[2].Outcome, "rejected:")
}

func TestListAcAudit_should_ReturnErrNotFound_When_ItemMissing(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	_, err := repo.ListAcAudit(context.Background(), "bl_01HXYZABCDEFGHJKMNPQRSTVWX")
	assert.ErrorIs(t, err, session.ErrNotFound)
}
