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
// It is total: exactly one flag names that cause, two or more are mixed, and
// no flag (e.g. the closed-PR branch) is unknown.
func ClassifyPRFixTrigger(ci, conflict, blockingReview, feedback bool) string {
	flags := 0
	for _, f := range [...]bool{ci, conflict, blockingReview, feedback} {
		if f {
			flags++
		}
	}
	switch {
	case flags >= 2:
		return PRFixTriggerMixed
	case ci:
		return PRFixTriggerCI
	case conflict:
		return PRFixTriggerConflict
	case blockingReview:
		return PRFixTriggerReview
	case feedback:
		return PRFixTriggerFeedback
	default:
		return PRFixTriggerUnknown
	}
}

// prFixTriggerUnknownCount counts trigger decisions classified unknown.
var prFixTriggerUnknownCount atomic.Int64

// PRFixTriggerUnknownCount returns how many trigger decisions classified as unknown.
func PRFixTriggerUnknownCount() int64 { return prFixTriggerUnknownCount.Load() }
