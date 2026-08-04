package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// Manual escape-hatch tests (project 7a383b3b — "manual escape hatch:
// associate a PR / override status on a backlog item by hand"):
//   - UpdateBacklogItem's pr_url/pr_number handling (both-or-neither, URL
//     parse/cross-check, review-status scoping, success notify)
//   - TransitionBacklogItemStatus's OverrideReason-gated success notify
//   - OverrideVerdict's CAS precondition fix (was previously a hardcoded nil)

func ptrStr(s string) *string { return &s }
func ptrI32(i int32) *int32   { return &i }

// ─── UpdateBacklogItem: pr_url/pr_number manual association ──────────────────

func TestUpdateBacklogItem_should_RejectPrUrlWithoutPrNumber_When_OnlyOneFieldSet(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{Title: "item"}))
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: created.Msg.Item.Id,
		PrUrl:  ptrStr("https://github.com/acme/widgets/pull/42"),
		// PrNumber deliberately omitted.
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestUpdateBacklogItem_should_RejectPrNumberWithoutPrUrl_When_OnlyOneFieldSet(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{Title: "item"}))
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   created.Msg.Item.Id,
		PrNumber: ptrI32(42),
		// PrUrl deliberately omitted.
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestUpdateBacklogItem_should_RejectPrAssociation_When_UrlIsNotAGitHubPRUrl(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(ctx, connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   item.ID,
		PrUrl:    ptrStr("not a url at all"),
		PrNumber: ptrI32(42),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestUpdateBacklogItem_should_RejectPrAssociation_When_UrlAndNumberDisagree(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(ctx, connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   item.ID,
		PrUrl:    ptrStr("https://github.com/acme/widgets/pull/42"),
		PrNumber: ptrI32(43), // disagrees with the URL's embedded #42
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	// The item must be untouched — a validation failure must not corrupt state.
	fetched, getErr := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, getErr)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status)
	assert.Empty(t, fetched.PrURL)
}

// TestUpdateBacklogItem_should_RejectPrAssociation_When_ItemNotInReviewStatus is
// AC4's regression guard: SetBacklogItemPRAndTransition (session/storage.go)
// hardcodes ExpectedStatus=review as a deliberate v1 scope limitation — an
// item in any other status must get a clear, distinguishable error, not a
// generic CAS-failure message and not a silent success.
func TestUpdateBacklogItem_should_RejectPrAssociation_When_ItemNotInReviewStatus(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(ctx, connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   item.ID,
		PrUrl:    ptrStr("https://github.com/acme/widgets/pull/42"),
		PrNumber: ptrI32(42),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "not in review status", "must be distinguishable from a generic CAS-failure message")

	fetched, getErr := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, getErr)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status, "must not silently succeed or corrupt state")
	assert.Empty(t, fetched.PrURL)
}

// TestUpdateBacklogItem_should_AssociatePRAndNotify_When_ItemInReviewStatus is
// AC2/AC9's happy-path proof: an operator can associate an existing PR with a
// review-status item with no live linked session, the write goes through
// SetBacklogItemPRAndTransition (review -> pr_pending), and a success
// notification is published (previously only failure paths notified).
func TestUpdateBacklogItem_should_AssociatePRAndNotify_When_ItemInReviewStatus(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "item awaiting review", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	resp, err := svc.UpdateBacklogItem(ctx, connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   item.ID,
		PrUrl:    ptrStr("https://github.com/acme/widgets/pull/42"),
		PrNumber: ptrI32(42),
	}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), resp.Msg.Item.Status, "must transition review -> pr_pending atomically")
	assert.Equal(t, "https://github.com/acme/widgets/pull/42", resp.Msg.Item.PrUrl)
	assert.Equal(t, int32(42), resp.Msg.Item.PrNumber)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, item.ID, ev.SessionID, "notification must be scoped to this item")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a manual-override success notification, got none")
	}
}

// ─── TransitionBacklogItemStatus: OverrideReason-gated success notify ────────

// TestTransitionBacklogItemStatus_should_NotifyAndAppendNote_When_OverrideReasonProvided
// covers AC7/AC9: a non-empty OverrideReason is this repo's signal that the
// caller is the manual "operator override" control, not a routine automated
// button — that path must publish a visible notification and progress note.
func TestTransitionBacklogItemStatus_should_NotifyAndAppendNote_When_OverrideReasonProvided(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stuck item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	resp, err := svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:         item.ID,
		TargetStatus:   string(session.BacklogStatusInProgress),
		OverrideReason: "recovering a wedged item — automation stopped responding",
	}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), resp.Msg.Item.Status)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a manual-override success notification when override_reason is set, got none")
	}

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotEmpty(t, fetched.ProgressNotes, "manual override must leave a visible audit trail, not a silent write")
	last := fetched.ProgressNotes[len(fetched.ProgressNotes)-1]
	assert.Equal(t, "manual_override", last.Status)
}

// TestTransitionBacklogItemStatus_should_NotNotify_When_OverrideReasonEmpty is
// the negative-space guard: every pre-existing (non-manual-override) caller of
// this RPC passes an empty reason and must stay exactly as quiet as before —
// this feature must not turn every routine automated-button transition into a
// notification.
func TestTransitionBacklogItemStatus_should_NotNotify_When_OverrideReasonEmpty(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "routine item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: string(session.BacklogStatusInProgress),
	}))
	require.NoError(t, err)

	select {
	case ev := <-ch:
		t.Fatalf("expected no notification for a routine transition with no override_reason, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the window.
	}
}

// ─── OverrideVerdict: CAS precondition fix (was previously a hardcoded nil) ──

// TestOverrideVerdict_should_RecordExactlyOneStatusTransition_When_TwoOverridesRaceConcurrently
// is the regression test for AC10 / the collateral fix to OverrideVerdict's
// nil CAS precondition (server/services/backlog_service_lifecycle.go). Before
// the fix, OverrideVerdict's own call to storage.TransitionBacklogItemStatus
// passed a nil precondition — unlike every sibling RPC handler in this file —
// which meant its write applied UNCONDITIONALLY regardless of the item's
// current status (see EntRepository.TransitionBacklogItemStatus: a nil
// precondition adds no WHERE clause beyond the row ID, so `affected` is never
// 0 and a BacklogStatusEvent audit row is recorded on every single call, win
// or not). Two OverrideVerdict calls targeting the same backlog item from two
// different item sessions, racing concurrently, must result in exactly ONE
// recorded review -> done transition event — not two — proving the second
// writer's write is now genuinely CAS-guarded instead of blindly re-applied.
func TestOverrideVerdict_should_RecordExactlyOneStatusTransition_When_TwoOverridesRaceConcurrently(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "racing overrides item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	is1, err := storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item.ID, SessionUUID: "override-race-a", SessionRole: session.SessionRoleReview})
	require.NoError(t, err)
	is2, err := storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item.ID, SessionUUID: "override-race-b", SessionRole: session.SessionRoleReview})
	require.NoError(t, err)

	var wg sync.WaitGroup
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)

	race := func(itemSessionID string) {
		defer wg.Done()
		startBarrier.Wait()
		_, _ = svc.OverrideVerdict(ctx, connect.NewRequest(&sessionv1.OverrideVerdictRequest{
			ItemSessionId:  itemSessionID,
			OverrideReason: "racing manual override",
			ToStatus:       string(session.BacklogStatusDone),
		}))
	}

	wg.Add(2)
	go race(is1.ID)
	go race(is2.ID)
	startBarrier.Done()
	wg.Wait()

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), fetched.Status, "the item must land in done regardless of which writer won")

	reviewToDone := 0
	for _, ev := range fetched.StatusEvents {
		if ev.FromStatus == string(session.BacklogStatusReview) && ev.ToStatus == string(session.BacklogStatusDone) {
			reviewToDone++
		}
	}
	assert.LessOrEqual(t, reviewToDone, 1,
		"exactly one writer's transition may be recorded — a nil CAS precondition would let both land unconditionally")
}
