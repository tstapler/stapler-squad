package session

import (
	"cmp"
	"fmt"
	"slices"

	domain "github.com/tstapler/stapler-squad/session/domain"
)

// BacklogStatus represents the lifecycle state of a backlog item.
// Type alias — session.BacklogStatus and domain.BacklogStatus are identical types;
// all existing callers continue to work without any import changes.
type BacklogStatus = domain.BacklogStatus

const (
	BacklogStatusIdea       = domain.BacklogStatusIdea
	BacklogStatusRefining   = domain.BacklogStatusRefining
	BacklogStatusReady      = domain.BacklogStatusReady
	BacklogStatusInProgress = domain.BacklogStatusInProgress
	BacklogStatusReview     = domain.BacklogStatusReview
	BacklogStatusPRPending  = domain.BacklogStatusPRPending
	BacklogStatusDone       = domain.BacklogStatusDone
	BacklogStatusArchived   = domain.BacklogStatusArchived
)

// Session role constants.
const (
	SessionRoleWork   = "work"
	SessionRoleTriage = "triage"
	SessionRoleReview = "review"
)

// Session tag constants for backlog-spawned sessions.
const (
	TagBacklogWork     = "backlog:work"
	TagBacklogRevision = "backlog:revision"
	TagAutonomous      = "autonomous"
)

// CategoryBacklog is the Session.Category value assigned to all sessions
// spawned by BacklogService (work, revision, review-gate, re-review) so they
// group under a "Backlog" bucket in the session list UI instead of falling
// into "Uncategorized".
const CategoryBacklog = "Backlog"

// TriggeredBy values for BacklogStatusEvent records.
const (
	TriggeredByUser   = "user"
	TriggeredBySystem = "system"
)

// DefaultBacklogPriority is the default priority assigned to new backlog items
// when no priority is specified. Lower values indicate higher priority.
const DefaultBacklogPriority = domain.DefaultBacklogPriority

// AcStatus represents the status of a single acceptance criterion.
// Type alias — session.AcStatus and domain.AcStatus are identical types.
type AcStatus = domain.AcStatus

const (
	AcStatusPending    = domain.AcStatusPending
	AcStatusInProgress = domain.AcStatusInProgress
	AcStatusDone       = domain.AcStatusDone
	AcStatusFail       = domain.AcStatusFail
)

// AcCriteriaJSON is the JSON-serialized form of []AcCriterion stored in the DB.
// Type alias — session.AcCriteriaJSON and domain.AcCriteriaJSON are identical types.
type AcCriteriaJSON = domain.AcCriteriaJSON

// AcCriteriaJSONEmpty is the zero value — an empty criteria list.
const AcCriteriaJSONEmpty = domain.AcCriteriaJSONEmpty

// AcCriterion is a single acceptance criterion for a backlog item.
// Type alias — session.AcCriterion and domain.AcCriterion are identical types.
type AcCriterion = domain.AcCriterion

// ParseAcCriteria deserializes acceptance criteria from a JSON string.
var ParseAcCriteria = domain.ParseAcCriteria

// SerializeAcCriteria serializes acceptance criteria to an AcCriteriaJSON value.
var SerializeAcCriteria = domain.SerializeAcCriteria

// MergeAcCriteria merges incoming criteria into existing by index.
// Criteria not mentioned in incoming are preserved unchanged.
// Returns an error if incoming contains duplicate indices.
func MergeAcCriteria(existing []AcCriterion, incoming []AcCriterion) (AcCriteriaJSON, error) {
	// Validate: no duplicate indices in incoming.
	seen := make(map[int]struct{}, len(incoming))
	for _, ac := range incoming {
		if _, dup := seen[ac.Index]; dup {
			return "", fmt.Errorf("duplicate index %d in incoming acceptance criteria", ac.Index)
		}
		seen[ac.Index] = struct{}{}
	}

	// Build lookup from existing criteria.
	byIndex := make(map[int]AcCriterion, len(existing)+len(incoming))
	for _, ac := range existing {
		byIndex[ac.Index] = ac
	}

	// Apply incoming: add new or overwrite existing entries.
	for _, ac := range incoming {
		byIndex[ac.Index] = ac
	}

	// Rebuild ordered slice, sorted by index.
	merged := make([]AcCriterion, 0, len(byIndex))
	for _, ac := range byIndex {
		merged = append(merged, ac)
	}
	slices.SortFunc(merged, func(a, b AcCriterion) int {
		return cmp.Compare(a.Index, b.Index)
	})

	return SerializeAcCriteria(merged)
}

// ReviewOutcome is a typed verdict outcome value (PASS, FAIL, PARTIAL, UNVERIFIABLE).
// Type alias — session.ReviewOutcome and domain.ReviewOutcome are identical types.
type ReviewOutcome = domain.ReviewOutcome

const (
	ReviewOutcomePass         = domain.ReviewOutcomePass
	ReviewOutcomeFail         = domain.ReviewOutcomeFail
	ReviewOutcomePartial      = domain.ReviewOutcomePartial
	ReviewOutcomeUnverifiable = domain.ReviewOutcomeUnverifiable
)

// Backward-compatible aliases so callers can be migrated incrementally.
// Prefer ReviewOutcome* constants in new code.
const (
	ReviewVerdictPass         = domain.ReviewVerdictPass
	ReviewVerdictFail         = domain.ReviewVerdictFail
	ReviewVerdictPartial      = domain.ReviewVerdictPartial
	ReviewVerdictUnverifiable = domain.ReviewVerdictUnverifiable
)

// CriterionVerdict holds the review outcome for a single acceptance criterion.
// Type alias — session.CriterionVerdict and domain.CriterionVerdict are identical types.
type CriterionVerdict = domain.CriterionVerdict

// AggregateOutcome computes the overall outcome from a slice of CriterionVerdicts.
var AggregateOutcome = domain.AggregateOutcome

// CanTransitionBacklog reports whether a transition from one backlog status to another is permitted.
var CanTransitionBacklog = domain.CanTransitionBacklog

// validTransitions is a package-level snapshot of the domain transition table,
// kept for use by WorkflowEngine (which deep-copies it at construction time).
var validTransitions = domain.ValidTransitions()

// Sentinel errors for transition guards.
var (
	ErrACRequired            = domain.ErrACRequired
	ErrPlanRequired          = domain.ErrPlanRequired
	ErrPlanArtifactsRequired = domain.ErrPlanArtifactsRequired
	ErrVerdictRequired       = domain.ErrVerdictRequired
	ErrCodeNotOnMain         = domain.ErrCodeNotOnMain
)

// BacklogItemTransitionInput carries the fields needed by TransitionGuard.
// Type alias — session.BacklogItemTransitionInput and domain.BacklogItemTransitionInput are identical types.
type BacklogItemTransitionInput = domain.BacklogItemTransitionInput

// TransitionGuard validates business rules before a status transition.
var TransitionGuard = domain.TransitionGuard
