package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// GuidanceQuestionsRetention is how long a terminal (answered or cancelled) row
// keeps its questions_json: option text can quote user content (pre-mortem #18).
const GuidanceQuestionsRetention = 30 * 24 * time.Hour

// DefaultGuidanceRequestPendingCap is the fallback per-(scope, scope_key)
// pending cap used when a caller doesn't supply one (CreateGuidanceRequestInput.Cap
// == 0). Phase 4 of the durable-guidance-request plan wires a real,
// operator-configurable value (config.GetGuidanceRequestPendingCap) through as
// the Cap field on every call site; until then, this constant is what backs
// it. Cap is a caller-supplied parameter rather than something this method
// reads from config directly, so that wiring lands as a call-site change, not
// a change to CreateGuidanceRequest's own logic.
const DefaultGuidanceRequestPendingCap = 4

// ErrPendingCapExceeded is returned by CreateGuidanceRequest when creating a
// new GuidanceRequest would push a scope's open (pending) row count past its
// cap. Never returned for a call that resolves to a pre-existing open row via
// dedup — see CreateGuidanceRequest's doc comment.
var ErrPendingCapExceeded = errors.New("guidance request pending cap exceeded")

// GuidanceRequestStatus is the derived (never stored) lifecycle state of a
// GuidanceRequest row — see GuidanceRequestData.Status.
type GuidanceRequestStatus string

const (
	GuidanceRequestStatusPending   GuidanceRequestStatus = "pending"
	GuidanceRequestStatusAnswered  GuidanceRequestStatus = "answered"
	GuidanceRequestStatusCancelled GuidanceRequestStatus = "cancelled"
)

// GuidanceRequestData is the plain, ent-independent view of a GuidanceRequest
// row returned by this file's repository methods — mirrors OpenStuckStateData's
// role of exposing entity data outside the ent package.
type GuidanceRequestData struct {
	ID           string
	Scope        RequestScope
	ItemID       *uuid.UUID
	SessionUUID  string
	QuestionText string
	QuestionType QuestionType
	Options      string
	Answer       string
	CreatedAt    time.Time
	NotifiedAt   *time.Time
	AnsweredAt   *time.Time
	CancelledAt  *time.Time

	// Live-question and peer-help columns (ADR-003); legacy rows carry the defaults.
	Source              QuestionSource
	AskerSessionUUID    string
	AnswererKind        AnswererKind
	AnswererSessionUUID string
	MultiSelect         bool
	QuestionsJSON       string
	DeliveryRoute       DeliveryRoute
	DeliveryState       DeliveryState
	DeliveryAttempts    int
	HookCallID          HookCallID
	ExpiresAt           *time.Time
	CancelReason        CancelReason
}

// Status derives the row's lifecycle state from its timestamps. Cancelled is
// terminal and dominates even if AnsweredAt is also set (matching the Domain
// Glossary's "cancelled, terminal" rule) — an answer recorded moments before a
// self-heal sweep cancels the row (e.g. its owning item was hard-deleted in
// between) should not read back as "answered".
func (d GuidanceRequestData) Status() GuidanceRequestStatus {
	switch {
	case d.CancelledAt != nil:
		return GuidanceRequestStatusCancelled
	case d.AnsweredAt != nil:
		return GuidanceRequestStatusAnswered
	default:
		return GuidanceRequestStatusPending
	}
}

// CreateGuidanceRequestInput carries the fields needed by CreateGuidanceRequest.
type CreateGuidanceRequestInput struct {
	Scope        RequestScope
	ItemID       *uuid.UUID // required for Scope == RequestScopeBacklogItem
	SessionUUID  string     // required for Scope == RequestScopeSession
	QuestionText string
	QuestionType QuestionType
	Options      string // JSON-encoded []string; only meaningful for QuestionTypeMultipleChoice
	// Cap is the caller-supplied per-(scope, scope_key) pending-request limit.
	// Zero means "use DefaultGuidanceRequestPendingCap" — see that constant's
	// doc comment for why this is a parameter rather than a config read here.
	Cap int

	// Mirror is set only by CreateMirroredGuidanceRequest, from a request that
	// passed NewGuidanceRequest; nil means a legacy agent_mcp create.
	Mirror *GuidanceRequestParams
}
