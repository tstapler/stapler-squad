package session

import (
	"encoding/json"
	"errors"
)

// BacklogStatus represents the lifecycle state of a backlog item.
type BacklogStatus string

const (
	BacklogStatusIdea       BacklogStatus = "idea"
	BacklogStatusReady      BacklogStatus = "ready"
	BacklogStatusInProgress BacklogStatus = "in_progress"
	BacklogStatusReview     BacklogStatus = "review"
	BacklogStatusDone       BacklogStatus = "done"
	BacklogStatusArchived   BacklogStatus = "archived"
)

// Session role constants.
const (
	SessionRoleWork   = "work"
	SessionRoleTriage = "triage"
	SessionRoleReview = "review"
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
var validTransitions = map[BacklogStatus]map[BacklogStatus]bool{
	BacklogStatusIdea: {
		BacklogStatusReady:    true,
		BacklogStatusArchived: true,
	},
	BacklogStatusReady: {
		BacklogStatusInProgress: true,
		BacklogStatusIdea:       true,
		BacklogStatusArchived:   true,
	},
	BacklogStatusInProgress: {
		BacklogStatusReview: true,
		BacklogStatusReady:  true,
	},
	BacklogStatusReview: {
		BacklogStatusDone:       true,
		BacklogStatusInProgress: true,
	},
	BacklogStatusDone: {
		BacklogStatusReview:   true,
		BacklogStatusArchived: true,
	},
	BacklogStatusArchived: {
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
	ErrACRequired             = errors.New("acceptance criteria required before marking ready")
	ErrPlanRequired           = errors.New("plan must be approved or skip_planning must be true before spawning work session")
	ErrPlanArtifactsRequired  = errors.New("plan artifacts path is required when planning is not skipped")
	ErrVerdictRequired        = errors.New("PASS verdict or manual override required before marking done")
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

	case from == BacklogStatusReview && to == BacklogStatusDone:
		if item.OverrideReason != "" {
			return nil
		}
		if item.OverallOutcome != ReviewVerdictPass {
			return ErrVerdictRequired
		}
		return nil

	default:
		// All other permitted transitions have no additional guards.
		return nil
	}
}
