package session

import (
	"fmt"
	"testing"
	"time"
)

// TestReviewQueueWorkflow demonstrates a typical review queue usage pattern
func TestReviewQueueWorkflow(t *testing.T) {
	// Create a review queue
	queue := NewReviewQueue()

	// Simulate three sessions needing attention
	queue.Add(&ReviewItem{
		SessionID:   "web-app",
		SessionName: "Web App Development",
		Reason:      ReasonApprovalPending,
		Priority:    PriorityHigh,
		DetectedAt:  time.Now(),
		Context:     "Would you like me to proceed with the database migration?",
		PatternName: "approval_dialog",
	})

	queue.Add(&ReviewItem{
		SessionID:   "api-service",
		SessionName: "API Service",
		Reason:      ReasonErrorState,
		Priority:    PriorityUrgent,
		DetectedAt:  time.Now().Add(-5 * time.Minute),
		Context:     "Error: Failed to connect to database",
		PatternName: "error_message",
	})

	queue.Add(&ReviewItem{
		SessionID:   "backend-tests",
		SessionName: "Backend Tests",
		Reason:      ReasonTaskComplete,
		Priority:    PriorityLow,
		DetectedAt:  time.Now().Add(-10 * time.Minute),
		Context:     "All tests passed successfully",
		PatternName: "task_complete",
	})

	// Verify queue size
	if queue.Count() != 3 {
		t.Errorf("Expected 3 items in queue, got %d", queue.Count())
	}

	// Get items sorted by priority (most urgent first)
	items := queue.List()

	fmt.Println("\n=== Review Queue ===")
	for i, item := range items {
		fmt.Printf("%d. %s %s [%s]\n", i+1, item.Priority.Emoji(), item.SessionName, item.Priority.String())
		fmt.Printf("   Reason: %s\n", item.Reason.String())
		fmt.Printf("   Context: %s\n", item.Context)
		fmt.Printf("   Detected: %v ago\n\n", time.Since(item.DetectedAt).Round(time.Second))
	}

	// Verify priority order
	if items[0].Priority != PriorityUrgent {
		t.Error("First item should be urgent (error state)")
	}
	if items[1].Priority != PriorityHigh {
		t.Error("Second item should be high priority (approval)")
	}
	if items[2].Priority != PriorityLow {
		t.Error("Third item should be low priority (task complete)")
	}

	// Test navigation
	nextID, _ := queue.Next("")
	if nextID != "api-service" {
		t.Errorf("Next() from empty should return highest priority item (api-service), got %s", nextID)
	}

	nextID, _ = queue.Next("api-service")
	if nextID != "web-app" {
		t.Errorf("Next() from api-service should return web-app, got %s", nextID)
	}

	// Test statistics
	stats := queue.GetStatistics()
	fmt.Printf("=== Queue Statistics ===\n")
	fmt.Printf("Total Items: %d\n", stats.TotalItems)
	fmt.Printf("By Priority: %v\n", stats.ByPriority)
	fmt.Printf("By Reason: %v\n", stats.ByReason)
	fmt.Printf("Average Age: %v\n", stats.AverageAge.Round(time.Second))
	fmt.Printf("Oldest Item: %s (%v old)\n\n", stats.OldestItem, stats.OldestAge.Round(time.Second))

	// Simulate resolving the error
	queue.Remove("api-service")
	fmt.Printf("Removed error state item. Queue now has %d items\n", queue.Count())
}

// TestReviewQueueIntegrationWithInstance demonstrates queue integration with instances
func TestReviewQueueIntegrationWithInstance(t *testing.T) {
	// Create a shared review queue
	queue := NewReviewQueue()

	// Create a session instance
	inst := &Instance{
		Title:  "test-session",
		Status: Running,
	}

	// Wire up the review queue
	inst.SetReviewQueue(queue)

	// Verify instance is not initially in review queue
	if inst.NeedsReview() {
		t.Error("Instance should not initially need review")
	}

	// Add instance to review queue
	queue.Add(&ReviewItem{
		SessionID:   inst.Title,
		SessionName: inst.Title,
		Reason:      ReasonInputRequired,
		Priority:    PriorityMedium,
		DetectedAt:  time.Now(),
		Context:     "Waiting for user input",
	})

	// Verify instance now needs review
	if !inst.NeedsReview() {
		t.Error("Instance should now need review")
	}

	// Get review item
	item, ok := inst.GetReviewItem()
	if !ok {
		t.Fatal("Should have review item")
	}

	fmt.Printf("\n=== Instance Review Status ===\n")
	fmt.Printf("Session: %s\n", inst.Title)
	fmt.Printf("Badge: %s\n", item.Priority.Emoji())
	fmt.Printf("Reason: %s\n", item.Reason.String())
	fmt.Printf("Priority: %s\n", item.Priority.String())
	fmt.Printf("Context: %s\n", item.Context)
}
