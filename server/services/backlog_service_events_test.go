package services

// backlog_service_events_test.go — proto shape tests for BacklogItemEvent
// (Story 1.1.1) plus the full WatchBacklogItems handler test suite (Epic
// 3.2, project_plans/backlog-event-driven-updates/implementation/plan.md).
//
// The handler tests drive svc.watchBacklogItems directly through the
// backlogItemEventSender interface (Task 3.1.1a) rather than a fake
// connect.ServerStream[T] — see backlog_service_events.go's doc comment for
// why the latter is not constructible against this repo's vendored
// connectrpc.com/connect v1.19.0.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

func TestBacklogItemEvent_should_exposeCorrectOneofVariant_When_StatusChangedIsSet(t *testing.T) {
	t.Parallel()
	event := &sessionv1.BacklogItemEvent{
		Event: &sessionv1.BacklogItemEvent_StatusChanged{
			StatusChanged: &sessionv1.BacklogItemStatusChangedEvent{
				ItemId:    "abc123",
				OldStatus: "in_progress",
				NewStatus: "review",
			},
		},
	}

	if got := event.GetStatusChanged().GetNewStatus(); got != "review" {
		t.Errorf("GetStatusChanged().GetNewStatus() = %q, want %q", got, "review")
	}
	if got := event.GetVerdictRecorded(); got != nil {
		t.Errorf("GetVerdictRecorded() = %v, want nil", got)
	}
}

func TestBacklogItemEvent_should_returnNilForUnsetOneofVariant_When_NoVariantIsSet(t *testing.T) {
	t.Parallel()
	event := &sessionv1.BacklogItemEvent{}

	if got := event.GetStatusChanged(); got != nil {
		t.Errorf("GetStatusChanged() = %v, want nil", got)
	}
	if got := event.GetVerdictRecorded(); got != nil {
		t.Errorf("GetVerdictRecorded() = %v, want nil", got)
	}
	if got := event.GetSessionAttached(); got != nil {
		t.Errorf("GetSessionAttached() = %v, want nil", got)
	}
	if got := event.GetItemUpdated(); got != nil {
		t.Errorf("GetItemUpdated() = %v, want nil", got)
	}
	if got := event.GetItemArchived(); got != nil {
		t.Errorf("GetItemArchived() = %v, want nil", got)
	}
	if got := event.GetItemRemoved(); got != nil {
		t.Errorf("GetItemRemoved() = %v, want nil", got)
	}
}

// ─── fakeBacklogItemEventSender (Task 3.2.1a) ─────────────────────────────

// fakeBacklogItemEventSender is a hand-rolled fake implementing
// backlogItemEventSender, capturing every sent message in a mutex-guarded
// slice — several tests below run watchBacklogItems in a goroutine while
// concurrently publishing to the real event bus, so Send is called
// concurrently with Sent().
type fakeBacklogItemEventSender struct {
	mu   sync.Mutex
	sent []*sessionv1.BacklogItemEvent
}

func (f *fakeBacklogItemEventSender) Send(e *sessionv1.BacklogItemEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, e)
	return nil
}

// Sent returns a snapshot copy of the messages sent so far, safe to read
// concurrently with in-flight Send calls.
func (f *fakeBacklogItemEventSender) Sent() []*sessionv1.BacklogItemEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*sessionv1.BacklogItemEvent, len(f.sent))
	copy(out, f.sent)
	return out
}

// newTestBacklogServiceWithBus builds a BacklogService backed by a real
// *session.Storage (SQLite in a temp dir, via the package's shared
// createTestStorage helper) and a real *events.EventBus — watchBacklogItems
// is exercised end-to-end against genuine storage/bus behavior, only the
// sender at the RPC boundary is faked.
func newTestBacklogServiceWithBus(t *testing.T) (*BacklogService, *session.Storage, *events.EventBus) {
	t.Helper()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	return svc, storage, bus
}

// runWatchBacklogItems starts svc.watchBacklogItems in a goroutine and
// returns a channel that receives its error once ctx is canceled (or it
// otherwise returns). Callers must eventually cancel ctx and then receive
// from the returned channel (with a timeout) to confirm a clean return.
func runWatchBacklogItems(ctx context.Context, svc *BacklogService, msg *sessionv1.WatchBacklogItemsRequest, sender backlogItemEventSender) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- svc.watchBacklogItems(ctx, msg, sender)
	}()
	return done
}

// requireCleanReturn cancels cancel and asserts watchBacklogItems's
// goroutine returns nil within a bounded deadline (proving the live loop's
// ctx.Done() branch actually fires rather than hanging forever).
func requireCleanReturn(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watchBacklogItems did not return after context cancellation")
	}
}

// ─── Story 3.2.1: fresh-snapshot branch (Task 3.2.1b) ─────────────────────

func TestWatchBacklogItems_should_sendSnapshotEventsForAllItems_When_AfterSeqIsZero(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	item1, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Item 1", Status: string(session.BacklogStatusReady)})
	require.NoError(t, err)
	item2, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Item 2", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{}, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 2, "exactly one snapshot event per seeded item, no more")

	gotIDs := map[string]bool{}
	for _, ev := range sent {
		upd := ev.GetItemUpdated()
		require.NotNil(t, upd, "fresh-connection snapshot events are always the item_updated variant")
		assert.True(t, upd.GetIsSnapshot(), "every fresh-snapshot event must be is_snapshot: true")
		gotIDs[upd.GetItemId()] = true
	}
	assert.True(t, gotIDs[item1.ID], "missing snapshot event for item1")
	assert.True(t, gotIDs[item2.ID], "missing snapshot event for item2")
}

// ─── Story 3.2.1: after_seq replay branch (Task 3.2.1c) ───────────────────
//
// NOTE ON DEVIATION FROM THE LITERAL TASK 3.2.1c BULLET TEXT: plan.md's
// Task 3.2.1c bullet says to assert "zero snapshot events" for replayed
// events. That text predates the pre-mortem P2 #4 repair loop, which added
// Task 3.1.1c's unconditional forceIsSnapshot(...) call (see
// backlog_service_events.go) — every event delivered through this branch is
// now REQUIRED to have is_snapshot forced to true, per Story 3.1.1's own
// (later-added) acceptance criterion and the shipped Epic 3.1 code. This
// test asserts the actual, current, correct behavior (is_snapshot: true on
// every replayed event) rather than the stale bullet text.
func TestWatchBacklogItems_should_replayBufferedEventsInSeqOrder_When_AfterSeqIsGreaterThanZero(t *testing.T) {
	t.Parallel()
	svc, storage, bus := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Replay item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	// Baseline event establishes afterSeq; the client has already seen this one.
	baseline := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind: events.BacklogChangeStatusTransition,
		Item: item,
	})
	bus.Publish(baseline)
	afterSeq := baseline.Seq

	// 3 buffered events published (and originally NOT snapshot events) before
	// the client reconnects.
	for i := 0; i < 3; i++ {
		ev := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
			Kind:          events.BacklogChangeItemUpdated,
			Item:          item,
			UpdatedFields: []string{fmt.Sprintf("marker-%d", i)},
			IsSnapshot:    false,
		})
		bus.Publish(ev)
	}

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{AfterSeq: afterSeq}, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 3 }, 2*time.Second, 10*time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 3, "exactly the 3 events published after afterSeq")

	for i, ev := range sent {
		upd := ev.GetItemUpdated()
		require.NotNil(t, upd)
		assert.Equal(t, []string{fmt.Sprintf("marker-%d", i)}, upd.GetUpdatedFields(), "replayed events must arrive in ascending Seq order")
		assert.True(t, upd.GetIsSnapshot(), "every replayed event must be forced is_snapshot: true regardless of its original flag")
	}
}

// ─── Pre-mortem P2 #4: double-delivery race window ────────────────────────
//
// This is the safety-critical test for the resolved adversarial-review
// CONCERN #1 / pre-mortem P2 #4: an event published in the race window
// between Subscribe() and the after_seq branch's EventsSince() read can be
// delivered via BOTH the replay branch and the live fan-out loop. The fix
// (Task 3.1.1c) forces is_snapshot: true unconditionally on every
// replay-branch send, so even if the SAME event is also delivered live
// (with its original, un-forced flag), exactly one of the two copies is
// ever marked as a snapshot event.
//
// Reproducing this deterministically requires landing a Publish() call
// exactly between the handler's Subscribe() and EventsSince() calls — pure
// goroutine-scheduling races cannot do this reliably (confirmed: no
// existing precedent in this repo's WatchSessions tests, which don't cover
// this scenario at all). withTestAfterSubscribeHook (backlog_service_events.go)
// is a minimal test-only seam added specifically to make this race
// reproducible on every run rather than only occasionally.
func TestWatchBacklogItems_should_deliverRaceWindowEventExactlyOnceAsSnapshot_When_PublishedBetweenSubscribeAndEventsSince(t *testing.T) {
	t.Parallel()
	svc, storage, bus := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Race item", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)

	baseline := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind: events.BacklogChangeStatusTransition,
		Item: item,
	})
	bus.Publish(baseline)
	afterSeq := baseline.Seq

	const raceMarker = "race-marker-field"
	raceEvent := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:          events.BacklogChangeItemUpdated,
		Item:          item,
		UpdatedFields: []string{raceMarker},
		IsSnapshot:    false, // as originally published — must not be trusted verbatim by the replay branch
	})

	runCtx, cancel := context.WithCancel(withTestAfterSubscribeHook(ctx, func() {
		bus.Publish(raceEvent)
	}))
	sender := &fakeBacklogItemEventSender{}
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{AfterSeq: afterSeq}, sender)

	// The race event, published right after Subscribe() registered the
	// handler's channel, must reach the sender via BOTH the replay branch
	// (buffer already contained it by the time EventsSince ran) and the live
	// fan-out loop (it was fanned out live at publish time) — i.e. 2 total
	// deliveries.
	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	requireCleanReturn(t, cancel, done)

	var matching []*sessionv1.BacklogItemEvent
	for _, ev := range sender.Sent() {
		if upd := ev.GetItemUpdated(); upd != nil && len(upd.GetUpdatedFields()) == 1 && upd.GetUpdatedFields()[0] == raceMarker {
			matching = append(matching, ev)
		}
	}

	require.Len(t, matching, 2, "the race-window event must be delivered exactly twice: once via replay, once via live fan-out")

	snapshotCount := 0
	for _, ev := range matching {
		if ev.GetItemUpdated().GetIsSnapshot() {
			snapshotCount++
		}
	}
	assert.Equal(t, 1, snapshotCount, "the race-window event must be marked is_snapshot exactly once (the forced replay copy) — never twice, never zero times")
}

// ─── Live fan-out (no filters) ─────────────────────────────────────────────

func TestWatchBacklogItems_should_forwardLiveEvent_When_PublishedWhileStreamIsLive(t *testing.T) {
	t.Parallel()
	svc, storage, bus := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Live item", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{}, sender)

	// Wait for the initial (1-item) snapshot before publishing live, so the
	// live event is unambiguously the 2nd message.
	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return bus.SubscriberCount() >= 1 }, 2*time.Second, 10*time.Millisecond)

	liveEvent := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:      events.BacklogChangeStatusTransition,
		Item:      item,
		OldStatus: "in_progress",
		NewStatus: "review",
	})
	bus.Publish(liveEvent)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 2)
	sc := sent[1].GetStatusChanged()
	require.NotNil(t, sc)
	assert.Equal(t, item.ID, sc.GetItemId())
	assert.Equal(t, "review", sc.GetNewStatus())
	assert.False(t, sc.GetIsSnapshot(), "a genuinely live event must not be marked as a snapshot")
}

// ─── Filters (status_filter / category_filter) ────────────────────────────

func TestWatchBacklogItems_should_excludeNonMatchingItems_When_StatusFilterAppliedToSnapshot(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	matching, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "In progress item", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Done item", Status: string(session.BacklogStatusDone)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{StatusFilter: []string{string(session.BacklogStatusInProgress)}}, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	// Give a brief window to make sure the filtered-out item's event doesn't
	// also arrive before we cancel.
	time.Sleep(50 * time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 1, "only the status-matching item should be sent")
	assert.Equal(t, matching.ID, sent[0].GetItemUpdated().GetItemId())
}

// TestWatchBacklogItems_should_returnEmptySnapshot_When_StatusFilterMatchesNoItems
// closes the Epic 7.4 (plan.md) / validation.md R9 error-path coverage gap:
// the existing filter tests above only exercise a filter matching SOME of N
// seeded items (1-of-2). This test covers the zero-match edge explicitly —
// an over-restrictive filter must degrade to an empty (not broken) stream,
// with no error/panic and a clean shutdown on cancel.
//
// NOTE ON DEVIATION FROM THE ORIGINAL ASSERTION: this test originally
// asserted `assert.Empty(t, sender.Sent(), ...)` — literally zero bytes on
// the wire for a zero-match connection. That is precisely the hang bug found
// during the e2e pass (see backlog_service_events.go's watchBacklogItems doc
// comment on the snapshot-complete marker): a genuinely empty send leaves
// the client's `for await` loop parked forever, never reaching
// connectionState "live". The handler now always sends exactly one
// snapshot_complete marker in this case so the client has *something* to
// unblock on; this test asserts that corrected behavior instead.
func TestWatchBacklogItems_should_returnEmptySnapshot_When_StatusFilterMatchesNoItems(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	_, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "In progress item", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Done item", Status: string(session.BacklogStatusDone)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{StatusFilter: []string{string(session.BacklogStatusArchived)}}, sender)

	// No seeded item is "archived", so the fresh-snapshot branch sends zero
	// real item events — but the handler must still send exactly one
	// snapshot_complete marker so the client isn't left hanging.
	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 1, "a status filter matching no items must produce exactly one snapshot-complete marker, no item events, no error/panic")
	assert.NotNil(t, sent[0].GetSnapshotComplete(), "the sole message must be the synthetic snapshot-complete marker")
}

// TestWatchBacklogItems_should_sendSnapshotCompleteMarker_When_BacklogIsGenuinelyEmpty
// covers the exact scenario the e2e pass flagged: no filter at all, zero
// backlog items in storage (not just zero matching a filter) — the fresh
// connection branch's ListBacklogItems call itself returns an empty slice.
// Proves the fix within a bounded time (not "hangs forever"): the handler
// sends the marker promptly and the stream is otherwise indistinguishable
// from a healthy connection with real items.
func TestWatchBacklogItems_should_sendSnapshotCompleteMarker_When_BacklogIsGenuinelyEmpty(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{}, sender)

	// Bounded wait, not "hangs forever": the whole point of this test is that
	// a genuinely empty backlog still produces a message promptly.
	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond,
		"a genuinely empty backlog must still send a snapshot-complete marker promptly, not hang")
	time.Sleep(50 * time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	assert.NotNil(t, sent[0].GetSnapshotComplete())
}

func TestWatchBacklogItems_should_excludeNonMatchingItems_When_CategoryFilterAppliedToSnapshot(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	matching, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Repo A item", Status: string(session.BacklogStatusReady), RepoPath: "/repo/a"})
	require.NoError(t, err)
	_, err = storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Repo B item", Status: string(session.BacklogStatusReady), RepoPath: "/repo/b"})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{CategoryFilter: []string{"/repo/a"}}, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 1, "category_filter is matched against RepoPath — see backlogItemMatchesFilters's doc comment")
	assert.Equal(t, matching.ID, sent[0].GetItemUpdated().GetItemId())
}

func TestWatchBacklogItems_should_onlyForwardMatchingLiveEvents_When_StatusFilterIsSet(t *testing.T) {
	t.Parallel()
	svc, storage, bus := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	matchItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Matches filter", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	otherItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Does not match", Status: string(session.BacklogStatusDone)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{StatusFilter: []string{string(session.BacklogStatusInProgress)}}, sender)

	// Wait for the (1-item, filtered) snapshot before publishing live events.
	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return bus.SubscriberCount() >= 1 }, 2*time.Second, 10*time.Millisecond)

	nonMatching := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:      events.BacklogChangeStatusTransition,
		Item:      otherItem,
		OldStatus: "review",
		NewStatus: "done",
	})
	bus.Publish(nonMatching)

	matchingEvt := events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:      events.BacklogChangeStatusTransition,
		Item:      matchItem,
		OldStatus: "ready",
		NewStatus: "in_progress",
	})
	bus.Publish(matchingEvt)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	// Extra grace window so a wrongly-unfiltered nonMatching send would have
	// time to show up as a 3rd message before we cancel.
	time.Sleep(50 * time.Millisecond)
	requireCleanReturn(t, cancel, done)

	sent := sender.Sent()
	require.Len(t, sent, 2, "1 snapshot + 1 live matching event; the non-matching live event must never be sent")
	sc := sent[1].GetStatusChanged()
	require.NotNil(t, sc)
	assert.Equal(t, matchItem.ID, sc.GetItemId())
}

// ─── Degraded mode: nil eventBus ───────────────────────────────────────────

func TestWatchBacklogItems_should_returnUnimplemented_When_EventBusIsNil(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil) // eventBus left nil (no SetEventBus call)

	sender := &fakeBacklogItemEventSender{}
	err := svc.watchBacklogItems(context.Background(), &sessionv1.WatchBacklogItemsRequest{}, sender)

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	assert.Empty(t, sender.Sent())
}

// ─── Context cancellation / clean unsubscribe ─────────────────────────────

func TestWatchBacklogItems_should_unsubscribeAndReturn_When_ContextIsCanceled(t *testing.T) {
	t.Parallel()
	svc, storage, bus := newTestBacklogServiceWithBus(t)
	ctx := context.Background()

	// Seed one item and wait for its snapshot to be sent before canceling —
	// otherwise a cancel racing the fresh-snapshot branch's
	// storage.ListBacklogItems(ctx, ...) call (which shares runCtx) can itself
	// return a "context canceled" error, which is correct propagation
	// behavior but would make this test about that race instead of about
	// clean shutdown/unsubscribe.
	_, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "Item", Status: string(session.BacklogStatusReady)})
	require.NoError(t, err)

	sender := &fakeBacklogItemEventSender{}
	runCtx, cancel := context.WithCancel(ctx)
	done := runWatchBacklogItems(runCtx, svc, &sessionv1.WatchBacklogItemsRequest{}, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, bus.SubscriberCount())

	requireCleanReturn(t, cancel, done)

	// Subscribe()'s own cleanup goroutine (pkg/events/bus.go) also unsubscribes
	// asynchronously on ctx.Done(), racing watchBacklogItems's deferred
	// Unsubscribe call — both are idempotent, so poll briefly rather than
	// asserting immediately.
	require.Eventually(t, func() bool { return bus.SubscriberCount() == 0 }, 2*time.Second, 10*time.Millisecond,
		"subscriber must be cleaned up after context cancellation (no leaked subscription)")
}

// ─── convertEventToBacklogItemEvent: all 6 BacklogChangeKind variants ─────

func TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	item := &session.BacklogItemData{ID: "item-1", Title: "T", Status: "review", CreatedAt: fixedTime, UpdatedAt: fixedTime}
	archivedAt := fixedTime

	tests := []struct {
		name    string
		payload *events.BacklogItemEventPayload
		check   func(t *testing.T, ev *sessionv1.BacklogItemEvent)
	}{
		{
			name: "status_changed",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeStatusTransition, Item: item,
				OldStatus: "in_progress", NewStatus: "review",
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				sc := ev.GetStatusChanged()
				require.NotNil(t, sc)
				assert.Equal(t, "item-1", sc.GetItemId())
				assert.Equal(t, "in_progress", sc.GetOldStatus())
				assert.Equal(t, "review", sc.GetNewStatus())
				assert.NotNil(t, sc.GetItem())
			},
		},
		{
			name: "verdict_recorded",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeVerdictRecorded, Item: item,
				Verdict: &session.ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass, Summary: "looks good"},
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				vr := ev.GetVerdictRecorded()
				require.NotNil(t, vr)
				assert.Equal(t, "item-1", vr.GetItemId())
				require.NotNil(t, vr.GetVerdict())
				assert.Equal(t, "PASS", vr.GetVerdict().GetOverallOutcome())
				assert.Equal(t, "looks good", vr.GetVerdict().GetSummary())
			},
		},
		{
			name: "session_attached",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeSessionAttached, Item: item, SessionID: "sess-1",
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				sa := ev.GetSessionAttached()
				require.NotNil(t, sa)
				assert.Equal(t, "item-1", sa.GetItemId())
				assert.Equal(t, "sess-1", sa.GetSessionId())
			},
		},
		{
			name: "item_updated",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeItemUpdated, Item: item, UpdatedFields: []string{"title", "description"},
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				upd := ev.GetItemUpdated()
				require.NotNil(t, upd)
				assert.Equal(t, "item-1", upd.GetItemId())
				assert.Equal(t, []string{"title", "description"}, upd.GetUpdatedFields())
			},
		},
		{
			// BacklogChangeTriageProgressUpdated reuses the item_updated wire
			// variant rather than a new proto message (plan.md Epic 2.2.5).
			name: "triage_progress_updated_reuses_item_updated_variant",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeTriageProgressUpdated, Item: item, UpdatedFields: []string{"triageResultSummary"},
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				upd := ev.GetItemUpdated()
				require.NotNil(t, upd)
				assert.Equal(t, []string{"triageResultSummary"}, upd.GetUpdatedFields())
			},
		},
		{
			name: "item_archived",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeItemArchived, Item: item, ArchivedAt: &archivedAt,
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				ar := ev.GetItemArchived()
				require.NotNil(t, ar)
				assert.Equal(t, "item-1", ar.GetItemId())
				require.NotNil(t, ar.GetArchivedAt())
				assert.True(t, ar.GetArchivedAt().AsTime().Equal(archivedAt))
			},
		},
		{
			name: "item_removed",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeItemRemoved, Item: item, RemovedReason: "deleted by user",
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				rm := ev.GetItemRemoved()
				require.NotNil(t, rm)
				assert.Equal(t, "item-1", rm.GetItemId())
				assert.Equal(t, "deleted by user", rm.GetReason())
			},
		},
		{
			// [validation.md Correction #1] Appended to this existing table
			// rather than a new standalone test function — see Epic 8.4,
			// Task 8.4.1b.
			name: "activity_note_added",
			payload: &events.BacklogItemEventPayload{
				Kind: events.BacklogChangeActivityNoteAdded, Item: item,
				ActivityNote: &session.ActivityNoteData{
					ID: "note-1", Message: "a note", AuthorSessionUUID: "sess-1", AuthorSessionTitle: "Worker",
				},
			},
			check: func(t *testing.T, ev *sessionv1.BacklogItemEvent) {
				an := ev.GetActivityNoteAdded()
				require.NotNil(t, an, "the oneof must never be left empty for this kind (Blocker 3's original risk)")
				assert.Equal(t, "item-1", an.GetItemId())
				require.NotNil(t, an.GetNote())
				assert.Equal(t, "note-1", an.GetNote().GetId())
				assert.Equal(t, "a note", an.GetNote().GetMessage())
				assert.Equal(t, "sess-1", an.GetNote().GetAuthorSessionUuid())
				assert.Equal(t, "Worker", an.GetNote().GetAuthorSessionTitle())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			evt := &events.Event{Type: events.EventBacklogItemChanged, Timestamp: fixedTime, BacklogItemPayload: tc.payload}
			out := convertEventToBacklogItemEvent(evt, nil)
			require.NotNil(t, out)
			tc.check(t, out)
		})
	}
}

// TestBacklogItemMatchesFilters_should_MatchActivityNoteEvent_When_StatusFilterSet
// (Epic 8.4, Story 8.4.3 / Blocker 2 fix) documents the bug the fix closes,
// not just the fixed state in isolation: a sparse pre-fix snapshot (only ID
// set) would have been silently dropped by any non-empty status_filter/
// category_filter, since backlogItemMatchesFilters treats a zero-value
// Status/RepoPath as a non-match. AppendActivityNote's Blocker-2 fix
// populates Status/RepoPath before publishing, so the post-fix shape passes.
func TestBacklogItemMatchesFilters_should_MatchActivityNoteEvent_When_StatusFilterSet(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.WatchBacklogItemsRequest{
		StatusFilter:   []string{"in_progress"},
		CategoryFilter: []string{"/repo/a"},
	}

	populated := &session.BacklogItemData{ID: "item-1", Status: "in_progress", RepoPath: "/repo/a"}
	assert.True(t, backlogItemMatchesFilters(populated, msg), "an activity-note event with Status/RepoPath populated must match a non-empty filter")

	sparse := &session.BacklogItemData{ID: "item-1"}
	assert.False(t, backlogItemMatchesFilters(sparse, msg), "a sparse snapshot with no Status/RepoPath (the pre-fix shape) would have been silently dropped by a non-empty filter")
}
