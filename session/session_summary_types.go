package session

import "time"

// SessionSummaryStatus is the lifecycle status of a SessionSummary row.
type SessionSummaryStatus string

const (
	// SessionSummaryStatusPending means a row exists but generation has not started.
	SessionSummaryStatusPending SessionSummaryStatus = "pending"
	// SessionSummaryStatusGenerating means the async pipeline is actively running.
	SessionSummaryStatusGenerating SessionSummaryStatus = "generating"
	// SessionSummaryStatusReady means generation completed successfully.
	SessionSummaryStatusReady SessionSummaryStatus = "ready"
	// SessionSummaryStatusError means generation failed at some stage.
	SessionSummaryStatusError SessionSummaryStatus = "error"
)

// IsValid reports whether s is a recognized SessionSummaryStatus.
func (s SessionSummaryStatus) IsValid() bool {
	switch s {
	case SessionSummaryStatusPending, SessionSummaryStatusGenerating,
		SessionSummaryStatusReady, SessionSummaryStatusError:
		return true
	default:
		return false
	}
}

// DiffSnapshot is a deterministic, point-in-time summary of a session's git diff.
type DiffSnapshot struct {
	FilesChanged int
	Added        int
	Removed      int
}

// IsEmpty reports whether the snapshot represents no changes at all.
func (d DiffSnapshot) IsEmpty() bool {
	return d.FilesChanged == 0 && d.Added == 0 && d.Removed == 0
}

// DecisionsSnapshot is a deterministic count of approval decisions made during a session.
type DecisionsSnapshot struct {
	AutoApproved        int
	ManuallyApproved    int
	Denied              int
	ReviewQueueResolved int
	StillOpen           int
}

// Total returns the sum of all decision counts.
func (d DecisionsSnapshot) Total() int {
	return d.AutoApproved + d.ManuallyApproved + d.Denied + d.ReviewQueueResolved + d.StillOpen
}

// Percent returns what percentage n represents of the snapshot's Total, or 0 if Total is 0.
func (d DecisionsSnapshot) Percent(n int) float64 {
	total := d.Total()
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// TimelineSnapshot captures the start/stop times of a session.
type TimelineSnapshot struct {
	StartedAt time.Time
	StoppedAt time.Time
}

// Duration returns the elapsed time between StartedAt and StoppedAt.
func (t TimelineSnapshot) Duration() time.Duration {
	return t.StoppedAt.Sub(t.StartedAt)
}

// CostSnapshot captures token usage/cost data for a session.
type CostSnapshot struct {
	TotalTokens      int64
	EstimatedCostUSD float64
	// DataUnavailable distinguishes "genuinely zero tokens" from "cost data could
	// not be read" (e.g. no transcript found for this session).
	DataUnavailable bool
}
