package session

import "time"

// BacklogChangeKind identifies which kind of backlog item mutation a
// BacklogItemChange describes. Mirrors events.BacklogChangeKind
// (pkg/events/types.go) one-to-one; kept as a separate type here because this
// package cannot import pkg/events directly — pkg/events imports session, so
// the reverse import would be a cycle. The adapter (server/services, Story
// 1.3.2) is responsible for converting between the two.
type BacklogChangeKind string

const (
	// ChangeStatusTransition is emitted when an item's status changes.
	ChangeStatusTransition BacklogChangeKind = "status_transition"
	// ChangeVerdictRecorded is emitted when a review verdict is saved.
	ChangeVerdictRecorded BacklogChangeKind = "verdict_recorded"
	// ChangeSessionAttached is emitted when a session is attached to an item.
	ChangeSessionAttached BacklogChangeKind = "session_attached"
	// ChangeItemUpdated is emitted when item fields (title, description, etc.) change.
	ChangeItemUpdated BacklogChangeKind = "item_updated"
	// ChangeItemArchived is emitted when an item is archived.
	ChangeItemArchived BacklogChangeKind = "item_archived"
	// ChangeItemRemoved is emitted when an item is deleted.
	ChangeItemRemoved BacklogChangeKind = "item_removed"
	// ChangeTriageProgressUpdated is emitted when in-flight triage progress is
	// written (UpdateItemSessionTriageResult). Converts to the existing
	// item_updated wire event, not a new proto message.
	ChangeTriageProgressUpdated BacklogChangeKind = "triage_progress_updated"
	// ChangeActivityNoteAdded is emitted by AppendActivityNote (ADR-001's
	// sibling table) — carries only the new note via ActivityNote, never
	// OldStatus/NewStatus/etc.
	ChangeActivityNoteAdded BacklogChangeKind = "activity_note_added"
)

// BacklogItemChange describes a single backlog item mutation, passed to
// ItemChangePublisher.PublishItemChanged by the repository method that made
// the mutation. Only the fields relevant to Kind are expected to be populated
// by the caller.
type BacklogItemChange struct {
	// Kind identifies which backlog mutation this change describes.
	Kind BacklogChangeKind
	// OldStatus is the prior status for ChangeStatusTransition.
	OldStatus string
	// NewStatus is the new status for ChangeStatusTransition.
	NewStatus string
	// UpdatedFields lists which fields changed for ChangeItemUpdated (and
	// ChangeTriageProgressUpdated).
	UpdatedFields []string
	// SessionID identifies the session for ChangeSessionAttached.
	SessionID string
	// ClaimantHostID is the claiming/attaching process's own stable host
	// identifier for ChangeSessionAttached, mirrored from
	// ItemSessionData.ClaimantHostID — never derived from the session being
	// attached. See ItemSession.claimant_host_id's schema comment.
	ClaimantHostID string
	// ArchivedAt is the archival timestamp for ChangeItemArchived.
	ArchivedAt *time.Time
	// RemovedReason describes why an item was removed for ChangeItemRemoved.
	RemovedReason string
	// Verdict is populated only when Kind == ChangeVerdictRecorded, set
	// directly from the ReviewVerdictData value the caller already has in
	// hand (the actual parameter type of SaveReviewVerdict /
	// CreateItemSessionWithVerdict, session/storage_backlog.go) — this
	// carries the verdict through the pipeline as first-class data, not via
	// a client-side join against item_sessions.
	Verdict *ReviewVerdictData
	// ActivityNote is populated only when Kind == ChangeActivityNoteAdded.
	ActivityNote *ActivityNoteData
}

// ItemChangePublisher publishes a backlog item mutation to interested
// subscribers (typically the event bus). Implemented outside this package
// (typically a thin adapter over the event bus, in server/services) since
// this package cannot import pkg/events directly — pkg/events imports
// session, so the reverse import would be a cycle. Mirrors the Notifier
// interface's cross-package adapter pattern (see Notifier, above in
// backlog_lifecycle.go).
//
// Publish is always best-effort: implementations must never block or panic
// into the caller, and callers must nil-check before invoking since a
// publisher may not be wired (e.g. in tests, or a repository that hasn't had
// SetItemChangePublisher called on it).
type ItemChangePublisher interface {
	PublishItemChanged(item *BacklogItemData, change BacklogItemChange)
}
