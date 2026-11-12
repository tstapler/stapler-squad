package backlogdto

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// OpenStuckStateData is a projected, already-filtered (open + un-snoozed)
// BacklogStuckState row joined with its parent item's rendering-relevant
// fields. Returned only by FindOpenStuckStates, which applies the "open"
// (resolved_at IS NULL) and "not currently snoozed" filters at the query
// boundary — callers never need to re-check ResolvedAt/SnoozedUntil
// nullability themselves (parse-don't-validate at the repository boundary).
type OpenStuckStateData struct {
	ID                     string
	ItemID                 string
	Reason                 domain.StuckReason
	FirstDetectedAt        time.Time
	LastCheckedAt          time.Time
	NotifiedAt             *time.Time
	Context                string
	ItemTitle              string
	ItemStatus             domain.BacklogStatus
	PrNumber               int
	PrURL                  string
	RemediationAttempts    int32
	NextRemediationAt      *time.Time
	GraceBootTime          *time.Time
	PlanArtifactsPath      string
	DiagnoseNudgeCount     int32
	DiagnoseNextEligibleAt *time.Time
}

// StuckStateHistoryData is a single BacklogStuckState row for one item,
// open or resolved — the full-history counterpart to OpenStuckStateData,
// which deliberately filters to open-only. Used by
// ListStuckStatesForItem for stuck-item root-cause reporting, where a
// resolved row (e.g. an exhausted bounce or a since-fixed PR check) is
// exactly the history a human or diagnose session needs to see.
type StuckStateHistoryData struct {
	ID                  string
	Reason              domain.StuckReason
	FirstDetectedAt     time.Time
	LastCheckedAt       time.Time
	ResolvedAt          *time.Time
	Context             string
	RemediationAttempts int32
	DiagnoseNudgeCount  int32
}
