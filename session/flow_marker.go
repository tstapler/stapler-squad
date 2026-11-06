package session

import (
	"sync/atomic"
	"time"
)

// FlowMarkerKind names a family of server-decided flow markers. Only server
// code sets it (EntRepository.RecordFlowMarker); it is never parsed from text.
type FlowMarkerKind string

const (
	FlowMarkerPRFixOutcome FlowMarkerKind = "pr_fix_outcome"
	FlowMarkerPRFixTrigger FlowMarkerKind = "pr_fix_trigger"
)

// PR-fix outcome values.
const (
	PRFixOutcomeSteered       = "steered"
	PRFixOutcomeRespawned     = "respawned"
	PRFixOutcomeSkippedActive = "skipped_active"
)

// PR-fix trigger values.
const (
	PRFixTriggerCI       = "ci"
	PRFixTriggerConflict = "conflict"
	PRFixTriggerReview   = "review"
	PRFixTriggerFeedback = "feedback"
	PRFixTriggerMixed    = "mixed"
	PRFixTriggerUnknown  = "unknown"
)

// FlowMarkerData is one stored marker row.
type FlowMarkerData struct {
	ItemID    string
	Kind      FlowMarkerKind
	Value     string
	PRNumber  int
	CreatedAt time.Time
}

// ClassifyPRFixTrigger maps the poller's decision flags to a trigger value.
// It is total: only the five documented single-cause shapes (plus ci+conflict
// as mixed) are named; every other combination, including all-false, is unknown.
func ClassifyPRFixTrigger(ci, conflict, blockingReview, feedback bool) string {
	switch {
	case ci && conflict && !blockingReview && !feedback:
		return PRFixTriggerMixed
	case ci && !conflict && !blockingReview && !feedback:
		return PRFixTriggerCI
	case conflict && !ci && !blockingReview && !feedback:
		return PRFixTriggerConflict
	case blockingReview && !ci && !conflict && !feedback:
		return PRFixTriggerReview
	case feedback && !ci && !conflict && !blockingReview:
		return PRFixTriggerFeedback
	default:
		return PRFixTriggerUnknown
	}
}

// prFixTriggerUnknownCount counts trigger decisions classified unknown.
var prFixTriggerUnknownCount atomic.Int64

// PRFixTriggerUnknownCount returns how many trigger decisions classified as unknown.
func PRFixTriggerUnknownCount() int64 { return prFixTriggerUnknownCount.Load() }
