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
	BacklogStatusQueued     = domain.BacklogStatusQueued
	BacklogStatusInProgress = domain.BacklogStatusInProgress
	BacklogStatusReview     = domain.BacklogStatusReview
	BacklogStatusPRPending  = domain.BacklogStatusPRPending
	BacklogStatusDone       = domain.BacklogStatusDone
	BacklogStatusArchived   = domain.BacklogStatusArchived
)

// BacklogCategory represents a coarse frontend-defaulting classification for
// a backlog item (bugfix/feature/chore/refactor, or "" for uncategorized).
// Type alias — session.BacklogCategory and domain.BacklogCategory are
// identical types; all existing callers continue to work without any import
// changes.
type BacklogCategory = domain.BacklogCategory

const (
	BacklogCategoryBugfix   = domain.BacklogCategoryBugfix
	BacklogCategoryFeature  = domain.BacklogCategoryFeature
	BacklogCategoryChore    = domain.BacklogCategoryChore
	BacklogCategoryRefactor = domain.BacklogCategoryRefactor
)

// IsValidBacklogCategory reports whether s is a known backlog category value
// or the empty string (uncategorized).
func IsValidBacklogCategory(s string) bool {
	return BacklogCategory(s).IsValid()
}

// Session role constants.
const (
	SessionRoleWork   = "work"
	SessionRoleTriage = "triage"
	SessionRoleReview = "review"
)

// IsTmuxBackedSessionRole reports whether role identifies a session that runs as a
// persistent, live tmux-attached claude process — one that must be explicitly
// archived AND have its tmux pane killed once its backlog item goes terminal, or it
// leaks indefinitely (root cause of the 2026-07-29 OOM: dozens of done/archived
// items' work and review sessions still running, each with its own MCP subprocess
// fleet). Work and review sessions are tmux-backed. Triage sessions are not: they run
// as bounded one-shot headless subprocess calls (see headlessTriageUUIDPrefix) that
// exit on their own when the call returns, so they were never tracked as a live
// Instance in the first place and have nothing to kill — their own failure mode
// (a crashed/hung goroutine leaving a stale DB row) is handled separately by
// reconcileOrphanedTriageItems/reconcileOrphanedTriageRemediation.
//
// This is the single source of truth for "which roles does the terminal-item sweep
// clean up" — both reconcileTerminalItemSessions (session/backlog_lifecycle.go) and
// archiveItemWorkSessions (server/services/backlog_service.go) call this rather than
// each re-deriving the role set, so the two can't silently drift apart again the way
// they already did once (the archive-and-kill fix originally covered work sessions
// only; review sessions kept leaking until this predicate unified both call sites).
func IsTmuxBackedSessionRole(role string) bool {
	return role == SessionRoleWork || role == SessionRoleReview
}

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

// TriggeredBy values for BacklogStatusEvent records. Agent-initiated
// transitions (e.g. request_review, report_duplicate) use TriggeredByAgent.
const (
	TriggeredByUser   = "user"
	TriggeredBySystem = "system"
	TriggeredByAgent  = "agent"
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

// AllowedTransitionsBacklog returns the sorted set of statuses reachable from from.
var AllowedTransitionsBacklog = domain.AllowedTransitionsBacklog

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
