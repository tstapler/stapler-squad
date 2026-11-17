package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent/backlogitemdependency"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
)

// Effect-level pins for the Client-only writers that live in session/backlogstore.

func newStuckRow(t *testing.T, repo *EntRepository, reason domain.StuckReason) (string, context.Context) {
	t.Helper()
	ctx := context.Background()
	itemID := createStuckTestItem(t, repo, ctx, domain.BacklogStatusPRPending)
	applied, err := repo.MarkStuck(ctx, itemID, reason, domain.BacklogStatusPRPending, "ctx")
	require.NoError(t, err)
	require.True(t, applied)
	return itemID, ctx
}

func stuckRow(t *testing.T, repo *EntRepository, ctx context.Context, itemID string, reason domain.StuckReason) (attempts, nudges int32, row interface{}) {
	t.Helper()
	r, err := repo.Client.BacklogStuckState.Query().
		Where(backlogstuckstate.ItemID(uuid.MustParse(itemID)), backlogstuckstate.Reason(string(reason))).
		Only(ctx)
	require.NoError(t, err)
	return r.RemediationAttempts, r.DiagnoseNudgeCount, r
}

func TestClientWriters_RecordRemediationAttempt_should_SetCountAndNext_ThenClearNext_AndSkipResolvedRows(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	reason := domain.StuckReasonBouncing
	q := func() *time.Time {
		r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
		require.NoError(t, err)
		assert.Equal(t, int32(2), r.RemediationAttempts)
		return r.NextRemediationAt
	}

	next := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	ok, err := repo.RecordRemediationAttempt(ctx, itemID, reason, 2, &next)
	require.NoError(t, err)
	assert.True(t, ok)
	got := q()
	require.NotNil(t, got)
	assert.True(t, next.Equal(*got))

	ok, err = repo.RecordRemediationAttempt(ctx, itemID, reason, 2, nil)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Nil(t, q(), "nil nextAt clears next_remediation_at")

	_, err = repo.ResolveStuck(ctx, itemID, reason)
	require.NoError(t, err)
	ok, err = repo.RecordRemediationAttempt(ctx, itemID, reason, 4, &next)
	require.NoError(t, err)
	assert.False(t, ok, "a resolved row is never resurrected")
}

func TestClientWriters_RecordRemediationRestartGrace_should_SetGraceBootTimeOnly(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	reason := domain.StuckReasonBouncing
	next := time.Now().Add(time.Hour)
	_, err := repo.RecordRemediationAttempt(ctx, itemID, reason, 3, &next)
	require.NoError(t, err)

	boot := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	ok, err := repo.RecordRemediationRestartGrace(ctx, itemID, reason, boot)
	require.NoError(t, err)
	assert.True(t, ok)

	r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, r.GraceBootTime)
	assert.True(t, boot.Equal(*r.GraceBootTime))
	assert.Equal(t, int32(3), r.RemediationAttempts, "grace pass spends no attempt budget")
	assert.NotNil(t, r.NextRemediationAt)

	_, err = repo.ResolveStuck(ctx, itemID, reason)
	require.NoError(t, err)
	ok, err = repo.RecordRemediationRestartGrace(ctx, itemID, reason, boot)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClientWriters_ResetStuckRemediation_should_ClearCountersAndNotified_AndIgnoreResolvedRows(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	reason := domain.StuckReasonBouncing
	next := time.Now().Add(time.Hour)
	_, err := repo.RecordRemediationAttempt(ctx, itemID, reason, 4, &next)
	require.NoError(t, err)
	_, err = repo.MarkStuckNotified(ctx, itemID, reason)
	require.NoError(t, err)

	ok, err := repo.ResetStuckRemediation(ctx, itemID, reason)
	require.NoError(t, err)
	assert.True(t, ok)
	r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(0), r.RemediationAttempts)
	assert.Nil(t, r.NextRemediationAt)
	assert.Nil(t, r.NotifiedAt)

	ok, err = repo.ResetStuckRemediation(ctx, itemID, domain.StuckReasonStaleWork)
	require.NoError(t, err)
	assert.False(t, ok, "no open row for another reason")
}

func TestClientWriters_BulkResetStuckRemediation_should_RespectOnlyParkedAndReasonFilter(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	parkedID, _ := newStuckRow(t, repo, domain.StuckReasonBouncing)
	freshID, _ := newStuckRow(t, repo, domain.StuckReasonBouncing)
	otherID, _ := newStuckRow(t, repo, domain.StuckReasonStaleWork)
	_, err := repo.RecordRemediationAttempt(ctx, parkedID, domain.StuckReasonBouncing, MaxRemediationAttempts, nil)
	require.NoError(t, err)
	_, err = repo.RecordRemediationAttempt(ctx, freshID, domain.StuckReasonBouncing, MaxRemediationAttempts-1, nil)
	require.NoError(t, err)
	_, err = repo.RecordRemediationAttempt(ctx, otherID, domain.StuckReasonStaleWork, MaxRemediationAttempts, nil)
	require.NoError(t, err)

	bouncing := domain.StuckReasonBouncing
	n, err := repo.BulkResetStuckRemediation(ctx, &bouncing, true)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the parked row of the filtered reason resets (cap boundary is inclusive, cap-1 is not parked)")

	attempts, _, _ := stuckRow(t, repo, ctx, freshID, domain.StuckReasonBouncing)
	assert.Equal(t, MaxRemediationAttempts-1, attempts)
	attempts, _, _ = stuckRow(t, repo, ctx, otherID, domain.StuckReasonStaleWork)
	assert.Equal(t, MaxRemediationAttempts, attempts, "other reason untouched")

	n, err = repo.BulkResetStuckRemediation(ctx, nil, false)
	require.NoError(t, err)
	assert.Equal(t, 3, n, "onlyParked=false with no reason resets every open row")
}

func TestClientWriters_SnoozeStuckState_should_SetSnoozedUntil(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	until := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	ok, err := repo.SnoozeStuckState(ctx, itemID, domain.StuckReasonBouncing, until)
	require.NoError(t, err)
	assert.True(t, ok)
	r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, r.SnoozedUntil)
	assert.True(t, until.Equal(*r.SnoozedUntil))

	ok, err = repo.SnoozeStuckState(ctx, itemID, domain.StuckReasonStaleWork, until)
	require.NoError(t, err)
	assert.False(t, ok, "no open row for another reason")
}

func TestClientWriters_DiagnoseNudgeWriters_should_EnforceCapAndCooldownAtomically(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	reason := domain.StuckReasonBouncing
	now := time.Now().UTC().Truncate(time.Second)

	next := now.Add(time.Hour)
	ok, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, reason, 1, &next)
	require.NoError(t, err)
	assert.True(t, ok)
	r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), r.DiagnoseNudgeCount)
	require.NotNil(t, r.DiagnoseNextEligibleAt)
	ok, err = repo.RecordDiagnoseNudgeAttempt(ctx, itemID, reason, 1, nil)
	require.NoError(t, err)
	assert.True(t, ok)
	r, err = repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, r.DiagnoseNextEligibleAt, "nil nextAt clears the eligibility time")

	// IfBelowCap: count 1, cap 2 -> increments to 2 once, then refuses.
	ok, err = repo.RecordDiagnoseNudgeAttemptIfBelowCap(ctx, itemID, reason, 2, next)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.RecordDiagnoseNudgeAttemptIfBelowCap(ctx, itemID, reason, 2, next)
	require.NoError(t, err)
	assert.False(t, ok, "at cap")
	_, nudges, _ := stuckRow(t, repo, ctx, itemID, reason)
	assert.Equal(t, int32(2), nudges)

	// ClaimSlot: the IfBelowCap call left an hour-long cooldown, so claim from after it has elapsed.
	now = now.Add(2 * time.Hour)
	ok, err = repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, 3, now, time.Hour)
	require.NoError(t, err)
	assert.True(t, ok)
	r, err = repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(3), r.DiagnoseNudgeCount)
	require.NotNil(t, r.DiagnoseNextEligibleAt)
	assert.True(t, now.Add(time.Hour).Equal(*r.DiagnoseNextEligibleAt))
	ok, err = repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, 5, now, time.Hour)
	require.NoError(t, err)
	assert.False(t, ok, "cooldown active")
	ok, err = repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, 5, now.Add(time.Hour), 0)
	require.NoError(t, err)
	assert.True(t, ok, "claim at exactly the eligibility time is allowed")
	r, err = repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, r.DiagnoseNextEligibleAt, "cooldown <= 0 clears it")
	ok, err = repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, 4, now.Add(3*time.Hour), time.Hour)
	require.NoError(t, err)
	assert.False(t, ok, "count 4 is at claimLimit 4")
}

func TestClientWriters_StuckStateWriters_should_LeaveResolvedRowsAlone(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	itemID, ctx := newStuckRow(t, repo, domain.StuckReasonBouncing)
	reason := domain.StuckReasonBouncing
	_, err := repo.ResolveStuck(ctx, itemID, reason)
	require.NoError(t, err)
	next := time.Now().Add(time.Hour)

	ok, err := repo.RecordDiagnoseNudgeAttempt(ctx, itemID, reason, 1, &next)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.RecordDiagnoseNudgeAttemptIfBelowCap(ctx, itemID, reason, 5, next)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, 5, time.Now(), time.Hour)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.SnoozeStuckState(ctx, itemID, reason, next)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.ResetStuckRemediation(ctx, itemID, reason)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.ResolveStuck(ctx, itemID, reason)
	require.NoError(t, err)
	assert.False(t, ok, "resolving twice is a no-op")

	r, err := repo.Client.BacklogStuckState.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(0), r.DiagnoseNudgeCount)
	assert.Nil(t, r.SnoozedUntil)
}

func TestClientWriters_RemoveBacklogItemDependency_should_DeleteOnlyThatEdge_AndRejectMissingItems(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	mk := func() string {
		it, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "dep", Status: string(domain.BacklogStatusIdea)})
		require.NoError(t, err)
		return it.ID
	}
	a, b, c := mk(), mk(), mk()
	ab := backlogdto.BacklogItemDependencyEdge{BlockerID: a, BlockedID: b}
	ac := backlogdto.BacklogItemDependencyEdge{BlockerID: a, BlockedID: c}
	require.NoError(t, repo.AddBacklogItemDependency(ctx, ab))
	require.NoError(t, repo.AddBacklogItemDependency(ctx, ac))
	require.NoError(t, repo.AddBacklogItemDependency(ctx, ab), "re-adding an existing edge is a no-op")
	n, err := repo.Client.BacklogItemDependency.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	require.NoError(t, repo.RemoveBacklogItemDependency(ctx, ab))
	//nolint:entfullscan test assertion over a test-scoped in-memory DB; only the seeded edges exist.
	left, err := repo.Client.BacklogItemDependency.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, uuid.MustParse(c), left[0].BlockedID)
	assert.Equal(t, 0, mustCount(t, repo, ctx, uuid.MustParse(a), uuid.MustParse(b)))

	err = repo.RemoveBacklogItemDependency(ctx, backlogdto.BacklogItemDependencyEdge{BlockerID: a, BlockedID: uuid.NewString()})
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// a -> c exists, so c -> a (and the transitive c -> b -> ... ) must be rejected.
	require.NoError(t, repo.AddBacklogItemDependency(ctx, backlogdto.BacklogItemDependencyEdge{BlockerID: c, BlockedID: b}))
	err = repo.AddBacklogItemDependency(ctx, backlogdto.BacklogItemDependencyEdge{BlockerID: b, BlockedID: a})
	assert.ErrorIs(t, err, backlogdto.ErrDependencyCycle, "a->c->b means b->a closes a three-node cycle")
}

func mustCount(t *testing.T, repo *EntRepository, ctx context.Context, blocker, blocked uuid.UUID) int {
	t.Helper()
	n, err := repo.Client.BacklogItemDependency.Query().
		Where(backlogitemdependency.BlockerIDEQ(blocker), backlogitemdependency.BlockedIDEQ(blocked)).
		Count(ctx)
	require.NoError(t, err)
	return n
}

func TestClientWriters_AppendProgressNote_should_PersistEveryCallAsSeparateHistoryRow(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "notes", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)

	require.NoError(t, repo.AppendProgressNote(ctx, item.ID, 1, "first", "in_progress"))
	require.NoError(t, repo.AppendProgressNote(ctx, item.ID, 1, "second", "done"))
	//nolint:entfullscan test assertion over a test-scoped in-memory DB; the item has only these two notes.
	rows, err := repo.Client.BacklogProgressNote.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	got := map[string]string{rows[0].Note: rows[0].Status, rows[1].Note: rows[1].Status}
	assert.Equal(t, map[string]string{"first": "in_progress", "second": "done"}, got)
	assert.Equal(t, 1, rows[0].CriterionIndex)

	assert.Error(t, repo.AppendProgressNote(ctx, "not-a-real-id", 0, "x", "y"))
}

func newItemSession(t *testing.T, repo *EntRepository, ctx context.Context, itemID, sessionUUID, role string) string {
	t.Helper()
	is, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: sessionUUID, SessionRole: role})
	require.NoError(t, err)
	return is.ID
}

func TestClientWriters_ItemSessionFieldWriters_should_PersistEachColumn(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "is", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	id := newItemSession(t, repo, ctx, item.ID, "sess-fields", domain.SessionRoleWork)
	uid := uuid.MustParse(id)

	end := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.UpdateItemSessionEndedWithDetail(ctx, id, end, "other", "boom detail"))
	row, err := repo.Client.ItemSession.Get(ctx, uid)
	require.NoError(t, err)
	require.NotNil(t, row.EndedAt)
	assert.True(t, end.Equal(*row.EndedAt))
	assert.Equal(t, "other", row.EndReason)
	assert.Equal(t, "boom detail", row.ErrorDetail)

	end2 := end.Add(time.Minute)
	require.NoError(t, repo.UpdateItemSessionEnded(ctx, id, end2))
	row, err = repo.Client.ItemSession.Get(ctx, uid)
	require.NoError(t, err)
	assert.True(t, end2.Equal(*row.EndedAt))
	assert.Equal(t, "", row.EndReason, "UpdateItemSessionEnded writes an empty reason")

	require.NoError(t, repo.UpdateItemSessionFailureCapture(ctx, id, "/var/capture/x.txt"))
	require.NoError(t, repo.UpdateItemSessionConversationUUID(ctx, id, "conv-123"))
	require.NoError(t, repo.SetItemSessionBaseCommit(ctx, id, "abc123"))
	row, err = repo.Client.ItemSession.Get(ctx, uid)
	require.NoError(t, err)
	assert.Equal(t, "/var/capture/x.txt", row.FailureCapturePath)
	assert.Equal(t, "conv-123", row.ConversationUUID)
	assert.Equal(t, "abc123", row.BaseCommitSha)

	for _, err := range []error{
		repo.UpdateItemSessionEnded(ctx, "nope", end),
		repo.UpdateItemSessionEndedWithDetail(ctx, "nope", end, "r", "d"),
		repo.UpdateItemSessionFailureCapture(ctx, "nope", "p"),
		repo.UpdateItemSessionConversationUUID(ctx, "nope", "c"),
		repo.SetItemSessionBaseCommit(ctx, "nope", "s"),
		repo.UpdateItemSessionCost(ctx, "nope", 1, true),
	} {
		assert.Error(t, err, "a malformed id is rejected")
	}
}

func TestClientWriters_ItemSessionCostWriters_should_AccumulateAndStickUnpriced(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "cost", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	id := newItemSession(t, repo, ctx, item.ID, "sess-cost", domain.SessionRoleWork)
	uid := uuid.MustParse(id)

	require.NoError(t, repo.UpdateItemSessionCost(ctx, id, 1.5, true))
	require.NoError(t, repo.AddHeadlessCostBySessionUUID(ctx, "sess-cost", 0.5))
	row, err := repo.Client.ItemSession.Get(ctx, uid)
	require.NoError(t, err)
	assert.InDelta(t, 2.0, row.EstimatedCostUsd, 1e-9, "priced calls add up")
	assert.True(t, row.CostPriced)

	require.NoError(t, repo.AddHeadlessCostBySessionUUID(ctx, "sess-cost", 0), "zero usd is a no-op")
	require.NoError(t, repo.AddHeadlessCostBySessionUUID(ctx, "sess-cost", -3), "negative usd is a no-op")
	require.NoError(t, repo.AddHeadlessCostBySessionUUID(ctx, "no-such-session", 4), "unknown session is not an error")

	require.NoError(t, repo.UpdateItemSessionCost(ctx, id, 9, false))
	require.NoError(t, repo.UpdateItemSessionCost(ctx, id, 1, true))
	row, err = repo.Client.ItemSession.Get(ctx, uid)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, row.EstimatedCostUsd, 1e-9, "an unpriced call adds nothing")
	assert.False(t, row.CostPriced, "cost_priced stays false once any call was unpriced")
}

func TestClientWriters_ClaimDiagnoseNudgeAttempt_should_ClaimOncePerDiagnoseSession(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "nudge", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	newItemSession(t, repo, ctx, item.ID, "diag-1", domain.SessionRoleDiagnose)
	newItemSession(t, repo, ctx, item.ID, "work-1", domain.SessionRoleWork)

	claimed, err := repo.ClaimDiagnoseNudgeAttempt(ctx, "diag-1")
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = repo.ClaimDiagnoseNudgeAttempt(ctx, "diag-1")
	require.NoError(t, err)
	assert.False(t, claimed, "second claim is refused")
	claimed, err = repo.ClaimDiagnoseNudgeAttempt(ctx, "work-1")
	require.NoError(t, err)
	assert.False(t, claimed, "a non-diagnose session never claims")
	claimed, err = repo.ClaimDiagnoseNudgeAttempt(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestClientWriters_ListItemSessionsForItems_should_GroupByItemOldestFirst_AndSkipNonUUIDs(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	mk := func(title string) string {
		it, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: title, Status: string(domain.BacklogStatusIdea)})
		require.NoError(t, err)
		return it.ID
	}
	itemA, itemB, itemEmpty := mk("a"), mk("b"), mk("empty")
	base := time.Now().Add(-time.Hour)
	add := func(itemID, sess string, at time.Time) {
		_, err := repo.Client.ItemSession.Create().SetSessionUUID(sess).SetSessionRole(domain.SessionRoleWork).
			SetBacklogItemID(uuid.MustParse(itemID)).SetCreatedAt(at).Save(ctx)
		require.NoError(t, err)
	}
	add(itemA, "a-new", base.Add(2*time.Minute))
	add(itemA, "a-old", base)
	add(itemB, "b-1", base.Add(time.Minute))

	got, err := repo.ListItemSessionsForItems(ctx, []string{itemA, itemB, itemEmpty, "not-a-uuid"})
	require.NoError(t, err)
	require.Len(t, got, 2, "the empty item and the malformed id map to no entry")
	require.Len(t, got[itemA], 2)
	assert.Equal(t, []string{"a-old", "a-new"}, []string{got[itemA][0].SessionUUID, got[itemA][1].SessionUUID}, "oldest first")
	assert.Equal(t, itemA, got[itemA][0].BacklogItemID)
	require.Len(t, got[itemB], 1)
	assert.Equal(t, itemB, got[itemB][0].BacklogItemID)
}

func TestClientWriters_ListItemSessionsForItems_should_CoverEveryChunkOfALargeBatch(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	n := domain.ItemSessionBatchChunk + 1
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		it, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "bulk", Status: string(domain.BacklogStatusIdea)})
		require.NoError(t, err)
		_, err = repo.Client.ItemSession.Create().SetSessionUUID(uuid.NewString()).SetSessionRole(domain.SessionRoleWork).
			SetBacklogItemID(uuid.MustParse(it.ID)).Save(ctx)
		require.NoError(t, err)
		ids = append(ids, it.ID)
	}

	got, err := repo.ListItemSessionsForItems(ctx, ids)
	require.NoError(t, err)
	assert.Len(t, got, n, "the item after the first chunk boundary is included")
}
