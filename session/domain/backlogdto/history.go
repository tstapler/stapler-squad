package backlogdto

import "time"

// ProgressNoteData is the domain DTO replacing *ent.BacklogProgressNote in Storage returns.
// Unlike the current-note-per-criterion stored on BacklogItem.AcceptanceCriteria, this
// represents a single append-only history entry from one report_progress call.
type ProgressNoteData struct {
	ID             string
	CriterionIndex int
	Note           string
	Status         string
	CreatedAt      time.Time
}

// ActivityNoteData is the domain DTO replacing *ent.BacklogActivityNote in Storage returns.
// Unlike ProgressNoteData (written only by the role-gated report_progress tool), this
// represents a single append-only entry from the ungated post_backlog_update tool — see
// ADR-001 (sibling table, not extending BacklogProgressNote).
type ActivityNoteData struct {
	ID                 string
	Message            string
	AuthorSessionUUID  string
	AuthorSessionTitle string
	CreatedAt          time.Time
}

// SourceSyncEventData is the domain DTO replacing *ent.SourceSyncEvent in Storage returns.
type SourceSyncEventData struct {
	ID           string
	ItemsCreated int
	ItemsUpdated int
	ItemsSkipped int
	ItemsErrored int
	ErrorMessage string
	CursorAfter  string
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// BacklogStatusEventData is the domain DTO replacing *ent.BacklogStatusEvent in Storage returns.
type BacklogStatusEventData struct {
	ID          string
	FromStatus  string
	ToStatus    string
	TriggeredBy string
	Note        *string
	// StageNameSnapshot is Epic 2.5's frozen-at-transition-time human-readable
	// name of the destination BacklogStage, so item-detail history keeps
	// rendering the original stage name after that stage row is later renamed
	// or deleted. Nil for a row written before this field existed, or when no
	// matching BacklogStage row was found at write time.
	StageNameSnapshot *string
	// AllowedTransitionsSnapshot is ADR-004's sibling snapshot: the destination
	// BacklogStage's legal outgoing transition slugs (as domain.BacklogStatus string
	// values) at the moment of this transition, so CanTransition/
	// AllowedTransitions/PendingGates can fall back to it when that stage is
	// later deleted. Nil (not just empty) for a row written before this field
	// existed, or when no matching BacklogStage row was found at write time —
	// distinguishing "no snapshot captured" from "captured, zero transitions."
	AllowedTransitionsSnapshot []string
	CreatedAt                  time.Time
}
