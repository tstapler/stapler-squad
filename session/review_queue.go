package session

import (
	"claude-squad/session/git"
	"fmt"
	"sort"
	"sync"
	"time"
)

// AttentionReason describes why a session needs user attention.
type AttentionReason string

const (
	ReasonApprovalPending AttentionReason = "approval_pending" // Waiting for approval dialog response
	ReasonInputRequired   AttentionReason = "input_required"   // Waiting for user input
	ReasonErrorState      AttentionReason = "error_state"      // Error occurred
	ReasonIdleTimeout     AttentionReason = "idle_timeout"     // No activity for extended period
	ReasonTaskComplete    AttentionReason = "task_complete"    // Task completed, waiting for next instruction
)

// Priority defines the urgency level of a review item.
type Priority int

const (
	PriorityUrgent Priority = 1 // Blocking errors
	PriorityHigh   Priority = 2 // Approval dialogs
	PriorityMedium Priority = 3 // Input requests
	PriorityLow    Priority = 4 // Idle/complete
)

// ReviewItem represents a session that needs user attention.
type ReviewItem struct {
	SessionID   string            `json:"session_id"`
	SessionName string            `json:"session_name"`
	Reason      AttentionReason   `json:"reason"`
	Priority    Priority          `json:"priority"`
	DetectedAt  time.Time         `json:"detected_at"`
	Context     string            `json:"context"`            // Snippet of relevant output
	PatternName string            `json:"pattern_name"`       // Pattern that matched
	Metadata    map[string]string `json:"metadata,omitempty"` // Additional metadata

	// Session details for rich display (matching Instance fields)
	Program      string         `json:"program"`       // Program running (claude, aider, etc.)
	Branch       string         `json:"branch"`        // Git branch name
	Path         string         `json:"path"`          // Repository path
	WorkingDir   string         `json:"working_dir"`   // Working directory
	Status       Status         `json:"status"`        // Current session status
	Tags         []string       `json:"tags"`          // Session tags
	Category     string         `json:"category"`      // Session category
	DiffStats    *git.DiffStats `json:"diff_stats"`    // Git diff statistics (nullable)
	LastActivity time.Time      `json:"last_activity"` // Last meaningful output time (used for sorting and display)
}

// ReviewQueueObserver is notified when the review queue changes.
type ReviewQueueObserver interface {
	OnItemAdded(item *ReviewItem)
	OnItemRemoved(sessionID string)
	OnQueueUpdated(items []*ReviewItem)
}

// ReviewQueue manages sessions that need user attention.
type ReviewQueue struct {
	items     map[string]*ReviewItem
	mu        sync.RWMutex
	observers []ReviewQueueObserver
}

// NewReviewQueue creates a new review queue.
func NewReviewQueue() *ReviewQueue {
	return &ReviewQueue{
		items:     make(map[string]*ReviewItem),
		observers: make([]ReviewQueueObserver, 0),
	}
}

// Add adds a session to the review queue or updates it if already present.
// Returns true if this is a new item, false if it was updated.
func (rq *ReviewQueue) Add(item *ReviewItem) bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// Validate and fix invalid timestamps (per user requirement: reset to current time)
	minValidTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if item.DetectedAt.IsZero() || item.DetectedAt.Before(minValidTime) {
		item.DetectedAt = time.Now()
	}

	existingItem, exists := rq.items[item.SessionID]
	rq.items[item.SessionID] = item

	// Notify observers
	if !exists {
		for _, observer := range rq.observers {
			observer.OnItemAdded(item)
		}
	} else {
		// Only fire OnQueueUpdated if something meaningful changed
		// This prevents spurious notifications when the poller preserves DetectedAt
		hasSignificantChange := existingItem.Reason != item.Reason ||
			existingItem.Priority != item.Priority ||
			existingItem.Context != item.Context ||
			!existingItem.LastActivity.Equal(item.LastActivity)

		if hasSignificantChange {
			for _, observer := range rq.observers {
				observer.OnQueueUpdated(rq.getSortedItemsUnsafe())
			}
		}
	}

	return !exists
}

// Remove removes a session from the review queue.
// Returns true if the item was present and removed.
func (rq *ReviewQueue) Remove(sessionID string) bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if _, exists := rq.items[sessionID]; !exists {
		return false
	}

	delete(rq.items, sessionID)

	// Notify observers
	for _, observer := range rq.observers {
		observer.OnItemRemoved(sessionID)
	}

	return true
}

// Get retrieves a review item by session ID.
func (rq *ReviewQueue) Get(sessionID string) (*ReviewItem, bool) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	item, exists := rq.items[sessionID]
	return item, exists
}

// Has checks if a session is in the review queue.
func (rq *ReviewQueue) Has(sessionID string) bool {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	_, exists := rq.items[sessionID]
	return exists
}

// List returns all items sorted by priority (most urgent first).
func (rq *ReviewQueue) List() []*ReviewItem {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	return rq.getSortedItemsUnsafe()
}

// getSortedItemsUnsafe returns sorted items without locking (internal use).
func (rq *ReviewQueue) getSortedItemsUnsafe() []*ReviewItem {
	items := make([]*ReviewItem, 0, len(rq.items))
	for _, item := range rq.items {
		items = append(items, item)
	}

	// Sort by priority (lower number = higher priority), then by last activity time (most recent first)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		// Sort by last activity - most recent activity first (After means j is older than i)
		return items[i].LastActivity.After(items[j].LastActivity)
	})

	return items
}

// Count returns the number of items in the queue.
func (rq *ReviewQueue) Count() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	return len(rq.items)
}

// CountByPriority returns the count of items for each priority level.
func (rq *ReviewQueue) CountByPriority() map[Priority]int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	counts := make(map[Priority]int)
	for _, item := range rq.items {
		counts[item.Priority]++
	}

	return counts
}

// CountByReason returns the count of items for each attention reason.
func (rq *ReviewQueue) CountByReason() map[AttentionReason]int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	counts := make(map[AttentionReason]int)
	for _, item := range rq.items {
		counts[item.Reason]++
	}

	return counts
}

// Next returns the session ID of the next review item after the given session ID.
// If currentSessionID is empty or not found, returns the highest priority item.
// Returns empty string and false if the queue is empty.
func (rq *ReviewQueue) Next(currentSessionID string) (string, bool) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	items := rq.getSortedItemsUnsafe()
	if len(items) == 0 {
		return "", false
	}

	// If no current session, return first item
	if currentSessionID == "" {
		return items[0].SessionID, true
	}

	// Find current session in sorted list
	for i, item := range items {
		if item.SessionID == currentSessionID {
			// Return next item, wrapping around to start if at end
			nextIdx := (i + 1) % len(items)
			return items[nextIdx].SessionID, true
		}
	}

	// Current session not in queue, return first item
	return items[0].SessionID, true
}

// Previous returns the session ID of the previous review item before the given session ID.
// If currentSessionID is empty or not found, returns the highest priority item.
// Returns empty string and false if the queue is empty.
func (rq *ReviewQueue) Previous(currentSessionID string) (string, bool) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	items := rq.getSortedItemsUnsafe()
	if len(items) == 0 {
		return "", false
	}

	// If no current session, return first item
	if currentSessionID == "" {
		return items[0].SessionID, true
	}

	// Find current session in sorted list
	for i, item := range items {
		if item.SessionID == currentSessionID {
			// Return previous item, wrapping around to end if at start
			prevIdx := (i - 1 + len(items)) % len(items)
			return items[prevIdx].SessionID, true
		}
	}

	// Current session not in queue, return first item
	return items[0].SessionID, true
}

// Clear removes all items from the queue.
func (rq *ReviewQueue) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.items = make(map[string]*ReviewItem)

	// Notify observers
	for _, observer := range rq.observers {
		observer.OnQueueUpdated([]*ReviewItem{})
	}
}

// Subscribe adds an observer to receive queue update notifications.
func (rq *ReviewQueue) Subscribe(observer ReviewQueueObserver) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.observers = append(rq.observers, observer)
}

// Unsubscribe removes an observer from receiving notifications.
func (rq *ReviewQueue) Unsubscribe(observer ReviewQueueObserver) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for i, obs := range rq.observers {
		if obs == observer {
			rq.observers = append(rq.observers[:i], rq.observers[i+1:]...)
			return
		}
	}
}

// GetStatistics returns summary statistics about the review queue.
func (rq *ReviewQueue) GetStatistics() ReviewQueueStatistics {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	stats := ReviewQueueStatistics{
		TotalItems: len(rq.items),
		ByPriority: make(map[Priority]int),
		ByReason:   make(map[AttentionReason]int),
	}

	var totalAge time.Duration
	var oldestTime time.Time // Initialize to zero time (far past)
	var validItemCount int   // Count of items with valid timestamps

	// Timestamp validation threshold - any timestamp before this is considered invalid
	minValidTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, item := range rq.items {
		stats.ByPriority[item.Priority]++
		stats.ByReason[item.Reason]++

		// Skip items with zero or invalid timestamps for age calculations
		if item.DetectedAt.IsZero() || item.DetectedAt.Before(minValidTime) {
			continue
		}

		age := time.Since(item.DetectedAt)
		totalAge += age
		validItemCount++

		// Track the oldest (earliest) DetectedAt timestamp
		if oldestTime.IsZero() || item.DetectedAt.Before(oldestTime) {
			oldestTime = item.DetectedAt
			stats.OldestItem = item.SessionID
		}
	}

	if validItemCount > 0 {
		stats.AverageAge = totalAge / time.Duration(validItemCount)
		if !oldestTime.IsZero() {
			stats.OldestAge = time.Since(oldestTime)
		}
	}

	return stats
}

// ReviewQueueStatistics provides summary information about the queue.
type ReviewQueueStatistics struct {
	TotalItems int
	ByPriority map[Priority]int
	ByReason   map[AttentionReason]int
	AverageAge time.Duration
	OldestAge  time.Duration
	OldestItem string
}

// DeterminePriority calculates the priority for a review item based on multiple factors.
func DeterminePriority(reason AttentionReason, detectedStatus DetectedStatus, age time.Duration) Priority {
	// Base priority from reason
	basePriority := reasonToPriority(reason)

	// Adjust based on detected status
	if detectedStatus == StatusError {
		return PriorityUrgent
	}

	// Age-based urgency increase (items waiting longer get higher priority)
	if age > 30*time.Minute {
		if basePriority > PriorityUrgent {
			basePriority--
		}
	}

	return basePriority
}

// reasonToPriority maps attention reasons to base priority levels.
func reasonToPriority(reason AttentionReason) Priority {
	switch reason {
	case ReasonErrorState:
		return PriorityUrgent
	case ReasonApprovalPending:
		return PriorityHigh
	case ReasonInputRequired:
		return PriorityMedium
	case ReasonTaskComplete, ReasonIdleTimeout:
		return PriorityLow
	default:
		return PriorityMedium
	}
}

// String returns a human-readable description of the attention reason.
func (r AttentionReason) String() string {
	switch r {
	case ReasonApprovalPending:
		return "Approval Pending"
	case ReasonInputRequired:
		return "Input Required"
	case ReasonErrorState:
		return "Error State"
	case ReasonIdleTimeout:
		return "Idle Timeout"
	case ReasonTaskComplete:
		return "Task Complete"
	default:
		return string(r)
	}
}

// String returns a human-readable description of the priority level.
func (p Priority) String() string {
	switch p {
	case PriorityUrgent:
		return "Urgent"
	case PriorityHigh:
		return "High"
	case PriorityMedium:
		return "Medium"
	case PriorityLow:
		return "Low"
	default:
		return fmt.Sprintf("Priority(%d)", p)
	}
}

// Emoji returns an emoji representation of the priority level.
func (p Priority) Emoji() string {
	switch p {
	case PriorityUrgent:
		return "🔴"
	case PriorityHigh:
		return "🟡"
	case PriorityMedium:
		return "🔵"
	case PriorityLow:
		return "⚪"
	default:
		return "⭕"
	}
}
