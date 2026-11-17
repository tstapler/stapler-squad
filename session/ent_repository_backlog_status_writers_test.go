package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstatusevent"
)

// statusEventsFor returns an item's audit rows oldest first.
func statusEventsFor(t *testing.T, repo *EntRepository, itemID string) []*ent.BacklogStatusEvent {
	t.Helper()
	ctx := context.Background()
	id, err := repo.ResolveBacklogItemLookup(ctx, itemID)
	require.NoError(t, err)
	evs, err := repo.Client.BacklogStatusEvent.Query().
		Where(backlogstatusevent.ItemID(id)).
		Order(ent.Asc(backlogstatusevent.FieldCreatedAt), ent.Asc(backlogstatusevent.FieldID)).
		All(ctx)
	require.NoError(t, err)
	return evs
}

func newStatusWriterItem(t *testing.T, repo *EntRepository, status domain.BacklogStatus) *backlogdto.BacklogItemData {
	t.Helper()
	item, err := repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "status writer", Status: string(status)})
	require.NoError(t, err)
	return item
}

func TestRawUnarchiveBacklogItem_should_RecordIdeaStatusEvent_When_ArchivedItemIsUnarchived(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusDone)
	_, err := repo.RawArchiveBacklogItem(ctx, item.ID, nil, domain.TriggeredBySystem, "")
	require.NoError(t, err)

	_, err = repo.RawUnarchiveBacklogItem(ctx, item.ID, string(domain.BacklogStatusArchived))
	require.NoError(t, err)

	evs := statusEventsFor(t, repo, item.ID)
	require.Len(t, evs, 2, "one row for the archive, one for the unarchive")
	last := evs[1]
	assert.Equal(t, string(domain.BacklogStatusArchived), last.FromStatus)
	assert.Equal(t, string(domain.BacklogStatusIdea), last.ToStatus)
	assert.Equal(t, domain.TriggeredByUser, last.TriggeredBy)
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_WriteStatusPRFieldsAndEventTogether_When_PreconditionHolds(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)

	got, err := repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReview), Note: "opened"},
		domain.TriggeredByAgent, nil)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusPRPending), got.Status)
	assert.Equal(t, 7, got.PrNumber)
	assert.Equal(t, "https://example.test/o/r/pull/7", got.PrURL)

	evs := statusEventsFor(t, repo, item.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, string(domain.BacklogStatusReview), evs[0].FromStatus)
	assert.Equal(t, string(domain.BacklogStatusPRPending), evs[0].ToStatus)
	assert.Equal(t, domain.TriggeredByAgent, evs[0].TriggeredBy)
	require.NotNil(t, evs[0].Note)
	assert.Equal(t, "opened", *evs[0].Note)
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_WriteNothing_When_ExpectedStatusIsStale(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)

	_, err := repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusInProgress)},
		domain.TriggeredBySystem, nil)
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)

	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusReview), after.Status)
	assert.Zero(t, after.PrNumber, "a lost CAS must not leak the PR fields")
	assert.Empty(t, after.PrURL)
	assert.Empty(t, statusEventsFor(t, repo, item.ID), "a lost CAS writes no audit row")
}

func TestRawArchiveBacklogItem_should_RejectAndLeaveItemUntouched_When_ExpectedStatusIsStale(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusIdea)

	_, err := repo.RawArchiveBacklogItem(ctx, item.ID,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReview)}, domain.TriggeredBySystem, "")
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)

	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusIdea), after.Status)
	assert.Nil(t, after.ArchivedAt)
	assert.Empty(t, statusEventsFor(t, repo, item.ID))
}

func TestClaimChainFire_should_ClaimOnce_When_TwoCallersRaceWithTheSameUpdatedAt(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusDone)

	claimed, err := repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	require.NoError(t, err)
	require.True(t, claimed)

	// The claim bumps updated_at; a loser holding the old value, or a fresh value
	// with chain_fired already true, must both come back claimed=false without error.
	again, err := repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	require.NoError(t, err)
	assert.False(t, again, "stale updated_at loses the claim")

	fresh, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.True(t, fresh.ChainFired)
	again, err = repo.ClaimChainFire(ctx, item.ID, fresh.UpdatedAt)
	require.NoError(t, err)
	assert.False(t, again, "an already-claimed item cannot be claimed again even with a current updated_at")
}

func TestClaimChainFire_should_RefuseClaim_When_UpdatedAtMovedOnAnUnclaimedItem(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusDone)

	claimed, err := repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt.Add(-time.Hour))
	require.NoError(t, err)
	assert.False(t, claimed, "the claim is a CAS on updated_at as well as chain_fired")
	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, after.ChainFired)
}

func TestClaimChainFire_should_PublishChainFiredUpdate_When_ClaimSucceeds(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusDone)
	published := captureChanges(repo)

	claimed, err := repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	require.NoError(t, err)
	require.True(t, claimed)

	got := published()
	require.Len(t, got, 1)
	assert.Equal(t, backlogdto.ChangeItemUpdated, got[0].change.Kind)
	assert.Equal(t, []string{"chainFired"}, got[0].change.UpdatedFields)

	// A lost claim publishes nothing.
	_, err = repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	require.NoError(t, err)
	assert.Len(t, published(), 1)
}

func TestRevertChainFireClaim_should_ResetChainFiredToFalse_When_ItemWasClaimed(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusDone)
	claimed, err := repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, repo.RevertChainFireClaim(ctx, item.ID))

	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, after.ChainFired, "the reconciler can retry once the claim is released")
}

func TestChainFireClaim_should_ReturnNotFound_When_IDIsNotResolvable(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := repo.ClaimChainFire(ctx, "not-a-uuid-or-public-id", time.Now())
	require.ErrorIs(t, err, domain.ErrNotFound)
	err = repo.RevertChainFireClaim(ctx, "not-a-uuid-or-public-id")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRawTransitionBacklogItemStatus_should_StampChainedAtOnlyOnDone_When_NextWorkflowIsConfigured(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusInProgress)
	id, err := repo.ResolveBacklogItemLookup(ctx, item.ID)
	require.NoError(t, err)
	_, err = repo.Client.BacklogItem.UpdateOneID(id).SetNextWorkflowID(uuid.New()).Save(ctx)
	require.NoError(t, err)

	mid, err := repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusReview, nil, domain.TriggeredBySystem)
	require.NoError(t, err)
	assert.Nil(t, mid.ChainedAt, "chained_at is the terminal done transition's timestamp, not any transition's")

	done, err := repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusDone, nil, domain.TriggeredBySystem)
	require.NoError(t, err)
	assert.NotNil(t, done.ChainedAt)
}

func TestRawTransitionBacklogItemStatus_should_LeaveChainedAtUnset_When_NoNextWorkflow(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	done, err := repo.RawTransitionBacklogItemStatus(context.Background(), item.ID, domain.BacklogStatusDone, nil, domain.TriggeredBySystem)
	require.NoError(t, err)
	assert.Nil(t, done.ChainedAt)
}

type recordingDispatcher struct {
	mu     sync.Mutex
	events []string
}

func (d *recordingDispatcher) Dispatch(eventType string, _ any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, eventType)
}

func (d *recordingDispatcher) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.events...)
}

func TestRawTransitionBacklogItemStatus_should_DispatchSessionCompleteCallback_OnlyWhenTransitionedToDone(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	d := &recordingDispatcher{}
	repo.SetCallbackDispatcher(d)
	item := newStatusWriterItem(t, repo, domain.BacklogStatusInProgress)

	_, err := repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusReview, nil, domain.TriggeredBySystem)
	require.NoError(t, err)
	assert.Empty(t, d.snapshot(), "a non-terminal transition fires no callback")

	_, err = repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusDone, nil, domain.TriggeredBySystem)
	require.NoError(t, err)
	assert.Equal(t, []string{"session_complete"}, d.snapshot())
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_PublishStatusTransition_When_PreconditionHolds(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	published := captureChanges(repo)

	_, err := repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/9", 9,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReview)}, domain.TriggeredBySystem, nil)
	require.NoError(t, err)

	got := published()
	require.Len(t, got, 1)
	assert.Equal(t, backlogdto.ChangeStatusTransition, got[0].change.Kind)
	assert.Equal(t, string(domain.BacklogStatusReview), got[0].change.OldStatus)
	assert.Equal(t, string(domain.BacklogStatusPRPending), got[0].change.NewStatus)
}

// A concurrent notes edit injected after the writer's read and before its UPDATE
// must win: the note-appending UPDATE is pinned to the updated_at it read.
func TestRawTransitionBacklogItemStatusWithNote_should_LoseTheWrite_When_NotesChangeBetweenReadAndWrite(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	id, err := repo.ResolveBacklogItemLookup(ctx, item.ID)
	require.NoError(t, err)

	var once sync.Once
	repo.Client.BacklogItem.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(c context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(ent.OpUpdate) {
				once.Do(func() {
					_, rerr := repo.Client.BacklogItem.UpdateOneID(id).SetNotes("racing edit").Save(c)
					assert.NoError(t, rerr)
				})
			}
			return next.Mutate(c, m)
		})
	})

	_, err = repo.RawTransitionBacklogItemStatusWithNote(ctx, item.ID, domain.BacklogStatusPRPending, nil, domain.TriggeredBySystem, "[auto] note")
	require.ErrorIs(t, err, domain.ErrPreconditionFailed)

	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "racing edit", after.Notes, "the concurrent edit survives")
	assert.Equal(t, string(domain.BacklogStatusReview), after.Status)
}
