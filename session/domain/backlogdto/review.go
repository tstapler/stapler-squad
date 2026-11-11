// Package backlogdto holds the volatile backlog data-transfer types, kept out of the
// stable session/domain so DTO churn does not invalidate its importers. It imports only
// the standard library and session/domain.
package backlogdto

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// ReviewVerdictData is the input data for saving a ReviewVerdict.
type ReviewVerdictData struct {
	ItemSessionID  string
	OverallOutcome domain.ReviewOutcome
	PerCriterion   string // JSON
	Summary        string
	DiffHash       string
	PromptHash     string
	DiffTokenCount int
	DiffTruncated  bool
	// ReviewedCommitSha is the work-session tip commit the verdict reviewed; "" when unknown.
	ReviewedCommitSha string
	OverrideBy        string
	OverrideReason    string
	OverrideAt        *time.Time
}

// ReviewVerdictSummary is a domain DTO for a review verdict embedded in ItemSessionSummary.
type ReviewVerdictSummary struct {
	ID                string
	OverallOutcome    string
	PerCriterion      string // JSON []CriterionVerdict
	Summary           string
	DiffHash          string
	DiffTokenCount    int
	DiffTruncated     bool
	ReviewedCommitSha string
	OverrideBy        string
	OverrideReason    string
	OverrideAt        *time.Time
	CreatedAt         time.Time
}

// DiagnoseResultData is the JSON stored in item_sessions.diagnose_result.
type DiagnoseResultData struct {
	Outcome     string    `json:"outcome"`
	Summary     string    `json:"summary"`
	FiledItemID string    `json:"filed_item_id,omitempty"`
	DiagnosedAt time.Time `json:"diagnosed_at"`
}
