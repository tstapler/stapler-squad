package session

import (
	"testing"
	"time"
)

func TestReviewQueue_AddAndGet(t *testing.T) {
	rq := NewReviewQueue()

	item := &ReviewItem{
		SessionID:   "session-1",
		SessionName: "Test Session",
		Reason:      ReasonApprovalPending,
		Priority:    PriorityHigh,
		DetectedAt:  time.Now(),
		Context:     "Would you like to proceed?",
	}

	// Add item
	isNew := rq.Add(item)
	if !isNew {
		t.Error("Expected Add to return true for new item")
	}

	// Get item
	retrieved, exists := rq.Get("session-1")
	if !exists {
		t.Fatal("Expected item to exist in queue")
	}

	if retrieved.SessionID != item.SessionID {
		t.Errorf("Expected SessionID %s, got %s", item.SessionID, retrieved.SessionID)
	}
}

func TestReviewQueue_Remove(t *testing.T) {
	rq := NewReviewQueue()

	item := &ReviewItem{
		SessionID: "session-1",
		Reason:    ReasonApprovalPending,
		Priority:  PriorityHigh,
	}

	rq.Add(item)

	// Remove item
	removed := rq.Remove("session-1")
	if !removed {
		t.Error("Expected Remove to return true")
	}

	// Verify removed
	_, exists := rq.Get("session-1")
	if exists {
		t.Error("Expected item to be removed from queue")
	}

	// Remove non-existent item
	removed = rq.Remove("session-2")
	if removed {
		t.Error("Expected Remove to return false for non-existent item")
	}
}

func TestReviewQueue_Has(t *testing.T) {
	rq := NewReviewQueue()

	if rq.Has("session-1") {
		t.Error("Expected Has to return false for empty queue")
	}

	item := &ReviewItem{
		SessionID: "session-1",
		Reason:    ReasonApprovalPending,
		Priority:  PriorityHigh,
	}

	rq.Add(item)

	if !rq.Has("session-1") {
		t.Error("Expected Has to return true for added item")
	}

	rq.Remove("session-1")

	if rq.Has("session-1") {
		t.Error("Expected Has to return false after removal")
	}
}

func TestReviewQueue_List_SortsByPriority(t *testing.T) {
	rq := NewReviewQueue()

	// Add items with different priorities
	items := []*ReviewItem{
		{SessionID: "low", Priority: PriorityLow, DetectedAt: time.Now()},
		{SessionID: "urgent", Priority: PriorityUrgent, DetectedAt: time.Now()},
		{SessionID: "medium", Priority: PriorityMedium, DetectedAt: time.Now()},
		{SessionID: "high", Priority: PriorityHigh, DetectedAt: time.Now()},
	}

	for _, item := range items {
		rq.Add(item)
	}

	// Get sorted list
	sorted := rq.List()

	if len(sorted) != 4 {
		t.Fatalf("Expected 4 items, got %d", len(sorted))
	}

	// Verify order: Urgent > High > Medium > Low
	expectedOrder := []string{"urgent", "high", "medium", "low"}
	for i, expected := range expectedOrder {
		if sorted[i].SessionID != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, sorted[i].SessionID)
		}
	}
}

func TestReviewQueue_List_SortsByTime(t *testing.T) {
	rq := NewReviewQueue()

	now := time.Now()

	// Add items with same priority but different LastActivity times
	items := []*ReviewItem{
		{SessionID: "newer", Priority: PriorityHigh, DetectedAt: now, LastActivity: now.Add(2 * time.Second)},
		{SessionID: "oldest", Priority: PriorityHigh, DetectedAt: now, LastActivity: now},
		{SessionID: "older", Priority: PriorityHigh, DetectedAt: now, LastActivity: now.Add(1 * time.Second)},
	}

	for _, item := range items {
		rq.Add(item)
	}

	sorted := rq.List()

	// Should be sorted by LastActivity (most recent first) when priorities are equal
	if sorted[0].SessionID != "newer" {
		t.Errorf("Expected newer item first (most recent activity), got %s", sorted[0].SessionID)
	}
	if sorted[1].SessionID != "older" {
		t.Errorf("Expected older item second, got %s", sorted[1].SessionID)
	}
	if sorted[2].SessionID != "oldest" {
		t.Errorf("Expected oldest item third (least recent activity), got %s", sorted[2].SessionID)
	}
}

func TestReviewQueue_Count(t *testing.T) {
	rq := NewReviewQueue()

	if rq.Count() != 0 {
		t.Error("Expected empty queue to have count 0")
	}

	rq.Add(&ReviewItem{SessionID: "session-1", Priority: PriorityHigh})
	rq.Add(&ReviewItem{SessionID: "session-2", Priority: PriorityMedium})

	if rq.Count() != 2 {
		t.Errorf("Expected count 2, got %d", rq.Count())
	}

	rq.Remove("session-1")

	if rq.Count() != 1 {
		t.Errorf("Expected count 1 after removal, got %d", rq.Count())
	}
}

func TestReviewQueue_CountByPriority(t *testing.T) {
	rq := NewReviewQueue()

	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh})
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityHigh})
	rq.Add(&ReviewItem{SessionID: "s3", Priority: PriorityMedium})

	counts := rq.CountByPriority()

	if counts[PriorityHigh] != 2 {
		t.Errorf("Expected 2 high priority items, got %d", counts[PriorityHigh])
	}
	if counts[PriorityMedium] != 1 {
		t.Errorf("Expected 1 medium priority item, got %d", counts[PriorityMedium])
	}
	if counts[PriorityLow] != 0 {
		t.Errorf("Expected 0 low priority items, got %d", counts[PriorityLow])
	}
}

func TestReviewQueue_CountByReason(t *testing.T) {
	rq := NewReviewQueue()

	rq.Add(&ReviewItem{SessionID: "s1", Reason: ReasonApprovalPending})
	rq.Add(&ReviewItem{SessionID: "s2", Reason: ReasonApprovalPending})
	rq.Add(&ReviewItem{SessionID: "s3", Reason: ReasonErrorState})

	counts := rq.CountByReason()

	if counts[ReasonApprovalPending] != 2 {
		t.Errorf("Expected 2 approval pending items, got %d", counts[ReasonApprovalPending])
	}
	if counts[ReasonErrorState] != 1 {
		t.Errorf("Expected 1 error state item, got %d", counts[ReasonErrorState])
	}
}

func TestReviewQueue_Next(t *testing.T) {
	rq := NewReviewQueue()

	// Empty queue
	_, ok := rq.Next("")
	if ok {
		t.Error("Expected Next to return false for empty queue")
	}

	// Add items
	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh, DetectedAt: time.Now()})
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityMedium, DetectedAt: time.Now()})
	rq.Add(&ReviewItem{SessionID: "s3", Priority: PriorityLow, DetectedAt: time.Now()})

	// Next from empty (should return first)
	next, ok := rq.Next("")
	if !ok || next != "s1" {
		t.Errorf("Expected first item 's1', got '%s'", next)
	}

	// Next from s1
	next, ok = rq.Next("s1")
	if !ok || next != "s2" {
		t.Errorf("Expected 's2', got '%s'", next)
	}

	// Next from last item (should wrap)
	next, ok = rq.Next("s3")
	if !ok || next != "s1" {
		t.Errorf("Expected wrap to 's1', got '%s'", next)
	}

	// Next from non-existent (should return first)
	next, ok = rq.Next("non-existent")
	if !ok || next != "s1" {
		t.Errorf("Expected first item 's1' for non-existent, got '%s'", next)
	}
}

func TestReviewQueue_Previous(t *testing.T) {
	rq := NewReviewQueue()

	// Add items
	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh, DetectedAt: time.Now()})
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityMedium, DetectedAt: time.Now()})
	rq.Add(&ReviewItem{SessionID: "s3", Priority: PriorityLow, DetectedAt: time.Now()})

	// Previous from empty (should return first)
	prev, ok := rq.Previous("")
	if !ok || prev != "s1" {
		t.Errorf("Expected first item 's1', got '%s'", prev)
	}

	// Previous from s2
	prev, ok = rq.Previous("s2")
	if !ok || prev != "s1" {
		t.Errorf("Expected 's1', got '%s'", prev)
	}

	// Previous from first item (should wrap to last)
	prev, ok = rq.Previous("s1")
	if !ok || prev != "s3" {
		t.Errorf("Expected wrap to 's3', got '%s'", prev)
	}
}

func TestReviewQueue_Clear(t *testing.T) {
	rq := NewReviewQueue()

	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh})
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityMedium})

	rq.Clear()

	if rq.Count() != 0 {
		t.Errorf("Expected empty queue after Clear, got count %d", rq.Count())
	}
}

func TestReviewQueue_Update(t *testing.T) {
	rq := NewReviewQueue()

	item1 := &ReviewItem{
		SessionID: "session-1",
		Priority:  PriorityHigh,
		Context:   "Original context",
	}

	isNew := rq.Add(item1)
	if !isNew {
		t.Error("Expected first add to return true")
	}

	// Update with new item
	item2 := &ReviewItem{
		SessionID: "session-1",
		Priority:  PriorityUrgent,
		Context:   "Updated context",
	}

	isNew = rq.Add(item2)
	if isNew {
		t.Error("Expected update to return false")
	}

	// Verify update
	retrieved, _ := rq.Get("session-1")
	if retrieved.Priority != PriorityUrgent {
		t.Errorf("Expected priority to be updated to Urgent, got %v", retrieved.Priority)
	}
	if retrieved.Context != "Updated context" {
		t.Errorf("Expected context to be updated, got %s", retrieved.Context)
	}
}

func TestReviewQueue_GetStatistics(t *testing.T) {
	rq := NewReviewQueue()

	now := time.Now()
	old := now.Add(-5 * time.Minute)

	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh, Reason: ReasonApprovalPending, DetectedAt: now})
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityMedium, Reason: ReasonInputRequired, DetectedAt: old})

	stats := rq.GetStatistics()

	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", stats.TotalItems)
	}

	if stats.OldestItem != "s2" {
		t.Errorf("Expected oldest item to be 's2', got '%s'", stats.OldestItem)
	}

	if stats.OldestAge < 4*time.Minute {
		t.Errorf("Expected oldest age > 4 minutes, got %v", stats.OldestAge)
	}
}

func TestReviewQueue_GetStatistics_OldestTimeCalculation(t *testing.T) {
	rq := NewReviewQueue()

	now := time.Now()

	// Add items with different ages
	rq.Add(&ReviewItem{
		SessionID:  "old",
		Priority:   PriorityLow,
		Reason:     ReasonTaskComplete,
		DetectedAt: now.Add(-10 * time.Minute), // 10 minutes ago
	})
	rq.Add(&ReviewItem{
		SessionID:  "newer",
		Priority:   PriorityLow,
		Reason:     ReasonTaskComplete,
		DetectedAt: now.Add(-5 * time.Minute), // 5 minutes ago
	})

	stats := rq.GetStatistics()

	// Oldest item should be "old" (10 minutes)
	if stats.OldestItem != "old" {
		t.Errorf("Expected oldest item 'old', got '%s'", stats.OldestItem)
	}

	// Oldest age should be approximately 10 minutes
	expectedAge := 10 * time.Minute
	tolerance := 2 * time.Second
	ageDiff := stats.OldestAge - expectedAge
	if ageDiff < 0 {
		ageDiff = -ageDiff
	}
	if ageDiff > tolerance {
		t.Errorf("Expected oldest age ~10m (±2s), got %v", stats.OldestAge)
	}

	// Average age should be approximately 7.5 minutes
	expectedAvg := 7*time.Minute + 30*time.Second
	avgDiff := stats.AverageAge - expectedAvg
	if avgDiff < 0 {
		avgDiff = -avgDiff
	}
	if avgDiff > tolerance {
		t.Errorf("Expected average age ~7.5m (±2s), got %v", stats.AverageAge)
	}

	// Total items should include both
	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", stats.TotalItems)
	}
}

func TestReviewQueue_GetStatistics_WithZeroTimestamp(t *testing.T) {
	rq := NewReviewQueue()

	now := time.Now()

	// Add item with zero timestamp - should be reset to current time by Add()
	zeroItem := &ReviewItem{
		SessionID:  "zero-time",
		Priority:   PriorityMedium,
		Reason:     ReasonInputRequired,
		DetectedAt: time.Time{}, // Zero time
	}
	rq.Add(zeroItem)

	// Add item with valid timestamp
	rq.Add(&ReviewItem{
		SessionID:  "valid",
		Priority:   PriorityHigh,
		Reason:     ReasonApprovalPending,
		DetectedAt: now.Add(-2 * time.Minute),
	})

	stats := rq.GetStatistics()

	// Should have 2 total items
	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", stats.TotalItems)
	}

	// Oldest item should be "valid" since zero-time was reset to now
	if stats.OldestItem != "valid" {
		t.Errorf("Expected oldest item 'valid' (zero timestamp should be ignored), got '%s'", stats.OldestItem)
	}

	// Oldest age should be approximately 2 minutes
	expectedAge := 2 * time.Minute
	tolerance := 2 * time.Second
	ageDiff := stats.OldestAge - expectedAge
	if ageDiff < 0 {
		ageDiff = -ageDiff
	}
	if ageDiff > tolerance {
		t.Errorf("Expected oldest age ~2m (±2s), got %v", stats.OldestAge)
	}
}

func TestReviewQueue_GetStatistics_WithInvalidTimestamp(t *testing.T) {
	rq := NewReviewQueue()

	now := time.Now()

	// Add item with very old (invalid) timestamp - should be reset to current time
	invalidItem := &ReviewItem{
		SessionID:  "invalid",
		Priority:   PriorityLow,
		Reason:     ReasonIdleTimeout,
		DetectedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), // Before 2020
	}
	rq.Add(invalidItem)

	// Add item with valid timestamp
	rq.Add(&ReviewItem{
		SessionID:  "valid",
		Priority:   PriorityMedium,
		Reason:     ReasonInputRequired,
		DetectedAt: now.Add(-3 * time.Minute),
	})

	stats := rq.GetStatistics()

	// Should have 2 total items
	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", stats.TotalItems)
	}

	// Oldest item should be "valid" since invalid timestamp was reset
	if stats.OldestItem != "valid" {
		t.Errorf("Expected oldest item 'valid' (invalid timestamp should be reset), got '%s'", stats.OldestItem)
	}

	// Oldest age should NOT be 20378 days
	maxReasonableAge := 24 * time.Hour
	if stats.OldestAge > maxReasonableAge {
		t.Errorf("Expected oldest age < 24h (invalid timestamp was not properly reset), got %v", stats.OldestAge)
	}
}

func TestReviewQueue_GetStatistics_AllInvalidTimestamps(t *testing.T) {
	rq := NewReviewQueue()

	// Add items with only invalid timestamps - all should be reset to now
	rq.Add(&ReviewItem{
		SessionID:  "zero1",
		Priority:   PriorityHigh,
		Reason:     ReasonApprovalPending,
		DetectedAt: time.Time{}, // Zero time
	})
	rq.Add(&ReviewItem{
		SessionID:  "zero2",
		Priority:   PriorityMedium,
		Reason:     ReasonInputRequired,
		DetectedAt: time.Time{}, // Zero time
	})

	stats := rq.GetStatistics()

	// Should have 2 total items
	if stats.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", stats.TotalItems)
	}

	// Average age should be very small (near zero) since all timestamps were reset to now
	maxRecentAge := 5 * time.Second
	if stats.AverageAge > maxRecentAge {
		t.Errorf("Expected average age near zero (all invalid timestamps reset), got %v", stats.AverageAge)
	}

	// Oldest age should also be very small
	if stats.OldestAge > maxRecentAge {
		t.Errorf("Expected oldest age near zero (all invalid timestamps reset), got %v", stats.OldestAge)
	}
}

func TestReviewQueue_GetStatistics_EmptyQueue(t *testing.T) {
	rq := NewReviewQueue()

	stats := rq.GetStatistics()

	// Empty queue should have zero values
	if stats.TotalItems != 0 {
		t.Errorf("Expected 0 total items, got %d", stats.TotalItems)
	}

	if stats.AverageAge != 0 {
		t.Errorf("Expected zero average age for empty queue, got %v", stats.AverageAge)
	}

	if stats.OldestAge != 0 {
		t.Errorf("Expected zero oldest age for empty queue, got %v", stats.OldestAge)
	}

	if stats.OldestItem != "" {
		t.Errorf("Expected empty oldest item for empty queue, got '%s'", stats.OldestItem)
	}
}

func TestReviewQueue_Observer(t *testing.T) {
	rq := NewReviewQueue()

	// Create observer
	addedCount := 0
	removedCount := 0
	updatedCount := 0

	observer := &testObserver{
		onAdd: func(item *ReviewItem) {
			addedCount++
		},
		onRemove: func(sessionID string) {
			removedCount++
		},
		onUpdate: func(items []*ReviewItem) {
			updatedCount++
		},
	}

	rq.Subscribe(observer)

	// Add item - should trigger onAdd
	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityHigh})
	if addedCount != 1 {
		t.Errorf("Expected 1 add notification, got %d", addedCount)
	}

	// Update item - should trigger onUpdate
	rq.Add(&ReviewItem{SessionID: "s1", Priority: PriorityUrgent})
	if updatedCount != 1 {
		t.Errorf("Expected 1 update notification, got %d", updatedCount)
	}

	// Remove item - should trigger onRemove
	rq.Remove("s1")
	if removedCount != 1 {
		t.Errorf("Expected 1 remove notification, got %d", removedCount)
	}

	// Unsubscribe
	rq.Unsubscribe(observer)

	// Operations after unsubscribe should not trigger notifications
	rq.Add(&ReviewItem{SessionID: "s2", Priority: PriorityHigh})
	if addedCount != 1 {
		t.Error("Expected no notifications after unsubscribe")
	}
}

// testObserver is a test implementation of ReviewQueueObserver
type testObserver struct {
	onAdd    func(*ReviewItem)
	onRemove func(string)
	onUpdate func([]*ReviewItem)
}

func (o *testObserver) OnItemAdded(item *ReviewItem) {
	if o.onAdd != nil {
		o.onAdd(item)
	}
}

func (o *testObserver) OnItemRemoved(sessionID string) {
	if o.onRemove != nil {
		o.onRemove(sessionID)
	}
}

func (o *testObserver) OnQueueUpdated(items []*ReviewItem) {
	if o.onUpdate != nil {
		o.onUpdate(items)
	}
}

func TestDeterminePriority(t *testing.T) {
	tests := []struct {
		name        string
		reason      AttentionReason
		status      DetectedStatus
		age         time.Duration
		expectedMin Priority
		expectedMax Priority
	}{
		{
			name:        "error always urgent",
			reason:      ReasonInputRequired,
			status:      StatusError,
			age:         0,
			expectedMin: PriorityUrgent,
			expectedMax: PriorityUrgent,
		},
		{
			name:        "approval pending is high",
			reason:      ReasonApprovalPending,
			status:      StatusNeedsApproval,
			age:         0,
			expectedMin: PriorityHigh,
			expectedMax: PriorityHigh,
		},
		{
			name:        "old item gets elevated",
			reason:      ReasonTaskComplete,
			status:      StatusReady,
			age:         35 * time.Minute,
			expectedMin: PriorityMedium,
			expectedMax: PriorityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := DeterminePriority(tt.reason, tt.status, tt.age)
			if priority < tt.expectedMin || priority > tt.expectedMax {
				t.Errorf("Expected priority between %v and %v, got %v",
					tt.expectedMin, tt.expectedMax, priority)
			}
		})
	}
}

func TestAttentionReason_String(t *testing.T) {
	tests := []struct {
		reason   AttentionReason
		expected string
	}{
		{ReasonApprovalPending, "Approval Pending"},
		{ReasonInputRequired, "Input Required"},
		{ReasonErrorState, "Error State"},
		{ReasonIdleTimeout, "Idle Timeout"},
		{ReasonTaskComplete, "Task Complete"},
	}

	for _, tt := range tests {
		if got := tt.reason.String(); got != tt.expected {
			t.Errorf("AttentionReason.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestPriority_String(t *testing.T) {
	tests := []struct {
		priority Priority
		expected string
	}{
		{PriorityUrgent, "Urgent"},
		{PriorityHigh, "High"},
		{PriorityMedium, "Medium"},
		{PriorityLow, "Low"},
	}

	for _, tt := range tests {
		if got := tt.priority.String(); got != tt.expected {
			t.Errorf("Priority.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestPriority_Emoji(t *testing.T) {
	tests := []struct {
		priority Priority
		expected string
	}{
		{PriorityUrgent, "🔴"},
		{PriorityHigh, "🟡"},
		{PriorityMedium, "🔵"},
		{PriorityLow, "⚪"},
	}

	for _, tt := range tests {
		if got := tt.priority.Emoji(); got != tt.expected {
			t.Errorf("Priority.Emoji() = %v, want %v", got, tt.expected)
		}
	}
}
