package session

import (
	"encoding/json"
	"errors"
)

// BacklogStatus represents the lifecycle state of a backlog item.
type BacklogStatus string

const (
	BacklogStatusIdea       BacklogStatus = "idea"
	BacklogStatusRefining   BacklogStatus = "refining"
	BacklogStatusReady      BacklogStatus = "ready"
	BacklogStatusInProgress BacklogStatus = "in_progress"
	BacklogStatusReview     BacklogStatus = "review"
	BacklogStatusDone       BacklogStatus = "done"
	BacklogStatusArchived   BacklogStatus = "archived"
	BacklogStatusDuplicate  BacklogStatus = "duplicate"
)

// Session role constants.
const (
	SessionRoleWork   = "work"
	SessionRoleTriage = "triage"
	SessionRoleReview = "review"
)

// TriggeredBy values for BacklogStatusEvent records.
const (
	TriggeredByUser   = "user"
	TriggeredBySystem = "system"
)

// DefaultBacklogPriority is the default priority assigned to new backlog items
// when no priority is specified. Lower values indicate higher priority.
const DefaultBacklogPriority = 3

// AcCriterion is a single acceptance criterion for a backlog item.
type AcCriterion struct {
	Index  int    `json:"index"`
	Text   string `json:"text"`
	Status string `json:"status"` // "pending", "in_progress", "done"
}

// ParseAcCriteria deserializes acceptance criteria from a JSON string.
func ParseAcCriteria(raw string) ([]AcCriterion, error) {
	if raw == "" {
		return nil, nil
	}
	var criteria []AcCriterion
	if err := json.Unmarshal([]byte(raw), &criteria); err != nil {
		return nil, err
	}
	return criteria, nil
}

// SerializeAcCriteria serializes acceptance criteria to a JSON string.
func SerializeAcCriteria(criteria []AcCriterion) (string, error) {
	b, err := json.Marshal(criteria)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Review verdict outcome constants.
const (
	ReviewVerdictPass         = "PASS"
	ReviewVerdictFail         = "FAIL"
	ReviewVerdictPartial      = "PARTIAL"
	ReviewVerdictUnverifiable = "UNVERIFIABLE"
)

// CriterionVerdict holds the review outcome for a single acceptance criterion.
type CriterionVerdict struct {
	CriterionIndex int    `json:"criterion_index"`
	Outcome        string `json:"outcome"`
	Evidence       string `json:"evidence"`
}

// AggregateOutcome computes the overall outcome from a slice of CriterionVerdicts.
// Priority (highest to lowest): FAIL > PARTIAL > UNVERIFIABLE > PASS.
// Returns PASS only if every verdict is PASS.
func AggregateOutcome(verdicts []CriterionVerdict) string {
	if len(verdicts) == 0 {
		// No criteria evaluated — treat as FAIL, not PASS, to prevent auto-approval
		// of reviews that somehow bypassed the non-empty validation in submit_review_verdict.
		return ReviewVerdictFail
	}

	hasFail := false
	hasPartial := false
	hasUnverifiable := false

	for _, v := range verdicts {
		switch v.Outcome {
		case ReviewVerdictFail:
			hasFail = true
		case ReviewVerdictPartial:
			hasPartial = true
		case ReviewVerdictUnverifiable:
			hasUnverifiable = true
		}
	}

	switch {
	case hasFail:
		return ReviewVerdictFail
	case hasPartial:
		return ReviewVerdictPartial
	case hasUnverifiable:
		return ReviewVerdictUnverifiable
	default:
		return ReviewVerdictPass
	}
}

// validTransitions is the authoritative state machine transition table.
// idea→ready is a fast-track for items that already have AC written; items
// needing AC work should go idea→refining→ready instead.
var validTransitions = map[BacklogStatus]map[BacklogStatus]bool{
	BacklogStatusIdea: {
		BacklogStatusReady:     true,
		BacklogStatusRefining:  true,
		BacklogStatusArchived:  true,
		BacklogStatusDuplicate: true,
	},
	BacklogStatusRefining: {
		BacklogStatusReady:     true,
		BacklogStatusArchived:  true,
		BacklogStatusDuplicate: true,
	},
	BacklogStatusReady: {
		BacklogStatusInProgress: true,
		BacklogStatusIdea:       true,
		BacklogStatusArchived:   true,
		BacklogStatusDuplicate:  true,
	},
	BacklogStatusInProgress: {
		BacklogStatusReview:    true,
		BacklogStatusReady:     true,
		BacklogStatusDuplicate: true,
	},
	BacklogStatusReview: {
		BacklogStatusDone:       true,
		BacklogStatusInProgress: true,
		BacklogStatusDuplicate:  true,
	},
	BacklogStatusDone: {
		BacklogStatusReview:   true,
		BacklogStatusArchived: true,
	},
	BacklogStatusArchived: {
		BacklogStatusIdea: true,
	},
	BacklogStatusDuplicate: {
		BacklogStatusIdea: true,
	},
}

// CanTransitionBacklog reports whether a transition from one backlog status to another is permitted.
func CanTransitionBacklog(from, to BacklogStatus) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// Sentinel errors for transition guards.
var (
	ErrACRequired               = errors.New("acceptance criteria required before marking ready")
	ErrPlanRequired             = errors.New("plan must be approved or skip_planning must be true before spawning work session")
	ErrPlanArtifactsRequired    = errors.New("plan artifacts path is required when planning is not skipped")
	ErrVerdictRequired          = errors.New("PASS verdict or manual override required before marking done")
	ErrDuplicateOfRequired      = errors.New("duplicate_of_id is required when marking an item duplicate")
	ErrDuplicateOfSelf          = errors.New("duplicate_of_id cannot reference the item itself")
	ErrDuplicateOfInvalidTarget = errors.New("duplicate_of_id does not reference a valid (existing, non-duplicate) backlog item")
)

// BacklogItemTransitionInput carries the fields needed by TransitionGuard.
type BacklogItemTransitionInput struct {
	Status            BacklogStatus
	AcCriteriaJSON    string
	PlanApproved      bool
	SkipPlanning      bool
	PlanArtifactsPath string // path to plan artifacts written by triage session
	OverallOutcome    string // from linked ReviewVerdict
	OverrideReason    string
	ID                string        // the item's own id — needed for the self-reference check
	DuplicateOfID     string        // resolved from the transition request
	DuplicateOfExists bool          // resolved by caller via prior GetBacklogItem lookup
	DuplicateOfStatus BacklogStatus // resolved from the same lookup; used for chain-prevention
}

// NewTransitionInputFromItem builds the caller-independent portion of a
// BacklogItemTransitionInput from an already-fetched backlog item. Callers
// layer on any transition-specific fields (OverallOutcome, OverrideReason,
// DuplicateOfID, DuplicateOfExists, DuplicateOfStatus, etc.) afterward.
func NewTransitionInputFromItem(item *BacklogItemData) BacklogItemTransitionInput {
	return BacklogItemTransitionInput{
		ID:                item.ID,
		Status:            BacklogStatus(item.Status),
		AcCriteriaJSON:    item.AcceptanceCriteria,
		PlanApproved:      item.PlanApproved,
		SkipPlanning:      item.SkipPlanning,
		PlanArtifactsPath: item.PlanArtifactsPath,
	}
}

// TransitionGuard validates business rules before a status transition.
// It returns nil when the transition is allowed, or a sentinel error when a
// guard condition is violated. It does NOT check CanTransition — callers must
// invoke CanTransition separately if structural validity is also required.
func TransitionGuard(item BacklogItemTransitionInput, to BacklogStatus) error {
	from := item.Status

	switch {
	case from == BacklogStatusIdea && to == BacklogStatusReady:
		criteria, err := ParseAcCriteria(item.AcCriteriaJSON)
		if err != nil || len(criteria) == 0 {
			return ErrACRequired
		}
		return nil

	case from == BacklogStatusReady && to == BacklogStatusInProgress:
		if !item.PlanApproved && !item.SkipPlanning {
			return ErrPlanRequired
		}
		if item.PlanApproved && !item.SkipPlanning && item.PlanArtifactsPath == "" {
			return ErrPlanArtifactsRequired
		}
		return nil

	case from == BacklogStatusRefining && to == BacklogStatusReady:
		criteria, err := ParseAcCriteria(item.AcCriteriaJSON)
		if err != nil || len(criteria) == 0 {
			return ErrACRequired
		}
		return nil

	case from == BacklogStatusReview && to == BacklogStatusDone:
		if item.OverrideReason != "" {
			return nil
		}
		if item.OverallOutcome != ReviewVerdictPass {
			return ErrVerdictRequired
		}
		return nil

	case to == BacklogStatusDuplicate:
		if item.DuplicateOfID == "" {
			return ErrDuplicateOfRequired
		}
		if item.DuplicateOfID == item.ID {
			return ErrDuplicateOfSelf
		}
		if !item.DuplicateOfExists || item.DuplicateOfStatus == BacklogStatusDuplicate {
			return ErrDuplicateOfInvalidTarget
		}
		return nil

	default:
		// All other permitted transitions have no additional guards.
		return nil
	}
}
