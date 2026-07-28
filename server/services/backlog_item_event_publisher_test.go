package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// TestBacklogItemEventPublisher_should_publishConvertedEventToBus_When_PublishItemChangedCalled
// verifies PublishItemChanged converts a session.BacklogItemChange into an
// events.BacklogItemEventPayload and delivers it through a real EventBus
// (Story 1.3.2 AC).
func TestBacklogItemEventPublisher_should_publishConvertedEventToBus_When_PublishItemChangedCalled(t *testing.T) {
	bus := events.NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, _ := bus.Subscribe(ctx)

	publisher := &BacklogItemEventPublisher{Bus: bus}
	item := &session.BacklogItemData{ID: "item-123", Title: "test item"}

	publisher.PublishItemChanged(item, session.BacklogItemChange{
		Kind:      session.ChangeStatusTransition,
		OldStatus: "review",
		NewStatus: "done",
	})

	select {
	case received := <-eventCh:
		if received.Type != events.EventBacklogItemChanged {
			t.Fatalf("expected event type %s, got %s", events.EventBacklogItemChanged, received.Type)
		}
		if received.BacklogItemPayload == nil {
			t.Fatal("expected BacklogItemPayload to be non-nil")
		}
		if received.BacklogItemPayload.NewStatus != "done" {
			t.Errorf("expected NewStatus 'done', got %q", received.BacklogItemPayload.NewStatus)
		}
		if received.BacklogItemPayload.OldStatus != "review" {
			t.Errorf("expected OldStatus 'review', got %q", received.BacklogItemPayload.OldStatus)
		}
		if received.BacklogItemPayload.Kind != events.BacklogChangeStatusTransition {
			t.Errorf("expected Kind %s, got %s", events.BacklogChangeStatusTransition, received.BacklogItemPayload.Kind)
		}
		if received.BacklogItemPayload.Item != item {
			t.Error("expected payload Item to be the same pointer passed in")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// panickingChangeKind is a BacklogChangeKind value with no mapping in
// mapBacklogChangeKind, used to force a panic inside PublishItemChanged's
// payload-construction step without needing a separate test double type.
const panickingChangeKind session.BacklogChangeKind = "not-a-real-kind"

// TestBacklogItemEventPublisher_should_recoverAndLog_When_PublishItemChangedPanics
// verifies that a panic during payload construction (an unmapped
// BacklogChangeKind) is recovered inside PublishItemChanged and never
// propagates to the caller (Story 1.3.2 AC / Task 1.3.2b).
func TestBacklogItemEventPublisher_should_recoverAndLog_When_PublishItemChangedPanics(t *testing.T) {
	bus := events.NewEventBus(10)
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, _ := bus.Subscribe(ctx)

	publisher := &BacklogItemEventPublisher{Bus: bus}
	item := &session.BacklogItemData{ID: "item-456"}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PublishItemChanged panic reached the caller (recover() failed to contain it): %v", r)
			}
		}()
		publisher.PublishItemChanged(item, session.BacklogItemChange{Kind: panickingChangeKind})
	}()

	// No event should have been published — the panic happened before
	// bus.Publish was ever called.
	select {
	case received := <-eventCh:
		t.Fatalf("expected no event to be published after a panicking construction, got %+v", received)
	case <-time.After(100 * time.Millisecond):
		// Expected: nothing arrives.
	}
}

// TestBacklogItemEventPublisher_should_noOp_When_BusIsNil verifies the nil-Bus
// guard: a zero-value publisher (or one constructed without a Bus) must not
// panic when PublishItemChanged is called.
func TestBacklogItemEventPublisher_should_noOp_When_BusIsNil(t *testing.T) {
	publisher := &BacklogItemEventPublisher{}
	item := &session.BacklogItemData{ID: "item-789"}

	publisher.PublishItemChanged(item, session.BacklogItemChange{Kind: session.ChangeStatusTransition})
	// No assertion beyond "did not panic" — reaching this line is the test.
}

// TestEventBus_should_neverCrossDeliverBacklogEvents_When_TwoIndependentBusInstancesExist
// verifies Epic 7.1 / Task 7.1.1a / R15 (validation.md): workspace scoping for
// WatchBacklogItems is achieved entirely by process isolation, not by any
// filtering inside *events.EventBus itself (architecture.md §3). Two
// independently constructed *events.EventBus instances simulate two workspace
// OS processes; a publisher wired to busA only must never let its events
// reach a subscriber on the structurally separate busB.
func TestEventBus_should_neverCrossDeliverBacklogEvents_When_TwoIndependentBusInstancesExist(t *testing.T) {
	busA := events.NewEventBus(10)
	defer busA.Close()
	busB := events.NewEventBus(10)
	defer busB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chA, subA := busA.Subscribe(ctx)
	defer busA.Unsubscribe(subA)
	chB, subB := busB.Subscribe(ctx)
	defer busB.Unsubscribe(subB)

	// Publisher is wired to busA only, mirroring one workspace process's
	// single per-process eventBus.
	publisher := &BacklogItemEventPublisher{Bus: busA}
	item := &session.BacklogItemData{ID: "item-isolation-1"}

	publisher.PublishItemChanged(item, session.BacklogItemChange{
		Kind:      session.ChangeStatusTransition,
		OldStatus: "in_progress",
		NewStatus: "done",
	})

	// Positive confirmation: busA's own subscriber does receive the event —
	// proves the publish itself worked, so a subsequent silent busB is
	// evidence of isolation, not of a broken publisher.
	select {
	case received := <-chA:
		if received.Type != events.EventBacklogItemChanged {
			t.Fatalf("expected busA subscriber to receive %s, got %s", events.EventBacklogItemChanged, received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for busA subscriber to receive the published event")
	}

	// Negative confirmation: busB's subscriber must never see it.
	select {
	case received := <-chB:
		t.Fatalf("busB subscriber received an event published only to busA (cross-bus leak): %+v", received)
	case <-time.After(100 * time.Millisecond):
		// Expected: busB stays silent.
	}
}

// TestBacklogItemEventPublisher_should_notPanic_When_PublishingWithZeroSubscribers
// verifies R15's error/edge path (validation.md): publishing to a bus with no
// active subscribers (e.g. every WatchBacklogItems client has disconnected)
// must not block, error, or panic — the existing non-blocking fan-out in
// *events.EventBus is unaffected by the new BacklogItemEventPayload type.
func TestBacklogItemEventPublisher_should_notPanic_When_PublishingWithZeroSubscribers(t *testing.T) {
	bus := events.NewEventBus(10)
	defer bus.Close()

	publisher := &BacklogItemEventPublisher{Bus: bus}
	item := &session.BacklogItemData{ID: "item-no-subscribers"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		publisher.PublishItemChanged(item, session.BacklogItemChange{
			Kind:      session.ChangeStatusTransition,
			OldStatus: "in_progress",
			NewStatus: "done",
		})
	}()

	select {
	case <-done:
		// Expected: PublishItemChanged returned promptly without blocking or panicking.
	case <-time.After(time.Second):
		t.Fatal("PublishItemChanged blocked with zero subscribers on the bus")
	}
}

// newTestEntRepositoryForIsolation creates a temporary ent-backed
// *session.EntRepository, mirroring session_test's own
// newTestEntRepositoryForEvents helper (session/ent_repository_backlog_events_test.go).
// A local copy is kept here (rather than exporting/reusing that one) because
// this file lives in package services, not session_test, and the two files
// serve different packages' tests.
func newTestEntRepositoryForIsolation(t *testing.T) (*session.EntRepository, func()) {
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

// TestWorkspaceIsolation_should_holdEndToEnd_When_SimulatingTwoWorkspaceProcesses
// is R15's integration-level test (validation.md): a full-stack simulation of
// two workspace OS processes, each with its own ent-backed repository wired
// to its own *events.EventBus via the real BacklogItemEventPublisher adapter
// (architecture.md §3's resolved finding — isolation is structural, one bus
// per process). A real mutation (TransitionBacklogItemStatus) against
// workspace A's repository must never surface as an event on workspace B's
// subscriber.
func TestWorkspaceIsolation_should_holdEndToEnd_When_SimulatingTwoWorkspaceProcesses(t *testing.T) {
	repoA, cleanupA := newTestEntRepositoryForIsolation(t)
	defer cleanupA()
	repoB, cleanupB := newTestEntRepositoryForIsolation(t)
	defer cleanupB()

	busA := events.NewEventBus(10)
	defer busA.Close()
	busB := events.NewEventBus(10)
	defer busB.Close()

	repoA.SetItemChangePublisher(&BacklogItemEventPublisher{Bus: busA})
	repoB.SetItemChangePublisher(&BacklogItemEventPublisher{Bus: busB})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chA, subA := busA.Subscribe(ctx)
	defer busA.Unsubscribe(subA)
	chB, subB := busB.Subscribe(ctx)
	defer busB.Unsubscribe(subB)

	// Mutate workspace A only.
	item, err := repoA.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "workspace A item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = repoA.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusDone, &session.BacklogItemPrecondition{
		ExpectedStatus: string(session.BacklogStatusInProgress),
	}, session.TriggeredBySystem)
	require.NoError(t, err)

	// Positive confirmation: workspace A's own subscriber sees the event.
	select {
	case received := <-chA:
		if received.Type != events.EventBacklogItemChanged {
			t.Fatalf("expected workspace A subscriber to receive %s, got %s", events.EventBacklogItemChanged, received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for workspace A subscriber to receive the transition event")
	}

	// Negative confirmation: workspace B's subscriber, wired to a completely
	// separate repository and bus (simulating a separate OS process), never
	// sees a mutation that happened only in workspace A.
	select {
	case received := <-chB:
		t.Fatalf("workspace B subscriber received an event from workspace A's mutation (cross-workspace leak): %+v", received)
	case <-time.After(100 * time.Millisecond):
		// Expected: workspace B stays silent.
	}
}
