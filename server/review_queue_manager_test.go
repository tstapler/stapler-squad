package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil"
)

// TestReactiveQueueManagerIntegration tests the full reactive queue workflow
func TestReactiveQueueManagerIntegration(t *testing.T) {
	// Setup test directory
	testDir := filepath.Join(os.TempDir(), "stapler-squad-test-reactive-queue")
	defer os.RemoveAll(testDir)

	// Setup components
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(queue, statusManager, nil)
	eventBus := events.NewEventBus(10)
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create manager
	reactiveQueueMgr := NewReactiveQueueManager(
		queue,
		reviewQueuePoller,
		eventBus,
		statusManager,
		storage,
	)

	// Start manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reactiveQueueMgr.Start(ctx)

	// Wait for manager to initialize
	if err := testutil.WaitForCondition(func() bool {
		return reviewQueuePoller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatalf("manager failed to initialize: %v", err)
	}

	// Test 1: Add stream client
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	filters := &WatchReviewQueueFilters{
		InitialSnapshot:   true,
		IncludeStatistics: true,
	}

	eventCh, clientID := reactiveQueueMgr.AddStreamClient(clientCtx, filters)
	defer reactiveQueueMgr.RemoveStreamClient(clientID)

	if clientID == "" {
		t.Fatal("Expected non-empty client ID")
	}

	// Drain any initial events (statistics from empty queue)
	drainEvents(eventCh, 100*time.Millisecond)

	// Test 2: Add item to queue and verify event
	item := &session.ReviewItem{
		SessionID:   "test-session-1",
		SessionName: "Test Session",
		Reason:      session.ReasonApprovalPending,
		Priority:    session.PriorityHigh,
		DetectedAt:  time.Now(),
		Context:     "Test context",
	}

	queue.Add(item)

	// Should receive ItemAdded event (may also get statistics)
	foundItemAdded := false
	timeout := time.After(1 * time.Second)
	for !foundItemAdded {
		select {
		case event := <-eventCh:
			if event.Event == nil {
				t.Fatal("Expected event to have data")
			}
			if _, ok := event.Event.(*sessionv1.ReviewQueueEvent_ItemAdded); ok {
				foundItemAdded = true
			}
			// Ignore statistics events
		case <-timeout:
			t.Fatal("Timeout waiting for ItemAdded event")
		}
	}

	// Test 3: Remove item and verify event
	queue.Remove("test-session-1")

	foundItemRemoved := false
	timeout = time.After(1 * time.Second)
	for !foundItemRemoved {
		select {
		case event := <-eventCh:
			if event.Event == nil {
				t.Fatal("Expected event to have data")
			}
			if _, ok := event.Event.(*sessionv1.ReviewQueueEvent_ItemRemoved); ok {
				foundItemRemoved = true
			}
			// Ignore statistics events
		case <-timeout:
			t.Fatal("Timeout waiting for ItemRemoved event")
		}
	}

	// Test 4: Publish user interaction event
	eventBus.Publish(events.NewUserInteractionEvent(
		"test-session-1",
		"terminal_input",
		"",
	))

	// Test 5: Publish session acknowledged event
	eventBus.Publish(events.NewSessionAcknowledgedEvent(
		"test-session-1",
		"user_acknowledged",
	))

	// Test 6: Remove stream client (channel will be closed asynchronously)
	reactiveQueueMgr.RemoveStreamClient(clientID)

	// Cleanup
	reactiveQueueMgr.Stop()
}

// TestOnItemAdded_EventBusBehavior_BUG001 verifies the BUG-001 fix:
// OnItemAdded must NOT publish to the EventBus for ReasonApprovalPending because
// the ApprovalHandler already broadcasts a richer notification via broadcastApprovalNotification.
// Publishing a second event here creates duplicate notification cards in the UI.
// All other reasons SHOULD publish to the EventBus for history/toast routing.
func TestOnItemAdded_EventBusBehavior_BUG001(t *testing.T) {
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusManager, nil)
	eventBus := events.NewEventBus(10)

	mgr := NewReactiveQueueManager(queue, poller, eventBus, statusManager, nil)

	// Subscribe to the EventBus to capture any notifications published by OnItemAdded.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh, _ := eventBus.Subscribe(ctx)

	// ── Case 1: ReasonApprovalPending → NO EventBus event ──────────────────
	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:  "session-approval",
		Reason:     session.ReasonApprovalPending,
		Priority:   session.PriorityHigh,
		DetectedAt: time.Now(),
	})

	select {
	case ev := <-eventCh:
		t.Errorf("BUG-001 regression: expected NO EventBus event for ReasonApprovalPending, got type=%s", ev.Type)
	case <-time.After(100 * time.Millisecond):
		// Correct — no event emitted
	}

	// ── Case 2: ReasonInputRequired → EventBus notification SHOULD fire ────
	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:  "session-input",
		Reason:     session.ReasonInputRequired,
		Priority:   session.PriorityHigh,
		DetectedAt: time.Now(),
	})

	select {
	case ev := <-eventCh:
		if ev.Type != events.EventNotification {
			t.Errorf("expected EventNotification for ReasonInputRequired, got %s", ev.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected EventBus event for ReasonInputRequired, got none (timeout)")
	}

	// ── Case 3: ReasonErrorState → EventBus notification SHOULD fire ────────
	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:  "session-error",
		Reason:     session.ReasonErrorState,
		Priority:   session.PriorityMedium,
		DetectedAt: time.Now(),
	})

	select {
	case ev := <-eventCh:
		if ev.Type != events.EventNotification {
			t.Errorf("expected EventNotification for ReasonErrorState, got %s", ev.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected EventBus event for ReasonErrorState, got none (timeout)")
	}
}

// TestReactiveQueueManager_EventSessionDeleted_RemovesFromQueue verifies that
// publishing EventSessionDeleted removes the session from the review queue (UT-GO-03).
func TestReactiveQueueManager_EventSessionDeleted_RemovesFromQueue(t *testing.T) {
	mgr, poller, bus := newReactiveQueueTestSetup(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go mgr.Start(ctx)

	if err := testutil.WaitForCondition(func() bool {
		return poller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatalf("manager failed to initialize: %v", err)
	}
	defer mgr.Stop()

	// Add a session to the queue.
	const sessionID = "delete-target-session"
	mgr.queue.Add(&session.ReviewItem{
		SessionID:  sessionID,
		Reason:     session.ReasonInputRequired,
		Priority:   session.PriorityMedium,
		DetectedAt: time.Now(),
	})

	// Confirm it's in the queue.
	if items := mgr.queue.List(); len(items) == 0 {
		t.Fatal("expected item to be in queue before deletion")
	}

	// Publish EventSessionDeleted.
	bus.Publish(events.NewSessionDeletedEvent(sessionID))

	// Wait for the queue to be emptied.
	if err := testutil.WaitForCondition(func() bool {
		return len(mgr.queue.List()) == 0
	}, testutil.FastWaitConfig()); err != nil {
		t.Errorf("session was not removed from queue after EventSessionDeleted: queue len = %d", len(mgr.queue.List()))
	}
}

// drainEvents drains all events from channel within timeout
func drainEvents(ch <-chan *sessionv1.ReviewQueueEvent, timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		select {
		case <-ch:
			// Drain event
		case <-deadline:
			return
		}
	}
}

// TestReactiveQueueManagerMultipleClients tests multiple concurrent clients
func TestReactiveQueueManagerMultipleClients(t *testing.T) {
	// Setup test directory
	testDir := filepath.Join(os.TempDir(), "stapler-squad-test-multiple-clients")
	defer os.RemoveAll(testDir)

	// Setup
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(queue, statusManager, nil)
	eventBus := events.NewEventBus(10)
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	reactiveQueueMgr := NewReactiveQueueManager(
		queue,
		reviewQueuePoller,
		eventBus,
		statusManager,
		storage,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reactiveQueueMgr.Start(ctx)

	if err := testutil.WaitForCondition(func() bool {
		return reviewQueuePoller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatalf("manager failed to initialize: %v", err)
	}

	// Add 3 clients
	numClients := 3
	clients := make([]struct {
		ch       <-chan *sessionv1.ReviewQueueEvent
		id       string
		ctx      context.Context
		cancel   context.CancelFunc
		received int
	}, numClients)

	for i := 0; i < numClients; i++ {
		clientCtx, clientCancel := context.WithCancel(context.Background())
		filters := &WatchReviewQueueFilters{
			InitialSnapshot:   false,
			IncludeStatistics: true,
		}
		eventCh, clientID := reactiveQueueMgr.AddStreamClient(clientCtx, filters)
		clients[i].ch = eventCh
		clients[i].id = clientID
		clients[i].ctx = clientCtx
		clients[i].cancel = clientCancel
	}

	// Add item to queue
	item := &session.ReviewItem{
		SessionID:   "test-multi-1",
		SessionName: "Multi Test",
		Reason:      session.ReasonApprovalPending,
		Priority:    session.PriorityMedium,
		DetectedAt:  time.Now(),
	}
	queue.Add(item)

	// All clients should receive the event
	timeout := time.After(1 * time.Second)
	for i := 0; i < numClients; i++ {
		select {
		case <-clients[i].ch:
			clients[i].received++
		case <-timeout:
			t.Errorf("Client %d did not receive event", i)
		}
	}

	// Verify all clients received the event
	for i := 0; i < numClients; i++ {
		if clients[i].received != 1 {
			t.Errorf("Client %d expected 1 event, got %d", i, clients[i].received)
		}
	}

	// Cleanup
	for i := 0; i < numClients; i++ {
		clients[i].cancel()
		reactiveQueueMgr.RemoveStreamClient(clients[i].id)
	}

	reactiveQueueMgr.Stop()
}

// TestReactiveQueueManagerFiltering tests client-side filtering
func TestReactiveQueueManagerFiltering(t *testing.T) {
	// Setup test directory
	testDir := filepath.Join(os.TempDir(), "stapler-squad-test-filtering")
	defer os.RemoveAll(testDir)

	// Setup
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(queue, statusManager, nil)
	eventBus := events.NewEventBus(10)
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	reactiveQueueMgr := NewReactiveQueueManager(
		queue,
		reviewQueuePoller,
		eventBus,
		statusManager,
		storage,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reactiveQueueMgr.Start(ctx)

	if err := testutil.WaitForCondition(func() bool {
		return reviewQueuePoller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatalf("manager failed to initialize: %v", err)
	}

	// Add client with priority filter (only HIGH priority)
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	filters := &WatchReviewQueueFilters{
		PriorityFilter:    []session.Priority{session.PriorityHigh},
		InitialSnapshot:   false,
		IncludeStatistics: false,
	}

	eventCh, clientID := reactiveQueueMgr.AddStreamClient(clientCtx, filters)
	defer reactiveQueueMgr.RemoveStreamClient(clientID)

	// Add HIGH priority item - should be received
	highItem := &session.ReviewItem{
		SessionID:  "high-priority",
		Reason:     session.ReasonApprovalPending,
		Priority:   session.PriorityHigh,
		DetectedAt: time.Now(),
	}
	queue.Add(highItem)

	select {
	case <-eventCh:
		// Expected - high priority item received
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Expected to receive high priority event")
	}

	// Add LOW priority item - should NOT be received
	lowItem := &session.ReviewItem{
		SessionID:  "low-priority",
		Reason:     session.ReasonIdleTimeout,
		Priority:   session.PriorityLow,
		DetectedAt: time.Now(),
	}
	queue.Add(lowItem)

	select {
	case event := <-eventCh:
		t.Errorf("Expected to NOT receive low priority event, but got: %v", event)
	case <-time.After(200 * time.Millisecond):
		// Expected - low priority filtered out
	}

	reactiveQueueMgr.Stop()
}

// TestReactiveQueueManagerEventTypes tests all event types
func TestReactiveQueueManagerEventTypes(t *testing.T) {
	// Setup test directory
	testDir := filepath.Join(os.TempDir(), "stapler-squad-test-event-types")
	defer os.RemoveAll(testDir)

	// Setup
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(queue, statusManager, nil)
	eventBus := events.NewEventBus(10)
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	reactiveQueueMgr := NewReactiveQueueManager(
		queue,
		reviewQueuePoller,
		eventBus,
		statusManager,
		storage,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reactiveQueueMgr.Start(ctx)

	if err := testutil.WaitForCondition(func() bool {
		return reviewQueuePoller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		t.Fatalf("manager failed to initialize: %v", err)
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	filters := &WatchReviewQueueFilters{
		IncludeStatistics: true,
		InitialSnapshot:   false,
	}

	eventCh, clientID := reactiveQueueMgr.AddStreamClient(clientCtx, filters)
	defer reactiveQueueMgr.RemoveStreamClient(clientID)

	// Test ItemAdded
	item := &session.ReviewItem{
		SessionID:  "test-events",
		Priority:   session.PriorityMedium,
		Reason:     session.ReasonInputRequired,
		DetectedAt: time.Now(),
	}
	queue.Add(item)

	event := waitForEvent(t, eventCh, "ItemAdded", 500*time.Millisecond)
	if _, ok := event.Event.(*sessionv1.ReviewQueueEvent_ItemAdded); !ok {
		t.Errorf("Expected ItemAdded, got %T", event.Event)
	}

	// Test ItemRemoved
	queue.Remove("test-events")

	event = waitForEvent(t, eventCh, "ItemRemoved", 500*time.Millisecond)
	if _, ok := event.Event.(*sessionv1.ReviewQueueEvent_ItemRemoved); !ok {
		t.Errorf("Expected ItemRemoved, got %T", event.Event)
	}

	reactiveQueueMgr.Stop()
}

// Helper function to wait for an event with timeout
func waitForEvent(t *testing.T, eventCh <-chan *sessionv1.ReviewQueueEvent, eventType string, timeout time.Duration) *sessionv1.ReviewQueueEvent {
	t.Helper()
	select {
	case event := <-eventCh:
		if event == nil {
			t.Fatalf("Received nil event for %s", eventType)
		}
		return event
	case <-time.After(timeout):
		t.Fatalf("Timeout waiting for %s event", eventType)
		return nil
	}
}

// BenchmarkReactiveQueueManagerThroughput measures event processing throughput
func BenchmarkReactiveQueueManagerThroughput(b *testing.B) {
	// Use b.TempDir() so each benchmark calibration call gets an isolated directory
	// and automatic cleanup — avoids path conflicts between successive b.N runs.
	testDir := b.TempDir()

	// Setup
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	// Use a fast poll interval so event delivery isn't gated on the default 2s cycle.
	benchPollerCfg := session.DefaultReviewQueuePollerConfig()
	benchPollerCfg.PollInterval = 10 * time.Millisecond
	benchPollerCfg.SlowPollInterval = 10 * time.Millisecond
	reviewQueuePoller := session.NewReviewQueuePollerWithConfig(queue, statusManager, nil, benchPollerCfg)
	eventBus := events.NewEventBus(100)
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	reactiveQueueMgr := NewReactiveQueueManager(
		queue,
		reviewQueuePoller,
		eventBus,
		statusManager,
		storage,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go reactiveQueueMgr.Start(ctx)

	if err := testutil.WaitForCondition(func() bool {
		return reviewQueuePoller.IsRunning()
	}, testutil.FastWaitConfig()); err != nil {
		b.Fatalf("manager failed to initialize: %v", err)
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	filters := &WatchReviewQueueFilters{
		IncludeStatistics: false,
		InitialSnapshot:   false,
	}

	eventCh, clientID := reactiveQueueMgr.AddStreamClient(clientCtx, filters)
	defer reactiveQueueMgr.RemoveStreamClient(clientID)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		queue.Add(&session.ReviewItem{
			SessionID:  fmt.Sprintf("bench-%d", i),
			Priority:   session.PriorityMedium,
			Reason:     session.ReasonInputRequired,
			DetectedAt: time.Now(),
		})
	}

	b.StopTimer()

	// Drain events delivered during the run. The event channel is buffered at 100;
	// at high b.N most events are dropped by publishToClients (by design — slow consumers
	// don't block the publisher). We verify at least one event arrived to confirm the
	// reactive path is wired, without blocking indefinitely.
	delivered := 0
	drain := time.After(200 * time.Millisecond)
drainLoop:
	for {
		select {
		case <-eventCh:
			delivered++
		case <-drain:
			break drainLoop
		}
	}
	if delivered == 0 {
		b.Fatalf("no events delivered: reactive observer not wired (added %d items)", b.N)
	}
	b.ReportMetric(float64(delivered)/float64(b.N)*100, "delivery%")

	reactiveQueueMgr.Stop()
}

// --------------------------------------------------------------------------
// Session ID resolution in review-queue notification events
// --------------------------------------------------------------------------

// newReactiveQueueTestSetup creates a ReactiveQueueManager with a real ReviewQueuePoller
// and event bus, returning both for use in tests.
func newReactiveQueueTestSetup(t *testing.T) (*ReactiveQueueManager, *session.ReviewQueuePoller, *events.EventBus) {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("newReactiveQueueTestSetup: create repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("newReactiveQueueTestSetup: create storage: %v", err)
	}

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)

	mgr := NewReactiveQueueManager(queue, poller, bus, statusMgr, storage)
	return mgr, poller, bus
}

// TestOnItemAdded_NotificationUsesStableID verifies that when ReactiveQueueManager
// fires a notification for a newly-queued item, the event.SessionID is the session's
// UUID — not the title (which is what ReviewItem.SessionID holds as the queue key).
func TestOnItemAdded_NotificationUsesStableID(t *testing.T) {
	mgr, poller, bus := newReactiveQueueTestSetup(t)

	const sessionTitle = "stelekit"
	const sessionUUID = "aaaabbbb-1111-2222-3333-ffffffffffff"
	inst := &session.Instance{
		Title: sessionTitle,
		UUID:  sessionUUID,
	}
	poller.SetInstances([]*session.Instance{inst})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	// OnItemAdded is the callback fired by the queue when the poller adds an item.
	// ReviewItem.SessionID == inst.Title (the queue's key), as expected.
	item := &session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonInputRequired,
		Priority:    session.PriorityMedium,
		DetectedAt:  time.Now(),
	}
	mgr.OnItemAdded(item)

	// Collect events until we find the notification or time out.
	var gotID string
	deadline := time.After(2 * time.Second)
	for gotID == "" {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotID = e.SessionID
			}
		case <-deadline:
			t.Fatal("timed out waiting for notification event from OnItemAdded")
		}
	}

	if gotID != sessionUUID {
		t.Errorf("notification event.SessionID = %q, want UUID %q (title was %q)",
			gotID, sessionUUID, sessionTitle)
	}
}

// TestOnItemAdded_NotificationFallsBackToTitleWhenNoMatch verifies that when
// the poller has no matching instance, the raw title is used gracefully.
func TestOnItemAdded_NotificationFallsBackToTitleWhenNoMatch(t *testing.T) {
	mgr, _, bus := newReactiveQueueTestSetup(t)
	// No instances in poller — FindInstance will return nil.

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	item := &session.ReviewItem{
		SessionID:   "orphan-session",
		SessionName: "orphan-session",
		Reason:      session.ReasonIdleTimeout,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	}
	mgr.OnItemAdded(item)

	var gotID string
	deadline := time.After(2 * time.Second)
	for gotID == "" {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotID = e.SessionID
			}
		case <-deadline:
			t.Fatal("timed out waiting for notification event")
		}
	}

	if gotID != "orphan-session" {
		t.Errorf("notification event.SessionID = %q, want raw title %q", gotID, "orphan-session")
	}
}

// fakeOneShotPRCreator is a test double for OneShotPRCreator that records every
// call so maybeAutoCreatePR's trigger behavior can be asserted without exercising
// the real headless LLM pool / claude binary.
type fakeOneShotPRCreator struct {
	mu     sync.Mutex
	calls  []string // sessionIDs passed to RunOneShotForSession
	prURL  string
	runErr error
}

func (f *fakeOneShotPRCreator) RunOneShotForSession(_ context.Context, sessionID, _ string, _ int32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sessionID)
	return f.prURL, f.runErr
}

func (f *fakeOneShotPRCreator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// newReactiveQueueTestSetupWithStorage is like newReactiveQueueTestSetup but also
// returns the underlying *session.Storage, needed by tests that create backlog
// items / item sessions to exercise maybeAutoCreatePR's storage lookups.
func newReactiveQueueTestSetupWithStorage(t *testing.T) (*ReactiveQueueManager, *session.ReviewQueuePoller, *session.Storage) {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(testDir, "sessions.db")))
	if err != nil {
		t.Fatalf("newReactiveQueueTestSetupWithStorage: create repo: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("newReactiveQueueTestSetupWithStorage: create storage: %v", err)
	}

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)

	mgr := NewReactiveQueueManager(queue, poller, bus, statusMgr, storage)
	return mgr, poller, storage
}

// TestMaybeAutoCreatePR_RunsOneShot_When_AutoCreatePREnabled verifies the opt-in
// "auto-create PR on Complete" policy (docs/tasks/backlog-feature-improvement.md,
// 2026-07-17 entry): when the backlog item behind a session has AutoCreatePR set
// and that session newly transitions to TASK_COMPLETE, the one-shot PR-creation
// prompt runs automatically — no manual Review Queue "Create PR" click required.
func TestMaybeAutoCreatePR_RunsOneShot_When_AutoCreatePREnabled(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/42"}
	mgr.SetOneShotRunner(fake)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "auto-create-pr item",
		Status:       string(session.BacklogStatusInProgress),
		Priority:     3,
		AutoCreatePR: true,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "widgets-work-session"
	const sessionUUID = "cccc1111-2222-3333-4444-555566667777"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID}
	poller.SetInstances([]*session.Instance{inst})

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	deadline := time.After(3 * time.Second)
	for fake.callCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for AutoCreatePR to invoke the one-shot PR runner")
		case <-time.After(20 * time.Millisecond):
		}
	}

	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 one-shot call, got %d", got)
	}
	fake.mu.Lock()
	gotSessionID := fake.calls[0]
	fake.mu.Unlock()
	if gotSessionID != sessionUUID {
		t.Errorf("one-shot runner called with sessionID = %q, want stable UUID %q", gotSessionID, sessionUUID)
	}
}

// TestMaybeAutoCreatePR_DoesNothing_When_AutoCreatePRDisabled is the
// default-behavior guard: with AutoCreatePR left false (the default), a
// TASK_COMPLETE session must NOT trigger an automatic PR — the existing manual
// "Create PR" click in the Review Queue remains the only path.
func TestMaybeAutoCreatePR_DoesNothing_When_AutoCreatePRDisabled(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/99"}
	mgr.SetOneShotRunner(fake)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "manual-pr item",
		Status:   string(session.BacklogStatusInProgress),
		Priority: 3,
		// AutoCreatePR intentionally omitted — defaults to false.
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "manual-work-session"
	const sessionUUID = "dddd1111-2222-3333-4444-555566667777"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID}
	poller.SetInstances([]*session.Instance{inst})

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	assertNeverInvoked(t, fake, "AutoCreatePR=false must never invoke the one-shot PR runner")
}

// assertNeverInvoked polls fake.callCount() for a window instead of a single flat
// time.Sleep, failing immediately if the runner is ever invoked rather than only
// checking once after a fixed delay. Mirrors this file's existing "poll until"
// idiom (see TestOnItemAdded_NotificationUsesStableID) applied to a negative
// assertion.
func assertNeverInvoked(t *testing.T, fake *fakeOneShotPRCreator, msg string) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		if got := fake.callCount(); got > 0 {
			t.Fatalf("%s, got %d call(s)", msg, got)
		}
		select {
		case <-deadline:
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// TestMaybeAutoCreatePR_DoesNothing_When_SessionNotBacklogLinked covers the
// guard that keeps this feature scoped to backlog work sessions: an ordinary
// session with no ItemSession row (most sessions in this app aren't backlog
// work items) must never trigger AutoCreatePR, even if it reaches
// TASK_COMPLETE. Regression guard for storage.GetItemSessionBySessionUUID's
// ErrNotFound branch in maybeAutoCreatePR.
func TestMaybeAutoCreatePR_DoesNothing_When_SessionNotBacklogLinked(t *testing.T) {
	mgr, poller, _ := newReactiveQueueTestSetupWithStorage(t)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/1"}
	mgr.SetOneShotRunner(fake)

	const sessionTitle = "ad-hoc-session"
	const sessionUUID = "eeee1111-2222-3333-4444-555566667777"
	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID}
	poller.SetInstances([]*session.Instance{inst})
	// Deliberately no CreateBacklogItem / CreateItemSession call.

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	assertNeverInvoked(t, fake, "a session with no ItemSession row must never trigger AutoCreatePR")
}

// TestMaybeAutoCreatePR_DoesNothing_When_PRAlreadyExists covers the idempotency
// guard: a session that already has a GitHub PR URL (e.g. a prior automatic or
// manual run already created one) must not trigger a second one-shot run even
// if AutoCreatePR is true and the item re-enters the queue at TASK_COMPLETE.
func TestMaybeAutoCreatePR_DoesNothing_When_PRAlreadyExists(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/2"}
	mgr.SetOneShotRunner(fake)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "already-has-pr item",
		Status:       string(session.BacklogStatusInProgress),
		Priority:     3,
		AutoCreatePR: true,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "already-has-pr-session"
	const sessionUUID = "ffff1111-2222-3333-4444-555566667777"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID, GitHubPRURL: "https://github.com/acme/widgets/pull/1"}
	poller.SetInstances([]*session.Instance{inst})

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	assertNeverInvoked(t, fake, "a session that already has a PR must never trigger a second AutoCreatePR run")
}

// TestMaybeAutoCreatePR_DoesNothing_When_ReasonNotTaskComplete is the positive
// guard for the trigger condition itself: AutoCreatePR fires only for
// ReasonTaskComplete, not for any other AttentionReason that lands in the
// queue (approval pending, input required, etc.) even when AutoCreatePR is true.
func TestMaybeAutoCreatePR_DoesNothing_When_ReasonNotTaskComplete(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/3"}
	mgr.SetOneShotRunner(fake)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "not-yet-complete item",
		Status:       string(session.BacklogStatusInProgress),
		Priority:     3,
		AutoCreatePR: true,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "not-yet-complete-session"
	const sessionUUID = "aaaa2222-3333-4444-5555-666677778888"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID}
	poller.SetInstances([]*session.Instance{inst})

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonApprovalPending,
		Priority:    session.PriorityHigh,
		DetectedAt:  time.Now(),
	})

	assertNeverInvoked(t, fake, "a non-TASK_COMPLETE reason must never trigger AutoCreatePR")
}

// TestMaybeAutoCreatePR_TriggersViaOnQueueUpdated_When_ItemChangesReasonWhileQueued
// is the regression test for the CRITICAL gap found in code review: ReviewQueue.Add
// only fires OnItemAdded when the session's queue key is newly inserted
// (session/queue/queue.go's `exists` check). A session already queued for a
// different reason (e.g. ReasonIdle, when idle-timeout detection races ahead of
// the controller's completion-status detection) that later transitions to
// ReasonTaskComplete while remaining queued fires OnQueueUpdated instead — which
// must also trigger AutoCreatePR, or the feature silently misses this
// transition. Uses the real session.ReviewQueue (not direct OnItemAdded/
// OnQueueUpdated calls) so the observer wiring itself is exercised, not just
// maybeAutoCreatePR's internal logic.
func TestMaybeAutoCreatePR_TriggersViaOnQueueUpdated_When_ItemChangesReasonWhileQueued(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)
	mgr.queue.Subscribe(mgr) // wire the observer without a full Start() (no poller/event-bus loops needed)

	fake := &fakeOneShotPRCreator{prURL: "https://github.com/acme/widgets/pull/4"}
	mgr.SetOneShotRunner(fake)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "reason-changes-while-queued item",
		Status:       string(session.BacklogStatusInProgress),
		Priority:     3,
		AutoCreatePR: true,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "reason-changes-while-queued-session"
	const sessionUUID = "bbbb2222-3333-4444-5555-666677778888"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{Title: sessionTitle, UUID: sessionUUID}
	poller.SetInstances([]*session.Instance{inst})

	// Item first enters the queue for an unrelated reason — exists==false,
	// fires OnItemAdded, but Reason != TaskComplete so no trigger yet.
	mgr.queue.Add(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonIdle,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	// Same session, same queue key, now transitions to TaskComplete while
	// still present — exists==true, fires OnQueueUpdated, not OnItemAdded.
	mgr.queue.Add(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	deadline := time.After(3 * time.Second)
	for fake.callCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for AutoCreatePR to trigger via OnQueueUpdated after a reason change while queued")
		case <-time.After(20 * time.Millisecond):
		}
	}

	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 one-shot call, got %d", got)
	}
}

// ─── AC4: OnItemAdded suppression for Hidden backlog-linked sessions ──────────

// TestOnItemAdded_SuppressesNotification_When_SessionHidden verifies that a
// Hidden session (e.g. a headless triage/review worker) does not get a
// TASK_COMPLETE EventNotification published — Hidden sessions are excluded
// from the default session list/review queue UI, so routine completion
// notifications for them would be noise the operator can't act on directly.
func TestOnItemAdded_SuppressesNotification_When_SessionHidden(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "hidden-session backlog item",
		Status:   string(session.BacklogStatusInProgress),
		Priority: 3,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionUUID = "aaaa1111-2222-3333-4444-555566667777"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "review",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{
		Title:  "review:153f8eac",
		UUID:   sessionUUID,
		Hidden: true,
	}
	poller.SetInstances([]*session.Instance{inst})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := mgr.eventBus.Subscribe(ctx)

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:  "review:153f8eac",
		Reason:     session.ReasonTaskComplete,
		Priority:   session.PriorityLow,
		DetectedAt: time.Now(),
	})

	select {
	case ev := <-eventCh:
		t.Errorf("expected NO EventNotification for a Hidden session's TASK_COMPLETE, got type=%s", ev.Type)
	case <-time.After(300 * time.Millisecond):
		// Correct — no notification emitted.
	}
}

// TestOnItemAdded_SuppressesNotification_When_SessionHidden_ReasonVariants
// extends the base case to ReasonIdle and ReasonStale — the suppression must
// apply to all three "routine churn" reasons, not just TASK_COMPLETE.
func TestOnItemAdded_SuppressesNotification_When_SessionHidden_ReasonVariants(t *testing.T) {
	tests := []struct {
		name   string
		reason session.AttentionReason
	}{
		{"ReasonIdle", session.ReasonIdle},
		{"ReasonStale", session.ReasonStale},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

			item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
				Title:    "hidden-session backlog item " + tc.name,
				Status:   string(session.BacklogStatusInProgress),
				Priority: 3,
			})
			if err != nil {
				t.Fatalf("CreateBacklogItem: %v", err)
			}

			const sessionUUID = "aaaa1111-2222-3333-4444-555566667777"
			if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: sessionUUID,
				SessionRole: "review",
			}); err != nil {
				t.Fatalf("CreateItemSession: %v", err)
			}

			inst := &session.Instance{
				Title:  "review:153f8eac",
				UUID:   sessionUUID,
				Hidden: true,
			}
			poller.SetInstances([]*session.Instance{inst})

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			eventCh, _ := mgr.eventBus.Subscribe(ctx)

			mgr.OnItemAdded(&session.ReviewItem{
				SessionID:  "review:153f8eac",
				Reason:     tc.reason,
				Priority:   session.PriorityLow,
				DetectedAt: time.Now(),
			})

			select {
			case ev := <-eventCh:
				t.Errorf("expected NO EventNotification for a Hidden session's %s, got type=%s", tc.name, ev.Type)
			case <-time.After(300 * time.Millisecond):
				// Correct — no notification emitted.
			}
		})
	}
}

// TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked
// is the companion positive case: an ordinary (non-Hidden) backlog-linked
// session at TASK_COMPLETE must still get a notification, and that
// notification must carry item_id metadata resolved from the ItemSession
// linkage — proving the Hidden gate doesn't accidentally suppress the common
// case, and that metadata stamping actually reaches the published event.
func TestOnItemAdded_PublishesNotification_When_SessionNotHidden_EvenIfBacklogLinked(t *testing.T) {
	mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "visible-session backlog item",
		Status:   string(session.BacklogStatusInProgress),
		Priority: 3,
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	const sessionTitle = "widgets-work-session"
	const sessionUUID = "bbbb1111-2222-3333-4444-555566667777"
	if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}); err != nil {
		t.Fatalf("CreateItemSession: %v", err)
	}

	inst := &session.Instance{
		Title:  sessionTitle,
		UUID:   sessionUUID,
		Hidden: false,
	}
	poller.SetInstances([]*session.Instance{inst})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventCh, _ := mgr.eventBus.Subscribe(ctx)

	mgr.OnItemAdded(&session.ReviewItem{
		SessionID:   sessionTitle,
		SessionName: sessionTitle,
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		DetectedAt:  time.Now(),
	})

	var gotEvent *events.Event
	deadline := time.After(300 * time.Millisecond)
loop:
	for {
		select {
		case ev := <-eventCh:
			if ev.Type == events.EventNotification {
				gotEvent = ev
				break loop
			}
		case <-deadline:
			break loop
		}
	}

	if gotEvent == nil {
		t.Fatal("expected an EventNotification for a non-Hidden backlog-linked session, got none")
	}
	if got := gotEvent.NotificationMetadata["item_id"]; got != item.ID {
		t.Errorf("notification metadata item_id = %q, want %q", got, item.ID)
	}
	if got := gotEvent.NotificationMetadata["session_scoped"]; got != "true" {
		t.Errorf("notification metadata session_scoped = %q, want %q", got, "true")
	}
}

// TestOnItemAdded_PublishesNotification_When_SessionHidden_AndReasonIsErrorStateOrTestsFailing
// proves the suppression added above is correctly scoped: even a Hidden
// session must still surface a notification for ReasonErrorState/
// ReasonTestsFailing — those indicate a real problem an operator needs to
// see, unlike the routine TASK_COMPLETE/IDLE/STALE churn a headless worker
// generates in its ordinary course of operation.
func TestOnItemAdded_PublishesNotification_When_SessionHidden_AndReasonIsErrorStateOrTestsFailing(t *testing.T) {
	tests := []struct {
		name   string
		reason session.AttentionReason
	}{
		{"ReasonErrorState", session.ReasonErrorState},
		{"ReasonTestsFailing", session.ReasonTestsFailing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr, poller, storage := newReactiveQueueTestSetupWithStorage(t)

			item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
				Title:    "hidden-session-with-problem backlog item " + tc.name,
				Status:   string(session.BacklogStatusInProgress),
				Priority: 3,
			})
			if err != nil {
				t.Fatalf("CreateBacklogItem: %v", err)
			}

			const sessionUUID = "cccc9999-2222-3333-4444-555566667777"
			if _, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
				ItemID:      item.ID,
				SessionUUID: sessionUUID,
				SessionRole: "review",
			}); err != nil {
				t.Fatalf("CreateItemSession: %v", err)
			}

			inst := &session.Instance{
				Title:  "review:hidden-problem",
				UUID:   sessionUUID,
				Hidden: true,
			}
			poller.SetInstances([]*session.Instance{inst})

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			eventCh, _ := mgr.eventBus.Subscribe(ctx)

			mgr.OnItemAdded(&session.ReviewItem{
				SessionID:  "review:hidden-problem",
				Reason:     tc.reason,
				Priority:   session.PriorityMedium,
				DetectedAt: time.Now(),
			})

			select {
			case ev := <-eventCh:
				if ev.Type != events.EventNotification {
					t.Errorf("expected EventNotification for Hidden session's %s, got %s", tc.name, ev.Type)
				}
			case <-time.After(300 * time.Millisecond):
				t.Errorf("expected EventNotification for Hidden session's %s, got none (timeout)", tc.name)
			}
		})
	}
}
