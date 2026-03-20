package session

// review_queue.go re-exports the types from session/queue so that all existing callers
// in the session package and external packages continue to compile without changes.
//
// The canonical implementations live in session/queue/queue.go.
// This file is a thin compatibility shim using Go type aliases (=).

import "github.com/tstapler/stapler-squad/session/queue"

// AttentionReason re-export
type AttentionReason = queue.AttentionReason

const (
	ReasonApprovalPending    = queue.ReasonApprovalPending
	ReasonInputRequired      = queue.ReasonInputRequired
	ReasonErrorState         = queue.ReasonErrorState
	ReasonTestsFailing       = queue.ReasonTestsFailing
	ReasonIdleTimeout        = queue.ReasonIdleTimeout
	ReasonTaskComplete       = queue.ReasonTaskComplete
	ReasonUncommittedChanges = queue.ReasonUncommittedChanges
	ReasonIdle               = queue.ReasonIdle
	ReasonStale              = queue.ReasonStale
	ReasonWaitingForUser     = queue.ReasonWaitingForUser
)

// Priority re-export
type Priority = queue.Priority

const (
	PriorityUrgent = queue.PriorityUrgent
	PriorityHigh   = queue.PriorityHigh
	PriorityMedium = queue.PriorityMedium
	PriorityLow    = queue.PriorityLow
)

// ReviewItem re-export
type ReviewItem = queue.ReviewItem

// ReviewQueueObserver re-export
type ReviewQueueObserver = queue.ReviewQueueObserver

// ReviewQueue re-export
type ReviewQueue = queue.ReviewQueue

// ReviewQueueStatistics re-export
type ReviewQueueStatistics = queue.ReviewQueueStatistics

// NewReviewQueue creates a new review queue.
func NewReviewQueue() *ReviewQueue {
	return queue.NewReviewQueue()
}

// DeterminePriority re-export
var DeterminePriority = queue.DeterminePriority

// reasonToPriority re-export (package-level alias for test access within session package)
var reasonToPriority = queue.ReasonToPriority
