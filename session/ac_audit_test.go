package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func ownerCtx() context.Context {
	return session.WithAcActor(context.Background(), session.AcActor{
		Class: session.AcActorAuthenticatedSession, Source: "ui", RemoteAddr: "127.0.0.1:1", UserAgent: "ua",
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
	assert.Equal(t, string(session.AcActorAuthenticatedSession), rows[0].ActorClass)
	assert.Equal(t, "ui", rows[0].Source)
	assert.Equal(t, "127.0.0.1:1", rows[0].RemoteAddr)
	assert.Equal(t, "ua", rows[0].UserAgent)
	got := loadCriteria(t, repo, id)[0]
	assert.Equal(t, "session cookie credential", got.WaivedBy)
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
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func createACItem(t *testing.T, repo *session.EntRepository, texts ...string) string {
	t.Helper()
	cs := make([]session.AcCriterion, len(texts))
	for i, tx := range texts {
		cs[i] = session.AcCriterion{Index: i, Text: tx, Status: session.AcStatusPending}
	}
	ac, err := session.SerializeAcCriteria(cs)
	require.NoError(t, err)
	item, err := repo.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title: "ac item", Status: string(session.BacklogStatusInProgress), AcceptanceCriteria: ac,
	})
	require.NoError(t, err)
	return item.ID
}

func rewriteAC(t *testing.T, repo *session.EntRepository, id string, in []session.AcCriterion) error {
	t.Helper()
	ac, err := session.SerializeAcCriteria(in)
	require.NoError(t, err)
	_, err = repo.UpdateBacklogItem(ownerCtx(), id, session.BacklogItemUpdate{AcceptanceCriteria: &ac}, nil)
	return err
}

func pend(texts ...string) []session.AcCriterion {
	out := make([]session.AcCriterion, len(texts))
	for i, tx := range texts {
		out[i] = session.AcCriterion{Index: i, Text: tx, Status: session.AcStatusPending}
	}
	return out
}

func TestUpdateBacklogItem_should_CarryWaiverByText_When_CriteriaReorderedOrInserted(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := createACItem(t, repo, "alpha", "beta", "gamma")
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 1, "beta waived"))

	// insert at the front and reorder, with different casing/spacing on the waived text
	require.NoError(t, rewriteAC(t, repo, id, pend("new first", "gamma", "  BETA  ", "alpha")))

	got := loadCriteria(t, repo, id)
	require.Len(t, got, 4)
	for i, c := range got {
		if i == 2 {
			assert.Equal(t, session.AcStatusWaived, c.Status)
			assert.Equal(t, "beta waived", c.WaiverReason)
			assert.NotEmpty(t, c.WaivedBy)
		} else {
			assert.Equal(t, session.AcStatusPending, c.Status, "criterion %d must not inherit the waiver", i)
		}
	}
}

func TestUpdateBacklogItem_should_ClearAndAuditWaiver_When_CriterionEditedOrRemoved(t *testing.T) {
	t.Parallel()
	for name, next := range map[string][]session.AcCriterion{
		"edited":  pend("alpha", "beta changed meaning"),
		"removed": pend("alpha"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := session.NewTestEntRepository(t)
			id := createACItem(t, repo, "alpha", "beta")
			require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 1, "why"))

			require.NoError(t, rewriteAC(t, repo, id, next))

			for _, c := range loadCriteria(t, repo, id) {
				assert.NotEqual(t, session.AcStatusWaived, c.Status)
				assert.Empty(t, c.WaivedBy)
			}
			rows, err := repo.ListAcAudit(context.Background(), id)
			require.NoError(t, err)
			require.Len(t, rows, 2)
			assert.Equal(t, "waived", rows[1].PreviousStatus)
			assert.Contains(t, rows[1].Outcome, "waiver_cleared")
		})
	}
}

func TestUpdateBacklogItem_should_RejectAndAudit_When_RewriteIntroducesWaiver(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := createACItem(t, repo, "alpha")
	in := pend("alpha")
	in[0].Status = session.AcStatusWaived

	err := rewriteAC(t, repo, id, in)

	require.ErrorIs(t, err, session.ErrAcWaiverViaUpdate)
	assert.Equal(t, session.AcStatusPending, loadCriteria(t, repo, id)[0].Status)
	rows, lerr := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, lerr)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Outcome, "rejected:")
}

func TestAcAudit_should_SkipRepeatedSameStatusAndRefuseRewaive(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	agent := session.WithAcActor(context.Background(), session.AcActor{Class: session.AcActorMCPSession, Source: "mcp"})

	require.NoError(t, repo.UpdateAcCriterionStatus(agent, id, 0, "in_progress", "a"))
	require.NoError(t, repo.UpdateAcCriterionStatus(agent, id, 0, "in_progress", "b"))
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "first"))
	require.ErrorIs(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "second"), session.ErrAcCriterionAlreadyWaived)

	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, rows, 3, "one transition, one waiver, one refused re-waive")
	assert.Equal(t, "first", loadCriteria(t, repo, id)[0].WaiverReason)
}

func TestAcAudit_should_StayReadableAfterItemDelete_And_PruneByAge(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "r"))

	require.NoError(t, repo.DeleteBacklogItem(context.Background(), id))

	for _, key := range []string{item.ID, id} {
		rows, err := repo.ListAcAudit(context.Background(), key)
		require.NoError(t, err, key)
		require.Len(t, rows, 1, key)
	}
	n, err := repo.PruneAcAuditOlderThan(context.Background(), time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, err = repo.ListAcAudit(context.Background(), id)
	assert.ErrorIs(t, err, domain.ErrNotFound, "item gone and no rows left")
}

func TestAcAudit_should_KeepWaiverRow_When_DeniedFloodExceedsRefusalCap(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "owner call"))

	for i := 0; i < 500+5; i++ {
		require.NoError(t, repo.RecordAcAudit(ownerCtx(), session.AcAuditEventData{ItemID: item.ID, Outcome: "denied:flood"}))
	}

	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	var applied, refused int
	for _, r := range rows {
		if r.Outcome == "applied" {
			applied++
		} else {
			refused++
		}
	}
	assert.Equal(t, 1, applied, "the waiver's own row must survive")
	assert.Equal(t, session.MaxAcAuditRefusalsPerItem, refused)
}

func TestPruneAcAuditOlderThan_should_KeepLiveItemWaiverRow_But_DropOldRefusals(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := newWaiverItem(t, repo)
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 0, "r"))
	require.NoError(t, repo.RecordAcAudit(ownerCtx(), session.AcAuditEventData{ItemID: item.ID, Outcome: "denied:x"}))

	n, err := repo.PruneAcAuditOlderThan(context.Background(), time.Now().Add(time.Hour))

	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the refusal is old-and-prunable while the item lives")
	rows, err := repo.ListAcAudit(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "applied", rows[0].Outcome)
}

func TestAcActorFrom_should_DefaultToSystem_When_NoActorAttached(t *testing.T) {
	t.Parallel()
	a := session.AcActorFrom(context.Background())
	assert.Equal(t, session.AcActorSystem, a.Class)
	assert.Equal(t, "system (automated)", a.Label())
}

func TestUpdateBacklogItem_should_CarryWaivedByAtAndHandleDuplicateAndEmptyTexts(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := createACItem(t, repo, "same", "same", "")
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 1, "second dup"))
	before := loadCriteria(t, repo, id)[1]

	// swap the duplicates and add another empty-text criterion
	require.NoError(t, rewriteAC(t, repo, id, pend("same", "same", "", "")))

	got := loadCriteria(t, repo, id)
	require.Len(t, got, 4)
	waived := 0
	for _, c := range got {
		if c.Status == session.AcStatusWaived {
			waived++
			assert.Equal(t, before.WaivedBy, c.WaivedBy)
			assert.Equal(t, before.WaivedAt, c.WaivedAt)
			assert.Equal(t, "second dup", c.WaiverReason)
			assert.Equal(t, "same", c.Text)
		}
	}
	assert.Equal(t, 1, waived, "exactly one of the duplicate texts holds the single waiver")
	assert.NotEqual(t, session.AcStatusWaived, got[2].Status)
	assert.NotEqual(t, session.AcStatusWaived, got[3].Status, "empty text never carries a waiver")
}

func TestUpdateBacklogItem_should_ReportClearedWaivers_And_RollBackOtherFields_When_Rejected(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	id := createACItem(t, repo, "alpha", "beta")
	require.NoError(t, repo.WaiveAcCriterion(ownerCtx(), id, 1, "why"))

	title := "renamed"
	ac, err := session.SerializeAcCriteria(pend("alpha", "beta reworded"))
	require.NoError(t, err)
	res, err := repo.UpdateBacklogItem(ownerCtx(), id, session.BacklogItemUpdate{Title: &title, AcceptanceCriteria: &ac}, nil)
	require.NoError(t, err)
	require.Len(t, res.ClearedAcWaivers, 1)
	assert.Equal(t, "beta", res.ClearedAcWaivers[0].Text)
	assert.Equal(t, "renamed", res.Title)

	// A rejected rewrite must not apply the title either.
	bad := pend("alpha", "beta reworded")
	bad[0].Status = session.AcStatusWaived
	title2 := "should not stick"
	badJSON, err := session.SerializeAcCriteria(bad)
	require.NoError(t, err)
	_, err = repo.UpdateBacklogItem(ownerCtx(), id, session.BacklogItemUpdate{Title: &title2, AcceptanceCriteria: &badJSON}, nil)
	require.ErrorIs(t, err, session.ErrAcWaiverViaUpdate)
	item, err := repo.GetBacklogItem(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "renamed", item.Title)
}

func TestUpdateBacklogItem_should_TolerateLegacyPlainTextStoredCriteria_And_RejectPlainTextIncoming(t *testing.T) {
	t.Parallel()
	repo := session.NewTestEntRepository(t)
	item, err := repo.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title: "legacy", Status: string(session.BacklogStatusIdea), AcceptanceCriteria: session.AcCriteriaJSON("- do the thing\n- other"),
	})
	require.NoError(t, err)

	plain := session.AcCriteriaJSON("still plain text")
	_, err = repo.UpdateBacklogItem(ownerCtx(), item.ID, session.BacklogItemUpdate{AcceptanceCriteria: &plain}, nil)
	require.Error(t, err, "plain-text criteria are not valid input to an update")

	ac, serr := session.SerializeAcCriteria(pend("do the thing"))
	require.NoError(t, serr)
	_, err = repo.UpdateBacklogItem(ownerCtx(), item.ID, session.BacklogItemUpdate{AcceptanceCriteria: &ac}, nil)
	require.NoError(t, err, "an unreadable stored list pins nothing and is replaced")
}
