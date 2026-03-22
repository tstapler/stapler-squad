package server

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	time.Sleep(50 * time.Millisecond)

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

	// Wait for event processing
	time.Sleep(50 * time.Millisecond)

	// Test 5: Publish session acknowledged event
	eventBus.Publish(events.NewSessionAcknowledgedEvent(
		"test-session-1",
		"user_acknowledged",
	))

	// Wait for event processing
	time.Sleep(50 * time.Millisecond)

	// Test 6: Remove stream client (channel will be closed asynchronously)
	reactiveQueueMgr.RemoveStreamClient(clientID)

	// Cleanup
	reactiveQueueMgr.Stop()
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

	time.Sleep(50 * time.Millisecond)

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

	time.Sleep(50 * time.Millisecond)

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

	time.Sleep(50 * time.Millisecond)

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
	// Setup test directory
	testDir := filepath.Join(os.TempDir(), "stapler-squad-bench-throughput")
	defer os.RemoveAll(testDir)

	// Setup
	queue := session.NewReviewQueue()
	statusManager := session.NewInstanceStatusManager()
	reviewQueuePoller := session.NewReviewQueuePoller(queue, statusManager, nil)
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

	time.Sleep(50 * time.Millisecond)

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
		item := &session.ReviewItem{
			SessionID:  "bench-" + string(rune(i)),
			Priority:   session.PriorityMedium,
			Reason:     session.ReasonInputRequired,
			DetectedAt: time.Now(),
		}
		queue.Add(item)

		// Drain event channel
		select {
		case <-eventCh:
		case <-time.After(100 * time.Millisecond):
			b.Fatal("Timeout receiving event")
		}
	}

	reactiveQueueMgr.Stop()
}
