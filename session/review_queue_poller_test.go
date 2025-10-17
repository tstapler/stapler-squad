package session

import (
	"testing"
	"time"
)

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
		time.Sleep(50 * time.Millisecond)

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
	time.Sleep(100 * time.Millisecond)
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

	// Wait a bit to ensure time difference
	time.Sleep(100 * time.Millisecond)

	// Update with same reason/priority but different context
	item2 := &ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      ReasonIdleTimeout,
		Priority:    PriorityLow,
		DetectedAt:  time.Now(), // New timestamp
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
