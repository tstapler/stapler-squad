// Package flow computes aggregate backlog flow analytics (dwell, funnel,
// rework, thrash, stuck time) as pure functions over normalized events.
//
// The package is import-pure: standard library, session/domain and
// golang.org/x/sync/singleflight only (enforced by the isolate_flow_core
// depguard rule). Statuses reuse domain.BacklogStatus; no second status type.
package flow

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// ItemID identifies a backlog item (string UUID).
type ItemID string

// Event is one normalized backlog_status_events row. Note is raw text and is
// never sanitized here. Seq is a stable tiebreak ordering key only.
type Event struct {
	ItemID      ItemID
	From        domain.BacklogStatus
	To          domain.BacklogStatus
	At          time.Time
	Note        string
	TriggeredBy string
	Seq         int64
}

// FlowQuery is the single parameter object for an aggregation: a window spec,
// optional filters and Config overrides. Sections (empty means all), Compact
// and ListLimit shape the response only; they are part of the cache key.
type FlowQuery struct {
	Window  WindowSpec
	ItemIDs []ItemID
	Cohort  string
	Config  Config
	// Sections selects which sections to compute; the rest are Skipped.
	Sections []SectionName
	// Compact omits trend buckets and limits every top-N list to 3.
	Compact bool
	// ListLimit is the requested top-N list size (0 = default 10, max 50).
	ListLimit int
}

// Cause labels why an item was reopened or dequeued. Closed enum.
type Cause int

// Cause values. Unclassified is the zero value so an unset Cause is visibly unknown.
const (
	CauseUnclassified Cause = iota
	CauseDequeueSpawnFailed
	CauseDequeueClaim
	CausePRFixReopen
	CauseReviewReopen
)

var causeNames = [...]string{
	CauseUnclassified:       "unclassified",
	CauseDequeueSpawnFailed: "dequeue_spawn_failed",
	CauseDequeueClaim:       "dequeue_claim",
	CausePRFixReopen:        "pr_fix_reopen",
	CauseReviewReopen:       "review_reopen",
}

// AllCauses lists every Cause value.
func AllCauses() []Cause {
	return []Cause{CauseUnclassified, CauseDequeueSpawnFailed, CauseDequeueClaim, CausePRFixReopen, CauseReviewReopen}
}

// String returns the wire name, or "unknown" for an out-of-range value.
func (c Cause) String() string {
	if c < 0 || int(c) >= len(causeNames) {
		return "unknown"
	}
	return causeNames[c]
}

// FlipPair is the shape of a thrash flip: status A to B then B to A.
type FlipPair struct {
	A, B domain.BacklogStatus
}
