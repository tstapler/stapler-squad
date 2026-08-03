package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// makeAcknowledgedInstance creates a session instance with LastAcknowledged set after LastMeaningfulOutput,
// simulating a session the user has already dismissed from the review queue.
func makeAcknowledgedInstance(title string) *Instance {
	inst := &Instance{
		Title:  title,
		Status: Running,
	}
	inst.started.Store(true)
	inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Minute)
	inst.LastAcknowledged = time.Now().Add(-5 * time.Minute) // acked AFTER output
	// Keep the lock-free atomic shadows in sync with the plain fields set directly above —
	// IsAcknowledgedAfterOutput() reads only the atomic shadows (lastMeaningfulOutputNs,
	// lastAcknowledgedNs), not the plain time.Time fields, so tests that bypass the normal
	// write paths (UpdateTimestamps/MarkAcknowledged) must sync explicitly.
	inst.SyncAtomicTimestamps()
	return inst
}

// TestReviewQueuePoller_PreservesTimestampWhenStatusUnchanged verifies that
// the DetectedAt timestamp is only updated when the session's meaningful status changes,
// not on every poll cycle.
func TestReviewQueuePoller_PreservesTimestampWhenStatusUnchanged(t *testing.T) {
	// Create review queue
	queue := NewReviewQueue()

	// Simulate initial detection: session added to queue
	initialTime := time.Now().Add(-5 * time.Minute)
	reason := ReasonIdleTimeout
	priority := PriorityLow
	context := "Timed out after 5m of inactivity"

	// First poll: add item to queue
	item1 := &ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      reason,
		Priority:    priority,
		DetectedAt:  initialTime,
		Context:     context,
	}
	queue.Add(item1)

	t.Logf("Initial add: Reason=%s, Priority=%s, DetectedAt=%s",
		reason, priority, initialTime.Format(time.RFC3339))

	// Simulate multiple poll cycles with unchanged status
	for i := 0; i < 5; i++ {
		// Simulate poller checking and re-adding with same status
		// This is what the fixed poller does
		detectedAt := time.Now()
		if existingItem, exists := queue.Get("test-session"); exists {
			// Preserve timestamp if status hasn't changed
			if existingItem.Reason == reason &&
				existingItem.Priority == priority &&
				existingItem.Context == context {
				detectedAt = existingItem.DetectedAt
			}
		}

		updatedItem := &ReviewItem{
			SessionID:   "test-session",
			SessionName: "test-session",
			Reason:      reason,
			Priority:    priority,
			DetectedAt:  detectedAt,
			Context:     context,
		}
		queue.Add(updatedItem)
	}

	// Get the item after multiple poll cycles
	finalItem, exists := queue.Get("test-session")
	if !exists {
		t.Fatal("Expected session to be in review queue")
	}

	// Verify timestamp was PRESERVED (not updated)
	if !finalItem.DetectedAt.Equal(initialTime) {
		t.Errorf("Expected timestamp to be preserved when status unchanged.\nInitial: %s\nAfter polls: %s\nDifference: %s",
			initialTime.Format(time.RFC3339Nano),
			finalItem.DetectedAt.Format(time.RFC3339Nano),
			finalItem.DetectedAt.Sub(initialTime))
	}

	t.Logf("✓ After 5 poll cycles: Timestamp preserved correctly at %s",
		finalItem.DetectedAt.Format(time.RFC3339))

	// Now simulate a status change
	newReason := ReasonApprovalPending
	newPriority := PriorityHigh
	newContext := "Waiting for approval to proceed"

	// Simulate poller detecting status change
	detectedAt := time.Now()
	if existingItem, exists := queue.Get("test-session"); exists {
		if existingItem.Reason == newReason &&
			existingItem.Priority == newPriority &&
			existingItem.Context == newContext {
			detectedAt = existingItem.DetectedAt
		}
	}

	changedItem := &ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      newReason,
		Priority:    newPriority,
		DetectedAt:  detectedAt,
		Context:     newContext,
	}
	queue.Add(changedItem)

	// Get the updated item
	updatedItem, _ := queue.Get("test-session")

	// Verify timestamp HAS changed (status changed)
	if updatedItem.DetectedAt.Equal(initialTime) {
		t.Errorf("Expected timestamp to update when status changed, but it remained: %s",
			initialTime.Format(time.RFC3339))
	}

	// Verify the reason changed
	if updatedItem.Reason != newReason {
		t.Errorf("Expected reason to change to %s, got %s", newReason, updatedItem.Reason)
	}

	// Verify priority changed
	if updatedItem.Priority != newPriority {
		t.Errorf("Expected priority to change to %s, got %s", newPriority, updatedItem.Priority)
	}

	t.Logf("✓ Status change detected: New timestamp=%s, Reason=%s, Priority=%s",
		updatedItem.DetectedAt.Format(time.RFC3339),
		updatedItem.Reason,
		updatedItem.Priority)
}

// TestReviewQueuePoller_ContextChangeUpdatesTimestamp verifies that
// changes to the Context field also trigger a timestamp update.
func TestReviewQueuePoller_ContextChangeUpdatesTimestamp(t *testing.T) {
	// Create review queue
	queue := NewReviewQueue()

	// Manually add an item with initial context
	initialTime := time.Now().Add(-5 * time.Minute)
	item1 := &ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      ReasonIdleTimeout,
		Priority:    PriorityLow,
		DetectedAt:  initialTime,
		Context:     "Idle for 5 minutes",
	}
	queue.Add(item1)

	// Update with same reason/priority but different context
	item2 := &ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      ReasonIdleTimeout,
		Priority:    PriorityLow,
		DetectedAt:  time.Now(),            // New timestamp
		Context:     "Idle for 10 minutes", // Different context
	}

	// Simulate what the poller does: check existing item
	existingItem, exists := queue.Get("test-session")
	if !exists {
		t.Fatal("Expected item to exist in queue")
	}

	// Preserve timestamp if status unchanged
	if existingItem.Reason == item2.Reason &&
		existingItem.Priority == item2.Priority &&
		existingItem.Context == item2.Context {
		item2.DetectedAt = existingItem.DetectedAt
	}

	queue.Add(item2)

	// Get the updated item
	updatedItem, _ := queue.Get("test-session")

	// Since context changed, timestamp should be NEW (not preserved)
	if updatedItem.DetectedAt.Equal(initialTime) {
		t.Errorf("Expected timestamp to update when context changed, but it was preserved")
	}

	// Verify context was updated
	if updatedItem.Context != "Idle for 10 minutes" {
		t.Errorf("Expected context to be updated to 'Idle for 10 minutes', got '%s'", updatedItem.Context)
	}

	t.Logf("Context change correctly triggered timestamp update")
}

// TestReviewQueue_SortsByLastActivity verifies that review items are sorted
// by LastActivity timestamp, with most recent activity first (within same priority).
func TestReviewQueue_SortsByLastActivity(t *testing.T) {
	// Create review queue
	queue := NewReviewQueue()

	// Create three sessions with same priority but different LastActivity times
	now := time.Now()

	// Session 1: Last activity 5 days ago
	item1 := &ReviewItem{
		SessionID:    "session-old",
		SessionName:  "session-old",
		Reason:       ReasonInputRequired,
		Priority:     PriorityMedium,
		DetectedAt:   now.Add(-5 * 24 * time.Hour),
		Context:      "Waiting for input",
		LastActivity: now.Add(-5 * 24 * time.Hour), // 5 days ago
	}

	// Session 2: Last activity 6 days ago (oldest)
	item2 := &ReviewItem{
		SessionID:    "session-oldest",
		SessionName:  "session-oldest",
		Reason:       ReasonInputRequired,
		Priority:     PriorityMedium,
		DetectedAt:   now.Add(-6 * 24 * time.Hour),
		Context:      "Waiting for input",
		LastActivity: now.Add(-6 * 24 * time.Hour), // 6 days ago
	}

	// Session 3: Last activity 10 days ago but had recent activity
	item3 := &ReviewItem{
		SessionID:    "session-recent",
		SessionName:  "session-recent",
		Reason:       ReasonInputRequired,
		Priority:     PriorityMedium,
		DetectedAt:   now.Add(-10 * 24 * time.Hour),
		Context:      "Waiting for input",
		LastActivity: now.Add(-1 * time.Hour), // 1 hour ago (most recent)
	}

	// Add items in random order
	queue.Add(item2)
	queue.Add(item1)
	queue.Add(item3)

	// Get sorted list
	items := queue.List()

	// Verify we have all 3 items
	if len(items) != 3 {
		t.Fatalf("Expected 3 items in queue, got %d", len(items))
	}

	// Verify sorting: most recent activity should be first
	if items[0].SessionID != "session-recent" {
		t.Errorf("Expected first item to be 'session-recent' (most recent activity), got '%s'", items[0].SessionID)
	}

	if items[1].SessionID != "session-old" {
		t.Errorf("Expected second item to be 'session-old', got '%s'", items[1].SessionID)
	}

	if items[2].SessionID != "session-oldest" {
		t.Errorf("Expected third item to be 'session-oldest' (least recent activity), got '%s'", items[2].SessionID)
	}

	// Verify the LastActivity times are in correct order
	if !items[0].LastActivity.After(items[1].LastActivity) {
		t.Error("First item should have more recent LastActivity than second item")
	}

	if !items[1].LastActivity.After(items[2].LastActivity) {
		t.Error("Second item should have more recent LastActivity than third item")
	}

	t.Logf("✓ Review queue correctly sorted by LastActivity (most recent first)")
	t.Logf("  1. %s - Last activity: %s ago", items[0].SessionID, detection.FormatDuration(time.Since(items[0].LastActivity)))
	t.Logf("  2. %s - Last activity: %s ago", items[1].SessionID, detection.FormatDuration(time.Since(items[1].LastActivity)))
	t.Logf("  3. %s - Last activity: %s ago", items[2].SessionID, detection.FormatDuration(time.Since(items[2].LastActivity)))
}

// newSimpleTestPoller creates a ReviewQueuePoller wired with a fresh queue and status manager,
// with nil storage (no persistence). Safe to use in unit tests.
func newSimpleTestPoller() *ReviewQueuePoller {
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	return NewReviewQueuePoller(queue, statusMgr, nil)
}

// newSimpleTestPollerWithManager creates a ReviewQueuePoller and returns the concrete
// *InstanceStatusManager so tests can register controllers directly.
func newSimpleTestPollerWithManager() (*ReviewQueuePoller, *InstanceStatusManager) {
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	return NewReviewQueuePoller(queue, statusMgr, nil), statusMgr
}

// newTestPollerInstance creates a minimal started/paused Instance for use in poller tests.
func newTestPollerInstance(title, uuid string) *Instance {
	inst := &Instance{
		Title:   title,
		UUID:    uuid,
		Status:  Paused,
		Program: "claude",
	}
	inst.started.Store(true)
	return inst
}

// --- Instance management tests ---

// TestReviewQueuePoller_SetInstances_ReplacesAll verifies that SetInstances replaces
// all previously tracked instances with the provided slice.
func TestReviewQueuePoller_SetInstances_ReplacesAll(t *testing.T) {
	poller := newSimpleTestPoller()

	first := newTestPollerInstance("session-a", "uuid-a")
	second := newTestPollerInstance("session-b", "uuid-b")
	poller.SetInstances([]*Instance{first, second})

	replacement := newTestPollerInstance("session-c", "uuid-c")
	poller.SetInstances([]*Instance{replacement})

	got := poller.GetInstances()
	if len(got) != 1 {
		t.Fatalf("expected 1 instance after SetInstances, got %d", len(got))
	}
	if got[0].Title != "session-c" {
		t.Errorf("expected instance 'session-c', got %q", got[0].Title)
	}
}

// TestReviewQueuePoller_AddInstance_AppendsWithoutReplacing verifies that AddInstance
// appends a new instance without removing existing ones.
func TestReviewQueuePoller_AddInstance_AppendsWithoutReplacing(t *testing.T) {
	poller := newSimpleTestPoller()

	first := newTestPollerInstance("session-a", "uuid-a")
	poller.SetInstances([]*Instance{first})

	second := newTestPollerInstance("session-b", "uuid-b")
	poller.AddInstance(second)

	got := poller.GetInstances()
	if len(got) != 2 {
		t.Fatalf("expected 2 instances after AddInstance, got %d", len(got))
	}
}

// TestReviewQueuePoller_RemoveInstance_ByTitle verifies that RemoveInstance removes
// the instance matching the given title and leaves others intact.
func TestReviewQueuePoller_RemoveInstance_ByTitle(t *testing.T) {
	poller := newSimpleTestPoller()

	a := newTestPollerInstance("session-a", "uuid-a")
	b := newTestPollerInstance("session-b", "uuid-b")
	poller.SetInstances([]*Instance{a, b})

	poller.RemoveInstance("session-a")

	got := poller.GetInstances()
	if len(got) != 1 {
		t.Fatalf("expected 1 instance after RemoveInstance, got %d", len(got))
	}
	if got[0].Title != "session-b" {
		t.Errorf("expected remaining instance 'session-b', got %q", got[0].Title)
	}
}

// TestReviewQueuePoller_RemoveInstance_NotFound_NoError verifies that calling
// RemoveInstance with an unknown ID does not panic and leaves the list unchanged.
func TestReviewQueuePoller_RemoveInstance_NotFound_NoError(t *testing.T) {
	poller := newSimpleTestPoller()

	a := newTestPollerInstance("session-a", "uuid-a")
	poller.SetInstances([]*Instance{a})

	// Should not panic.
	poller.RemoveInstance("nonexistent")

	got := poller.GetInstances()
	if len(got) != 1 {
		t.Errorf("expected list unchanged (1 instance), got %d", len(got))
	}
}

// TestReviewQueuePoller_GetMonitoredCount verifies that GetMonitoredCount returns
// the number of currently tracked instances.
func TestReviewQueuePoller_GetMonitoredCount(t *testing.T) {
	poller := newSimpleTestPoller()

	if poller.GetMonitoredCount() != 0 {
		t.Fatalf("expected 0 monitored instances initially, got %d", poller.GetMonitoredCount())
	}

	poller.SetInstances([]*Instance{
		newTestPollerInstance("session-a", "uuid-a"),
		newTestPollerInstance("session-b", "uuid-b"),
		newTestPollerInstance("session-c", "uuid-c"),
	})

	if got := poller.GetMonitoredCount(); got != 3 {
		t.Errorf("expected 3 monitored instances, got %d", got)
	}
}

// TestReviewQueuePoller_FindInstance_ByTitle verifies that FindInstance returns
// the correct instance when looked up by title.
func TestReviewQueuePoller_FindInstance_ByTitle(t *testing.T) {
	poller := newSimpleTestPoller()

	a := newTestPollerInstance("session-a", "uuid-a")
	b := newTestPollerInstance("session-b", "uuid-b")
	poller.SetInstances([]*Instance{a, b})

	found := poller.FindInstance("session-a")
	if found == nil {
		t.Fatal("expected to find instance 'session-a', got nil")
	}
	if found.Title != "session-a" {
		t.Errorf("expected Title 'session-a', got %q", found.Title)
	}
}

// TestReviewQueuePoller_FindInstance_ByUUID verifies that FindInstance returns
// the correct instance when looked up by UUID.
func TestReviewQueuePoller_FindInstance_ByUUID(t *testing.T) {
	poller := newSimpleTestPoller()

	a := newTestPollerInstance("session-a", "uuid-a")
	b := newTestPollerInstance("session-b", "uuid-b")
	poller.SetInstances([]*Instance{a, b})

	found := poller.FindInstance("uuid-b")
	if found == nil {
		t.Fatal("expected to find instance by UUID 'uuid-b', got nil")
	}
	if found.UUID != "uuid-b" {
		t.Errorf("expected UUID 'uuid-b', got %q", found.UUID)
	}
}

// TestReviewQueuePoller_FindInstance_NotFound verifies that FindInstance returns
// nil when the given ID does not match any tracked instance.
func TestReviewQueuePoller_FindInstance_NotFound(t *testing.T) {
	poller := newSimpleTestPoller()

	poller.SetInstances([]*Instance{
		newTestPollerInstance("session-a", "uuid-a"),
	})

	found := poller.FindInstance("nonexistent")
	if found != nil {
		t.Errorf("expected nil for unknown ID, got %+v", found)
	}
}

// --- Lifecycle tests ---

// TestReviewQueuePoller_IsRunning_InitiallyFalse verifies that a newly created
// poller reports IsRunning() == false before Start is called.
func TestReviewQueuePoller_IsRunning_InitiallyFalse(t *testing.T) {
	poller := newSimpleTestPoller()

	if poller.IsRunning() {
		t.Error("expected IsRunning() == false before Start(), got true")
	}
}

// TestDefaultReviewQueuePollerConfig_should_return5MinStalenessThreshold_When_Called
// is a regression guard for ADR-001-staleness-threshold-recalibration.md: the
// StalenessThreshold default was silently reduced from 5min to 2min with no
// documented rationale (see the ADR's git-archaeology section), causing a
// 37/41 false-positive "Stale" badge rate live. This pins the recalibrated
// value so a future edit can't silently drift it again without updating the
// ADR.
func TestDefaultReviewQueuePollerConfig_should_return5MinStalenessThreshold_When_Called(t *testing.T) {
	cfg := DefaultReviewQueuePollerConfig()
	if cfg.StalenessThreshold != 5*time.Minute {
		t.Errorf("DefaultReviewQueuePollerConfig().StalenessThreshold = %s, want 5m", cfg.StalenessThreshold)
	}
}

// TestReviewQueuePoller_StartStop verifies that Start() transitions the poller to
// running and Stop() cleanly shuts it down.
func TestReviewQueuePoller_StartStop(t *testing.T) {
	// Use a fast poll interval so the goroutine does minimal work during the test.
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	cfg := DefaultReviewQueuePollerConfig()
	cfg.PollInterval = 100 * time.Millisecond
	cfg.ReconcileInterval = 0 // disable reconciliation to avoid tmux calls
	poller := NewReviewQueuePollerWithConfig(queue, statusMgr, nil, cfg)

	ctx := context.Background()
	poller.Start(ctx)
	t.Cleanup(poller.Stop)

	if !poller.IsRunning() {
		t.Error("expected IsRunning() == true after Start(), got false")
	}

	poller.Stop()

	// Wait up to 2s for the goroutine to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !poller.IsRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	if poller.IsRunning() {
		t.Error("expected IsRunning() == false after Stop(), still true after 2s")
	}
}

// TestReviewQueuePoller_Start_Idempotent verifies that calling Start() twice does
// not spawn a second goroutine or panic.
func TestReviewQueuePoller_Start_Idempotent(t *testing.T) {
	queue := NewReviewQueue()
	statusMgr := NewInstanceStatusManager()
	cfg := DefaultReviewQueuePollerConfig()
	cfg.PollInterval = 100 * time.Millisecond
	cfg.ReconcileInterval = 0
	poller := NewReviewQueuePollerWithConfig(queue, statusMgr, nil, cfg)

	ctx := context.Background()
	poller.Start(ctx)
	t.Cleanup(poller.Stop)

	// Second Start() should be a no-op (logged as "already started").
	poller.Start(ctx)

	if !poller.IsRunning() {
		t.Error("expected IsRunning() == true after double Start()")
	}
}

// TestReviewQueuePoller_AcknowledgedSession_RemovedOnNextPoll verifies that a session
// acknowledged after its last meaningful output is removed from the queue on the next poll.
// This is the regression test for the "skip button wipes list but doesn't remove status" bug.
func TestReviewQueuePoller_AcknowledgedSession_RemovedOnNextPoll(t *testing.T) {
	queue := NewReviewQueue()
	statusManager := NewInstanceStatusManager()
	poller := NewReviewQueuePollerWithConfig(queue, statusManager, nil, ReviewQueuePollerConfig{
		StalenessThreshold: 5 * time.Minute,
		IdleThreshold:      5 * time.Second,
	})

	inst := makeAcknowledgedInstance("acked-session")

	// Pre-populate queue to simulate session being visible before user clicked Skip.
	queue.Add(&ReviewItem{
		SessionID:   "acked-session",
		SessionName: "acked-session",
		Reason:      ReasonInputRequired,
		Priority:    PriorityMedium,
		DetectedAt:  time.Now().Add(-1 * time.Minute),
	})
	poller.AddInstance(inst)

	if _, exists := queue.Get("acked-session"); !exists {
		t.Fatal("precondition: session must be in queue before checkSession")
	}

	poller.checkSession(inst, nil)

	if _, exists := queue.Get("acked-session"); exists {
		t.Error("session should have been removed from queue after acknowledgment snooze")
	}
}

// TestReviewQueuePoller_AcknowledgedSession_ResurfacesAfterNewOutput verifies that
// a snoozed session re-enters the queue once new meaningful output arrives.
func TestReviewQueuePoller_AcknowledgedSession_ResurfacesAfterNewOutput(t *testing.T) {
	inst := makeAcknowledgedInstance("resurface-session")

	// Simulate new output arriving AFTER the acknowledgment.
	inst.LastMeaningfulOutput = time.Now().Add(-1 * time.Second) // newer than LastAcknowledged
	inst.SyncAtomicTimestamps()                                  // re-sync atomic shadow after direct field write

	// IsAcknowledgedAfterOutput should now return false — new output supersedes ack.
	if inst.IsAcknowledgedAfterOutput() {
		t.Error("session with new output after acknowledgment should NOT be considered snoozed")
	}
}

// TestReviewQueuePoller_ControllerSession_NotStarted_WithApproval_AddsToQueue verifies
// that sessions with GetController() != nil (controller wired but not yet started) are
// evaluated by the poller rather than skipped. When approval-prompt content is present in
// the terminal cache the session must appear in the review queue.
//
// This is the regression test for the bug where the early-return guard prevented any
// queue update for sessions with a non-nil controller, regardless of their actual state.
func TestReviewQueuePoller_ControllerSession_NotStarted_WithApproval_AddsToQueue(t *testing.T) {
	poller := newSimpleTestPoller()

	inst := &Instance{
		Title:  "controller-session",
		UUID:   "uuid-ctrl",
		Status: Running,
	}
	inst.started.Store(true)

	// Wire a controller that is NOT started (ctx == nil → IsStarted() = false).
	// GetController() returns non-nil, but IsControllerActive will be false so the
	// no-controller terminal-content path runs.
	bareController := &ClaudeController{
		sessionName: inst.Title,
		instance:    inst,
	}
	inst.controllerManager.SetController(bareController)

	// Pre-populate the content cache with an approval prompt so the no-controller
	// detection path finds it without needing a live tmux session.
	approvalContent := "Yes, allow reading /etc/hosts\nYes, allow once"
	poller.injectCachedContent(inst.Title, approvalContent)

	poller.AddInstance(inst)
	poller.checkSession(inst, nil)

	item, exists := poller.queue.Get(inst.Title)
	if !exists {
		t.Fatal("session with approval prompt must be added to the review queue")
	}
	if item.Reason != ReasonApprovalPending {
		t.Errorf("expected reason %s, got %s", ReasonApprovalPending, item.Reason)
	}
}

// TestReviewQueuePoller_ArchivedSession_ExcludedFromQueue verifies that a session with
// ArchivedAt set is skipped by shouldSkipSession and never added to the review queue —
// even when its cached terminal content would otherwise trigger an approval-pending
// detection. This is the regression test for the bug where archiveItemWorkSessions
// (server/services/backlog_service.go) sets Instance.ArchivedAt and kills the tmux pane
// during a backlog item reopen, but never sets Hidden or transitions Status to Stopped —
// so shouldSkipSession (which checked only Hidden/Status/Started) never excluded it, and
// the dead-paned session sat in the queue forever as a false ATTENTION_REASON_STALE entry.
// See docs/tasks/backlog-feature-improvement.md, 2026-08-02 entry.
func TestReviewQueuePoller_ArchivedSession_ExcludedFromQueue(t *testing.T) {
	poller := newSimpleTestPoller()

	inst := &Instance{
		Title:  "archived-session",
		UUID:   "uuid-archived",
		Status: Running,
	}
	inst.started.Store(true)
	archivedAt := time.Now().Add(-1 * time.Hour)
	inst.ArchivedAt = &archivedAt

	// Sanity check the fix directly: shouldSkipSession must report true for an
	// archived instance regardless of any other state.
	if !poller.shouldSkipSession(inst) {
		t.Fatal("shouldSkipSession must return true for an instance with ArchivedAt set")
	}

	// Pre-populate the content cache with an approval prompt — if the poller did NOT
	// skip archived sessions, this content would cause it to be added to the queue via
	// the same no-controller detection path exercised in
	// TestReviewQueuePoller_ControllerSession_NotStarted_WithApproval_AddsToQueue.
	approvalContent := "Yes, allow reading /etc/hosts\nYes, allow once"
	poller.injectCachedContent(inst.Title, approvalContent)

	poller.AddInstance(inst)
	poller.checkSession(inst, nil)

	if _, exists := poller.queue.Get(inst.Title); exists {
		t.Error("archived session must never be added to the review queue")
	}
}

// TestReviewQueuePoller_ControllerSession_Started_NeedsApproval_AddsToQueue verifies that
// sessions with an active (started) ClaudeController that reports StatusNeedsApproval are
// added to the review queue via the controller-based detection path (lines 696-828).
func TestReviewQueuePoller_ControllerSession_Started_NeedsApproval_AddsToQueue(t *testing.T) {
	poller, statusMgr := newSimpleTestPollerWithManager()

	// Use a mock InstanceContext so GetCurrentStatus() returns StatusNeedsApproval
	// without requiring a real tmux session.
	approvalContent := "Yes, allow reading /home/user/file.go\nYes, allow once"
	ctrl, _ := newControllerWithMock(approvalContent)

	inst := &Instance{
		Title:  "active-controller-session",
		UUID:   "uuid-active",
		Status: Running,
	}
	inst.started.Store(true)
	ctrl.sessionName = inst.Title

	// Mark the controller as started by setting a non-nil context.
	ctrl.lifecycle.Write(func(l *controllerLifecycle) { l.ctx = t.Context() })

	inst.controllerManager.SetController(ctrl)
	statusMgr.RegisterController(inst.Title, ctrl)

	poller.AddInstance(inst)
	poller.checkSession(inst, nil)

	item, exists := poller.queue.Get(inst.Title)
	if !exists {
		t.Fatal("session with active controller reporting NeedsApproval must be in the review queue")
	}
	if item.Reason != ReasonApprovalPending {
		t.Errorf("expected reason %s, got %s", ReasonApprovalPending, item.Reason)
	}
	if item.Priority != PriorityHigh {
		t.Errorf("expected priority %s, got %s", PriorityHigh, item.Priority)
	}
}

// TestReviewQueuePoller_AcknowledgmentSnooze_ConditionLogic documents the bypass that
// caused the bug and asserts the corrected condition applies universally.
func TestReviewQueuePoller_AcknowledgmentSnooze_ConditionLogic(t *testing.T) {
	cases := []struct {
		name               string
		shouldAdd          bool
		priority           Priority
		isControllerActive bool
		wantOldBypassSkip  bool // true = old code SKIPPED the snooze (the bug)
	}{
		{
			name:               "input-required medium-priority active-controller (the bug scenario)",
			shouldAdd:          true,
			priority:           PriorityMedium,
			isControllerActive: true,
			wantOldBypassSkip:  true, // old code bypassed snooze → bug
		},
		{
			name:               "error-state urgent active-controller",
			shouldAdd:          true,
			priority:           PriorityUrgent,
			isControllerActive: true,
			wantOldBypassSkip:  true, // old code bypassed snooze → bug
		},
		{
			name:               "stale low-priority no-controller",
			shouldAdd:          true,
			priority:           PriorityLow,
			isControllerActive: false,
			wantOldBypassSkip:  false, // old code entered snooze block — worked correctly
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reproduce the old condition that caused the bypass.
			// oldCondition=true  → snooze block is entered (not bypassed)
			// oldCondition=false → snooze block is skipped (bypassed = the bug)
			oldCondition := !tc.shouldAdd || tc.priority == PriorityLow || !tc.isControllerActive
			oldBypassed := !oldCondition
			if oldBypassed != tc.wantOldBypassSkip {
				t.Errorf("expected old-code bypass=%v for scenario %q, got bypass=%v (oldCondition=%v)",
					tc.wantOldBypassSkip, tc.name, oldBypassed, oldCondition)
			}

			// After the fix, IsAcknowledgedAfterOutput is checked unconditionally.
			// Verify the session state correctly reports "snoozed" after ack.
			inst := makeAcknowledgedInstance("test")
			if !inst.IsAcknowledgedAfterOutput() {
				t.Error("acknowledged session should report IsAcknowledgedAfterOutput=true")
			}
		})
	}
}

// makeStaleInstance creates a minimal Instance that will reach the staleness-check
// path in checkSession without calling tmux or git. The content cache is pre-warmed
// so getContent returns immediately without spawning a subprocess.
func makeStaleInstance(rqp *ReviewQueuePoller, title string) *Instance {
	inst := &Instance{
		Title:  title,
		Status: Running,
	}
	inst.started.Store(true)
	// CreatedAt well in the past → GetTimeSinceLastMeaningfulOutput() > StalenessThreshold
	inst.CreatedAt = time.Now().Add(-10 * time.Minute)
	inst.UpdatedAt = inst.CreatedAt

	// Pre-warm content cache so getContent returns without calling inst.Preview() (tmux).
	rqp.injectCachedContent(title, "")

	return inst
}

// BenchmarkCheckSessionsConcurrent exercises the concurrent hot path of checkSessions
// with N stale sessions. It is the regression gate for hot-path log-mutex contention:
// if InfoLog.Printf calls are re-added to the staleness section of checkSession, the
// concurrent goroutines will serialise on the log mutex and ns/op will increase sharply.
//
// Run with: go test -bench=BenchmarkCheckSessionsConcurrent -benchmem ./session/
func BenchmarkCheckSessionsConcurrent(b *testing.B) {
	for _, n := range []int{1, 5, 10, 20} {
		b.Run(fmt.Sprintf("sessions-%d", n), func(b *testing.B) {
			queue := NewReviewQueue()
			statusMgr := NewInstanceStatusManager()
			cfg := DefaultReviewQueuePollerConfig()
			cfg.StalenessThreshold = 1 * time.Minute // ensure all instances are stale
			rqp := NewReviewQueuePollerWithConfig(queue, statusMgr, nil, cfg)

			instances := make([]*Instance, n)
			for i := 0; i < n; i++ {
				instances[i] = makeStaleInstance(rqp, fmt.Sprintf("bench-session-%d", i))
			}
			rqp.SetInstances(instances)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rqp.checkSessions()
			}
		})
	}
}

// --- reconcileSessions multi-socket regression tests ---
//
// Instances can be spread across multiple tmux server sockets. reconcileSessions
// previously derived a single socket from the first managed instance that had one
// set and queried tmux.ListAllSessions with only that socket, applying its result to
// every instance regardless of the instance's own socket. An instance on a different
// socket than the one picked would be checked against the wrong socket's live-session
// set -- falsely "not found" (if actually alive on its own socket) or falsely "alive"
// (if a same-named session happened to exist on the picked socket) -- causing sessions
// to flap between Active and Stopped depending on iteration order. These tests pin the
// fixed behavior: each instance's liveness is resolved against its own socket only.

// TestReviewQueuePoller_ReconcileSessions_ActiveInstancesOnDifferentSockets_StayIndependent
// is the direct regression test for the bug: two Active instances on different sockets,
// both genuinely alive on their own socket. Under the old single-socket assumption,
// whichever socket was NOT picked would see its instance falsely marked Stopped.
func TestReviewQueuePoller_ReconcileSessions_ActiveInstancesOnDifferentSockets_StayIndependent(t *testing.T) {
	poller := newSimpleTestPoller()
	querier := newFakeTmuxSocketQuerier()
	poller.tmuxSocket = querier

	instDefault := makeSocketTestInstance("default-socket-session", "session-default", "", Active)
	instCustom := makeSocketTestInstance("custom-socket-session", "session-custom", "custom", Active)
	poller.SetInstances([]*Instance{instDefault, instCustom})

	// Each session is alive, but only on its own socket.
	querier.setLiveSessions("", "session-default")
	querier.setLiveSessions("custom", "session-custom")

	poller.reconcileSessions()

	if instDefault.Status != Active {
		t.Errorf("default-socket instance: got status %v, want Active (it IS alive on its own socket \"\")", instDefault.Status)
	}
	if instCustom.Status != Active {
		t.Errorf("custom-socket instance: got status %v, want Active (it IS alive on its own socket \"custom\")", instCustom.Status)
	}

	sockets := querier.socketsQueried()
	if len(sockets) != 2 {
		t.Fatalf("expected both sockets to be queried independently, got %v", sockets)
	}
}

// TestReviewQueuePoller_ReconcileSessions_StoppedInstancesOnDifferentSockets_ReviveIndependently
// covers the Stopped→Active direction: only the instance actually alive on its own
// socket should revive; the other must stay Stopped.
func TestReviewQueuePoller_ReconcileSessions_StoppedInstancesOnDifferentSockets_ReviveIndependently(t *testing.T) {
	poller := newSimpleTestPoller()
	querier := newFakeTmuxSocketQuerier()
	poller.tmuxSocket = querier

	instDefault := makeSocketTestInstance("default-socket-session", "session-default", "", Stopped)
	instCustom := makeSocketTestInstance("custom-socket-session", "session-custom", "custom", Stopped)
	poller.SetInstances([]*Instance{instDefault, instCustom})

	// Default-socket session came back; custom-socket session is still gone.
	querier.setLiveSessions("", "session-default")
	querier.setLiveSessions("custom") // empty: nothing alive there

	poller.reconcileSessions()

	if instDefault.Status != Active {
		t.Errorf("default-socket instance: got status %v, want Active (revived)", instDefault.Status)
	}
	if instCustom.Status != Stopped {
		t.Errorf("custom-socket instance: got status %v, want Stopped (still not found on its own socket)", instCustom.Status)
	}
}

// TestReviewQueuePoller_ReconcileSessions_ServerDownOnOneSocket_DoesNotAffectOthers
// verifies that a down tmux server on one socket only skips reconciliation for
// instances on that socket, not for instances on other, healthy sockets.
func TestReviewQueuePoller_ReconcileSessions_ServerDownOnOneSocket_DoesNotAffectOthers(t *testing.T) {
	poller := newSimpleTestPoller()
	querier := newFakeTmuxSocketQuerier()
	poller.tmuxSocket = querier

	instDefault := makeSocketTestInstance("default-socket-session", "session-default", "", Active)
	instCustom := makeSocketTestInstance("custom-socket-session", "session-custom", "custom", Active)
	poller.SetInstances([]*Instance{instDefault, instCustom})

	querier.setLiveSessions("", "session-default")
	querier.setDown("custom", true)

	poller.reconcileSessions()

	if instDefault.Status != Active {
		t.Errorf("default-socket instance: got status %v, want Active (its own socket is healthy)", instDefault.Status)
	}
	// custom's server is down, so reconciliation for it is skipped this pass -- it
	// must NOT be marked Stopped just because its socket happened to be unreachable.
	if instCustom.Status != Active {
		t.Errorf("custom-socket instance: got status %v, want Active (server-down must skip, not falsely mark Stopped)", instCustom.Status)
	}
}

// stubApprovalMetadataProvider is a minimal ApprovalMetadataProvider for tests. It records
// each key it was queried with so tests can assert lookup order.
type stubApprovalMetadataProvider struct {
	bySessionID map[string][]ApprovalMetadata
	queried     []string
}

func (s *stubApprovalMetadataProvider) GetApprovalMetadataBySession(sessionID string) []ApprovalMetadata {
	s.queried = append(s.queried, sessionID)
	return s.bySessionID[sessionID]
}

// TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID is the regression test for the bug
// fixed alongside this test: ApprovalHandler.resolveSessionID stores PendingApproval.SessionID
// keyed by the session's UUID (Title only as a fallback when no UUID exists — see
// approval_handler.go's stableIDForData), but checkSession's enrichment step queried the
// provider by snap.Title. That mismatch silently dropped pending_approval_id (and therefore
// the escalation reason) from every real queue item. This test seeds the stub provider ONLY
// under the UUID key — pre-fix, the first (and only) lookup by Title would find nothing and
// the escalation-reason fields would be absent from item.Metadata.
func TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID(t *testing.T) {
	poller, statusMgr := newSimpleTestPollerWithManager()

	approvalContent := "Yes, allow reading /etc/hosts\nYes, allow once"
	ctrl, _ := newControllerWithMock(approvalContent)

	inst := &Instance{
		Title:  "session-title-not-used-as-key",
		UUID:   "session-uuid-1234",
		Status: Running,
	}
	inst.started.Store(true)
	ctrl.sessionName = inst.Title
	ctrl.lifecycle.Write(func(l *controllerLifecycle) { l.ctx = t.Context() })
	inst.controllerManager.SetController(ctrl)
	statusMgr.RegisterController(inst.Title, ctrl)

	provider := &stubApprovalMetadataProvider{
		bySessionID: map[string][]ApprovalMetadata{
			inst.UUID: {{
				ApprovalID:         "approval-1",
				ToolName:           "Bash",
				EscalationReason:   "No matching rule; escalated for manual review.",
				EscalationCategory: "no-match",
			}},
		},
	}
	poller.SetApprovalProvider(provider)

	poller.AddInstance(inst)
	poller.checkSession(inst, nil)

	item, exists := poller.queue.Get(inst.Title)
	if !exists {
		t.Fatal("session with active controller reporting NeedsApproval must be in the review queue")
	}
	if got := item.Metadata["pending_approval_id"]; got != "approval-1" {
		t.Errorf("pending_approval_id = %q, want %q (queried keys: %v)", got, "approval-1", provider.queried)
	}
	if got := item.Metadata["escalation_reason"]; got != "No matching rule; escalated for manual review." {
		t.Errorf("escalation_reason = %q, want the seeded reason (queried keys: %v)", got, provider.queried)
	}
	if got := item.Metadata["escalation_reason_category"]; got != "no-match" {
		t.Errorf("escalation_reason_category = %q, want %q", got, "no-match")
	}
}

// TestReviewQueuePoller_EnrichesApprovalMetadata_ByTitleFallback covers the second half of
// checkSession's UUID-then-Title lookup: when a session has no UUID (or the UUID lookup
// misses), the provider must still be queried by Title so approvals keyed the old/fallback
// way are found. Seeds the stub ONLY under the Title key — pre-fix (or if this fallback were
// ever removed) the UUID-only lookup would miss and no metadata would be attached.
func TestReviewQueuePoller_EnrichesApprovalMetadata_ByTitleFallback(t *testing.T) {
	poller, statusMgr := newSimpleTestPollerWithManager()

	approvalContent := "Yes, allow reading /etc/hosts\nYes, allow once"
	ctrl, _ := newControllerWithMock(approvalContent)

	inst := &Instance{
		Title:  "session-with-no-uuid",
		UUID:   "", // no stable UUID — resolveSessionID would have fallen back to Title too
		Status: Running,
	}
	inst.started.Store(true)
	ctrl.sessionName = inst.Title
	ctrl.lifecycle.Write(func(l *controllerLifecycle) { l.ctx = t.Context() })
	inst.controllerManager.SetController(ctrl)
	statusMgr.RegisterController(inst.Title, ctrl)

	provider := &stubApprovalMetadataProvider{
		bySessionID: map[string][]ApprovalMetadata{
			inst.Title: {{
				ApprovalID:         "approval-title-fallback",
				EscalationReason:   "No matching rule; escalated for manual review.",
				EscalationCategory: "no-match",
			}},
		},
	}
	poller.SetApprovalProvider(provider)

	poller.AddInstance(inst)
	poller.checkSession(inst, nil)

	item, exists := poller.queue.Get(inst.Title)
	if !exists {
		t.Fatal("session with active controller reporting NeedsApproval must be in the review queue")
	}
	if got := item.Metadata["pending_approval_id"]; got != "approval-title-fallback" {
		t.Errorf("pending_approval_id = %q, want %q (queried keys: %v)", got, "approval-title-fallback", provider.queried)
	}
}
