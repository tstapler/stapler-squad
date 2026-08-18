package session_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// This file lives in the external session_test package (not the internal
// session package like ent_repository_backlog_test.go) because it needs to
// import server/services.BacklogItemEventPublisher (the real adapter), which
// itself imports session — an internal test file in package session cannot
// import anything that transitively imports session without Go's "import
// cycle not allowed in test" build error. See Epic 2.1's implementation notes
// for the exact error this avoids.

// newTestEntRepositoryForEvents creates a temporary ent-backed *session.EntRepository,
// mirroring session package's own unexported createTestEntRepository test helper
// (session/ent_repository_test.go), which is not reachable from this external
// test package.
func newTestEntRepositoryForEvents(t *testing.T) (*session.EntRepository, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return repo, cleanup
}

// panickingItemChangePublisher is a test double for Task 2.1.1d: its
// PublishItemChanged always panics, simulating a broken publisher wired
// directly (i.e. not wrapped by the real adapter's own recover()), so the
// test can prove a hooked repository method is unaffected by a panic inside
// the publish step regardless of which ItemChangePublisher implementation is
// wired in.
type panickingItemChangePublisher struct{}

func (panickingItemChangePublisher) PublishItemChanged(item *session.BacklogItemData, change session.BacklogItemChange) {
	panic("boom")
}

// TestTransitionBacklogItemStatus_should_publishOldAndNewStatus_When_CASSucceeds
// wires a real *pkgevents.EventBus into the repository via SetItemChangePublisher
// (through the real server/services.BacklogItemEventPublisher adapter) and asserts
// a subscriber receives a BacklogItemChanged event with the correct old/new status
// after a successful CAS transition (Task 2.1.1b, R4 happy path).
func TestTransitionBacklogItemStatus_should_publishOldAndNewStatus_When_CASSucceeds(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for status transition publish test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, &session.BacklogItemPrecondition{
		ExpectedStatus: string(session.BacklogStatusInProgress),
	}, session.TriggeredBySystem)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.Equal(t, pkgevents.EventBacklogItemChanged, ev.Type)
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeStatusTransition, ev.BacklogItemPayload.Kind)
		assert.Equal(t, string(session.BacklogStatusInProgress), ev.BacklogItemPayload.OldStatus)
		assert.Equal(t, string(session.BacklogStatusDone), ev.BacklogItemPayload.NewStatus)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestTransitionBacklogItemStatus_should_notPublish_When_CASAffectsZeroRows
// simulates a lost CAS race (a stale expectedOldStatus) and asserts no event
// reaches the subscriber — the precondition-failed path must never publish
// (Task 2.1.1c, R4 error path).
func TestTransitionBacklogItemStatus_should_notPublish_When_CASAffectsZeroRows(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for lost-race no-publish test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	// Stale precondition: the item is actually "in_progress", not "review", so
	// the CAS WHERE clause matches zero rows.
	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, &session.BacklogItemPrecondition{
		ExpectedStatus: string(session.BacklogStatusReview),
	}, session.TriggeredBySystem)
	require.Error(t, err)
	require.ErrorIs(t, err, session.ErrPreconditionFailed)

	select {
	case ev := <-sub:
		t.Fatalf("expected no event on a failed CAS, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the timeout.
	}
}

// TestTransitionBacklogItemStatus_should_returnSuccessAndPersistRow_When_ItemChangePublisherPanics
// wires a deliberately panicking ItemChangePublisher into a real ent-backed
// repository and confirms the panic is contained entirely within the publish
// step: the transition still returns success (no panic reaches this test
// goroutine) and the row is actually persisted (re-fetched and checked)
// (Task 2.1.1d, R4 integration — proves the panic-recovery guarantee added at
// the repository call site protects this real hooked method end-to-end, even
// against a raw ItemChangePublisher implementation that has no recover() of
// its own).
func TestTransitionBacklogItemStatus_should_returnSuccessAndPersistRow_When_ItemChangePublisherPanics(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	repo.SetItemChangePublisher(panickingItemChangePublisher{})

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for panicking publisher regression test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		updated, transErr := repo.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, &session.BacklogItemPrecondition{
			ExpectedStatus: string(session.BacklogStatusInProgress),
		}, session.TriggeredBySystem)
		require.NoError(t, transErr)
		require.NotNil(t, updated)
	})

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), fetched.Status)
}

// TestUpdateBacklogItem_should_publishChangedFieldNames_When_TitleIsUpdated
// updates only the Title field and asserts the published event's
// UpdatedFields contains exactly "title" (Task 2.2.1b, R5 happy path).
func TestUpdateBacklogItem_should_publishChangedFieldNames_When_TitleIsUpdated(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Old title",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	newTitle := "New title"
	_, err = repo.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{Title: &newTitle}, nil)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.Equal(t, pkgevents.EventBacklogItemChanged, ev.Type)
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Equal(t, []string{"title"}, ev.BacklogItemPayload.UpdatedFields)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestUpdateBacklogItem_should_notPublishMisleadingFieldList_When_NoParamsAreSet
// calls UpdateBacklogItem with every field left nil (a no-op update) and
// asserts the diff detector never fabricates a field name — the published
// UpdatedFields list (if an event is published at all) must be empty
// (Task 2.2.1a, R5 error/edge path).
func TestUpdateBacklogItem_should_notPublishMisleadingFieldList_When_NoParamsAreSet(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for no-op update test",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = repo.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{}, nil)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		assert.Empty(t, ev.BacklogItemPayload.UpdatedFields)
	case <-time.After(200 * time.Millisecond):
		// Also acceptable: no event published at all for a no-op update.
	}
}

// TestUpdateBacklogItem_should_deliverEventThroughRealBus_When_MultipleFieldsChange
// updates several fields together and asserts one event carries all their
// names in UpdatedFields plus the full updated BacklogItem snapshot (R5
// integration). Uses Title+Description+Notes rather than plan.md's literal
// "planText" example — there is no PlanText field on BacklogItemUpdate in this
// codebase, so Notes stands in as the third real field.
func TestUpdateBacklogItem_should_deliverEventThroughRealBus_When_MultipleFieldsChange(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Old title",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	newTitle := "New title"
	newDescription := "New description"
	newNotes := "New notes"
	_, err = repo.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{
		Title:       &newTitle,
		Description: &newDescription,
		Notes:       &newNotes,
	}, nil)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.ElementsMatch(t, []string{"title", "description", "notes"}, ev.BacklogItemPayload.UpdatedFields)
		require.NotNil(t, ev.BacklogItemPayload.Item)
		assert.Equal(t, newTitle, ev.BacklogItemPayload.Item.Title)
		assert.Equal(t, newDescription, ev.BacklogItemPayload.Item.Description)
		assert.Equal(t, newNotes, ev.BacklogItemPayload.Item.Notes)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestArchiveBacklogItem_should_publishArchivedAtTimestamp_When_DoneItemIsArchived
// archives a done item and asserts the published event carries a non-nil
// ArchivedAt timestamp (Task 2.2.2a/c, R6 happy path).
func TestArchiveBacklogItem_should_publishArchivedAtTimestamp_When_DoneItemIsArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item to archive",
		Status: string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	_, err = repo.ArchiveBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemArchived, ev.BacklogItemPayload.Kind)
		require.NotNil(t, ev.BacklogItemPayload.ArchivedAt)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestUnarchiveBacklogItem_should_returnNotFound_When_ItemIDDoesNotExist confirms
// an unarchive against a nonexistent item id returns ErrNotFound and never
// reaches the publish call, mirroring TestDeleteBacklogItem's not-found case
// (backlog item "Archiving a backlog item is irreversible", criterion 3).
func TestUnarchiveBacklogItem_should_returnNotFound_When_ItemIDDoesNotExist(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	_, err := repo.UnarchiveBacklogItem(ctx, uuid.NewString())
	require.Error(t, err)
	require.ErrorIs(t, err, session.ErrNotFound)

	select {
	case ev := <-sub:
		t.Fatalf("expected no event when unarchiving a nonexistent item, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the timeout.
	}
}

// TestUnarchiveBacklogItem_should_restoreToIdeaAndPublishTransition_When_ItemIsArchived
// covers the documented happy path: an archived item's archived_at is cleared,
// its status is restored to "idea", and a full ChangeStatusTransition event
// (not the lightweight ChangeItemArchived shape) is published so live list
// views update without a manual refresh (criteria 2 and 5).
func TestUnarchiveBacklogItem_should_restoreToIdeaAndPublishTransition_When_ItemIsArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item to unarchive",
		Status: string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	_, err = repo.ArchiveBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	unarchived, err := repo.UnarchiveBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusIdea), unarchived.Status)
	assert.Nil(t, unarchived.ArchivedAt)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeStatusTransition, ev.BacklogItemPayload.Kind)
		assert.Equal(t, string(session.BacklogStatusArchived), ev.BacklogItemPayload.OldStatus)
		assert.Equal(t, string(session.BacklogStatusIdea), ev.BacklogItemPayload.NewStatus)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestUnarchiveBacklogItem_should_unconditionallyFlipToIdea_When_ItemIsNotArchived
// documents and locks in the idempotency decision recorded on
// EntRepository.UnarchiveBacklogItem: calling unarchive on an item that is not
// currently archived still succeeds and unconditionally flips it to "idea",
// matching UnarchiveSession's precedent (server/services/session_service.go)
// rather than erroring or no-op'ing (criterion 3).
func TestUnarchiveBacklogItem_should_unconditionallyFlipToIdea_When_ItemIsNotArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item never archived",
		Status: string(session.BacklogStatusReady),
	})
	require.NoError(t, err)

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	unarchived, err := repo.UnarchiveBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusIdea), unarchived.Status)
	assert.Nil(t, unarchived.ArchivedAt)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeStatusTransition, ev.BacklogItemPayload.Kind)
		assert.Equal(t, string(session.BacklogStatusReady), ev.BacklogItemPayload.OldStatus)
		assert.Equal(t, string(session.BacklogStatusIdea), ev.BacklogItemPayload.NewStatus)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestDeleteBacklogItem_should_notPublish_When_ItemIDDoesNotExist confirms a
// delete against a nonexistent item id returns its existing not-found error
// and never reaches the publish call (Task 2.2.2b, R6 error path).
func TestDeleteBacklogItem_should_notPublish_When_ItemIDDoesNotExist(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	err := repo.DeleteBacklogItem(ctx, uuid.NewString())
	require.Error(t, err)
	require.ErrorIs(t, err, session.ErrNotFound)

	select {
	case ev := <-sub:
		t.Fatalf("expected no event when deleting a nonexistent item, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the timeout.
	}
}

// TestDeleteBacklogItem_should_publishRemovedNotUpdated_When_ExistingItemIsDeleted
// deletes an existing item and asserts the subscriber receives
// ChangeItemRemoved, not ChangeItemUpdated — confirming the downstream RPC
// handler will route this to a BacklogItemRemovedEvent (a delete signal), not
// an upsert (Task 2.2.2c, R6 integration).
func TestDeleteBacklogItem_should_publishRemovedNotUpdated_When_ExistingItemIsDeleted(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item to delete",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	err = repo.DeleteBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemRemoved, ev.BacklogItemPayload.Kind)
		assert.NotEqual(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestTransitionBacklogItemStatus_should_embedNonEmptyItemSessions_When_ItemHasReviewVerdict
// is the regression test for the Phase 5 spec-compliance sweep's confirmed
// blanking bug: backlogItemToData never copied the item_sessions edge, and
// none of the ~10 publish-hook call sites (including TransitionBacklogItemStatus,
// exercised here) eager-loaded it before publishing — so every live
// BacklogItemEvent shipped an empty ItemSessions slice even when the item had
// real linked sessions/verdicts, silently blanking gateVerdict/
// gateVerdictSummary/triageStatus on every frontend consumer (which derives
// them entirely from itemSessions, see web-app/src/lib/hooks/
// useBacklogService.ts's mapBacklogItem) — actively worse than the
// pre-event-driven REST-poll baseline that always fetched the full item.
// This proves attachItemSessionsForPublish (session/ent_repository_backlog.go)
// actually closes the gap: the item has a real review-role ItemSession with a
// saved verdict, and the event published by a subsequent
// TransitionBacklogItemStatus call — a totally different code path from the
// one that created the session/verdict — must still carry that session (with
// its verdict inline) in Item.ItemSessions.
func TestTransitionBacklogItemStatus_should_embedNonEmptyItemSessions_When_ItemHasReviewVerdict(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for embedded-itemSessions regression test",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "review",
	})
	require.NoError(t, err)

	// Drain CreateItemSession's own event — it is a different call site from
	// the one under test below.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	err = repo.SaveReviewVerdict(ctx, is.ID, session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
		Summary:        "looks good",
	})
	require.NoError(t, err)

	// Drain SaveReviewVerdict's own event too — the assertion below exercises
	// TransitionBacklogItemStatus specifically, a completely separate publish
	// call site, to prove the fix isn't accidentally scoped to just one hook.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SaveReviewVerdict's own BacklogItemChanged event")
	}

	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		require.NotNil(t, ev.BacklogItemPayload.Item)
		require.Len(t, ev.BacklogItemPayload.Item.ItemSessions, 1,
			"embedded item snapshot must carry the item's real ItemSessions, not an empty slice (the blanking regression)")
		embedded := ev.BacklogItemPayload.Item.ItemSessions[0]
		assert.Equal(t, is.ID, embedded.ID)
		assert.Equal(t, "review", embedded.Role)
		require.NotNil(t, embedded.ReviewVerdict, "embedded session's ReviewVerdict must be populated, not blanked")
		assert.Equal(t, string(session.ReviewOutcomePass), embedded.ReviewVerdict.OverallOutcome)
		assert.Equal(t, "looks good", embedded.ReviewVerdict.Summary)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from TransitionBacklogItemStatus")
	}
}

// TestUpdateAcCriterionStatus_should_publishItemUpdatedEvent_When_CriterionStatusChanges
// is the regression test for the second Phase 5 sweep finding: UpdateAcCriterionStatus
// (used for the "N/M done" acceptance-criteria progress badge) mutated AC status
// with zero publish call at all — a real event-system bypass, not just a blanked
// field. Asserts a subscriber now receives a ChangeItemUpdated event carrying the
// updated acceptance criteria JSON after a criterion's status changes.
func TestUpdateAcCriterionStatus_should_publishItemUpdatedEvent_When_CriterionStatusChanges(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	ac, err := session.SerializeAcCriteria([]session.AcCriterion{
		{Index: 0, Text: "First criterion", Status: session.AcStatusPending},
		{Index: 1, Text: "Second criterion", Status: session.AcStatusPending},
	})
	require.NoError(t, err)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:              "item for AC-status publish regression test",
		Status:             string(session.BacklogStatusInProgress),
		AcceptanceCriteria: ac,
	})
	require.NoError(t, err)

	err = repo.UpdateAcCriterionStatus(ctx, item.ID, 0, string(session.AcStatusDone), "")
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "acceptanceCriteria")
		require.NotNil(t, ev.BacklogItemPayload.Item)
		criteria, parseErr := session.ParseAcCriteria(ev.BacklogItemPayload.Item.AcceptanceCriteria)
		require.NoError(t, parseErr)
		require.Len(t, criteria, 2)
		assert.Equal(t, session.AcStatusDone, criteria[0].Status)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from UpdateAcCriterionStatus")
	}
}

// TestReconcileStuckItems_should_publishStatusChangedEvent_When_ItemTransitionsToReview
// is a sweep-discovered fix (Phase 5 spec-compliance sweep, backlog-event-driven-updates):
// ReconcileStuckItems mutates status via a raw ent transaction (session/storage_backlog.go)
// rather than going through TransitionBacklogItemStatus, so it originally bypassed the
// publish hook entirely — exactly the "missed call site" failure mode requirements.md's
// Feasibility Risks section warns about for internal reconcilers that touch storage
// directly (mirroring reconcileBouncingItems/ReconcilePRPending, which already go through
// TransitionBacklogItemStatus and were unaffected). This asserts the fix: a subscriber
// receives a BacklogItemStatusChangedEvent (in_progress -> review) after the sweep.
func TestReconcileStuckItems_should_publishStatusChangedEvent_When_ItemTransitionsToReview(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "stuck item for reconcile publish test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)

	// Drain the session-attach event published by CreateItemSession itself so
	// it doesn't get mistaken for ReconcileStuckItems' own event below.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	require.NoError(t, repo.UpdateItemSessionEnded(ctx, is.ID, time.Now().Add(-5*time.Minute)))

	count, err := repo.ReconcileStuckItems(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeStatusTransition, ev.BacklogItemPayload.Kind)
		assert.Equal(t, string(session.BacklogStatusInProgress), ev.BacklogItemPayload.OldStatus)
		assert.Equal(t, string(session.BacklogStatusReview), ev.BacklogItemPayload.NewStatus)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from ReconcileStuckItems")
	}
}

// TestUpdateItemSessionStarted_should_publishItemUpdatedEvent_When_StartedAtIsSet
// is a regression test for the Phase 5 spec-compliance sweep's follow-up pass:
// UpdateItemSessionStarted mutated started_at on an ItemSession with zero
// publish call at all — a real event-system bypass, exactly the same shape as
// the already-fixed UpdateAcCriterionStatus/UpdateItemSessionTriageResult
// bypasses. Asserts a subscriber now receives a ChangeItemUpdated event after
// the started_at write.
func TestUpdateItemSessionStarted_should_publishItemUpdatedEvent_When_StartedAtIsSet(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for UpdateItemSessionStarted publish regression test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)

	// Drain CreateItemSession's own event — it is a different call site from
	// the one under test below.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	err = repo.UpdateItemSessionStarted(ctx, is.ID, time.Now())
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "itemSessions")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from UpdateItemSessionStarted")
	}
}

// TestUpdateItemSessionGitActivity_should_publishItemUpdatedEvent_When_CommitIsRecorded
// is the same-shaped regression test for UpdateItemSessionGitActivity, which
// recorded commit sha/message/count with zero publish call.
func TestUpdateItemSessionGitActivity_should_publishItemUpdatedEvent_When_CommitIsRecorded(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for UpdateItemSessionGitActivity publish regression test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)

	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	err = repo.UpdateItemSessionGitActivity(ctx, is.ID, "abc1234", "commit message", time.Now(), 1)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "itemSessions")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from UpdateItemSessionGitActivity")
	}
}

// TestUpdateItemSessionFileTouch_should_publishItemUpdatedEvent_When_FileTouchIsRecorded
// is the same-shaped regression test for UpdateItemSessionFileTouch, which
// recorded the last-file-touch timestamp with zero publish call.
func TestUpdateItemSessionFileTouch_should_publishItemUpdatedEvent_When_FileTouchIsRecorded(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for UpdateItemSessionFileTouch publish regression test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)

	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	err = repo.UpdateItemSessionFileTouch(ctx, is.ID, time.Now())
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "itemSessions")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from UpdateItemSessionFileTouch")
	}
}

// TestUpdateItemSessionVerificationNotes_should_publishItemUpdatedEvent_When_NotesAreSaved
// is the same-shaped regression test for UpdateItemSessionVerificationNotes,
// which saved request_review's verification evidence with zero publish call.
func TestUpdateItemSessionVerificationNotes_should_publishItemUpdatedEvent_When_NotesAreSaved(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for UpdateItemSessionVerificationNotes publish regression test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "review",
	})
	require.NoError(t, err)

	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's own BacklogItemChanged event")
	}

	err = repo.UpdateItemSessionVerificationNotes(ctx, is.ID, "ran make ci; verified manually in browser")
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "itemSessions")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from UpdateItemSessionVerificationNotes")
	}
}

// TestBackfillMissingPRNumbers_should_publishItemUpdatedEvent_When_PrNumberIsBackfilled
// is the same-shaped regression test for BackfillMissingPRNumbers, which
// persisted a parsed pr_number with zero publish call, so a live viewer never
// saw a pr_pending item become visible to FindPRPendingItems' polling loop
// until their next full poll/refresh.
func TestBackfillMissingPRNumbers_should_publishItemUpdatedEvent_When_PrNumberIsBackfilled(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:              "item for BackfillMissingPRNumbers publish regression test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(session.BacklogStatusPRPending),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	prURL := "https://github.com/tstapler/stapler-squad/pull/148"
	_, err = repo.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{PrURL: &prURL}, nil)
	require.NoError(t, err)

	// Drain UpdateBacklogItem's own event — it is a different call site from
	// the one under test below.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for UpdateBacklogItem's own BacklogItemChanged event")
	}

	n, err := repo.BackfillMissingPRNumbers(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeItemUpdated, ev.BacklogItemPayload.Kind)
		assert.Contains(t, ev.BacklogItemPayload.UpdatedFields, "prNumber")
		require.NotNil(t, ev.BacklogItemPayload.Item)
		assert.Equal(t, 148, ev.BacklogItemPayload.Item.PrNumber)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from BackfillMissingPRNumbers")
	}
}

// TestAppendActivityNote_should_PublishActivityNoteAddedEvent_When_Called
// (Epic 8.2, Task 8.2.1b) lives in this file (package session_test), not
// session/ent_repository_backlog_test.go as plan.md's Story 8.2.1 literally
// names — a real ItemChangePublisher assertion needs the
// server/services.BacklogItemEventPublisher adapter, which imports session,
// so it can only be exercised from this external test package (see this
// file's own header comment for the import-cycle reasoning). Mirrors
// TestUpdateAcCriterionStatus_should_publishItemUpdatedEvent_When_CriterionStatusChanges's
// shape above.
func TestAppendActivityNote_should_PublishActivityNoteAddedEvent_When_Called(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for activity-note publish test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "session-uuid-1", "worker-1", "note from a session"))

	select {
	case ev := <-sub:
		require.Equal(t, pkgevents.EventBacklogItemChanged, ev.Type)
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeActivityNoteAdded, ev.BacklogItemPayload.Kind)
		require.NotNil(t, ev.BacklogItemPayload.ActivityNote)
		assert.Equal(t, "note from a session", ev.BacklogItemPayload.ActivityNote.Message)
		assert.Equal(t, "session-uuid-1", ev.BacklogItemPayload.ActivityNote.AuthorSessionUUID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from AppendActivityNote")
	}
}

// TestAppendActivityNote_should_PopulateStatusAndRepoPathOnPublishedSnapshot_When_Called
// (Epic 8.2, Task 8.2.1c) is Blocker 2's regression test: without the
// Select(Status, RepoPath).Only(ctx) read AppendActivityNote does before
// publishing, backlogItemMatchesFilters
// (server/services/backlog_service_events.go) would silently drop this event
// for any WatchBacklogItems caller with a non-empty status_filter/
// category_filter. Same file-placement reasoning as the test above.
func TestAppendActivityNote_should_PopulateStatusAndRepoPathOnPublishedSnapshot_When_Called(t *testing.T) {
	t.Parallel()
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "item with known status and repo path",
		Status:   string(session.BacklogStatusReview),
		RepoPath: "/repo/known-path",
	})
	require.NoError(t, err)

	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "", "", "a note"))

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		require.NotNil(t, ev.BacklogItemPayload.Item, "the published snapshot must carry a non-nil Item so backlogItemMatchesFilters doesn't unconditionally reject it")
		assert.Equal(t, string(session.BacklogStatusReview), ev.BacklogItemPayload.Item.Status, "Status must be populated on the snapshot, not left zero-value")
		assert.Equal(t, "/repo/known-path", ev.BacklogItemPayload.Item.RepoPath, "RepoPath must be populated on the snapshot, not left zero-value")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event from AppendActivityNote")
	}
}
