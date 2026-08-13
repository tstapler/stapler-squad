// Package domain contains pure domain types for the backlog subsystem.
// It imports only standard library packages — no ent, no headless, no git deps —
// so it can be imported by any layer (server, adapters, pkg/events) without
// creating an import cycle with the parent session package.
package domain

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
	BacklogStatusQueued     BacklogStatus = "queued"
	BacklogStatusInProgress BacklogStatus = "in_progress"
	BacklogStatusReview     BacklogStatus = "review"
	BacklogStatusPRPending  BacklogStatus = "pr_pending"
	BacklogStatusDone       BacklogStatus = "done"
	BacklogStatusArchived   BacklogStatus = "archived"
)

// DefaultBacklogPriority is the default priority assigned to new backlog items
// when no priority is specified. Lower values indicate higher priority.
const DefaultBacklogPriority = 3

// StuckReason is a validated string-backed enum of the classes a backlog item
// can be "stuck" for — matching the house BacklogStatus/ReviewOutcome style
// (validated at the boundary via IsValid, not a truly-unrepresentable sum
// type). Only these compile-time constants should ever reach MarkStuck; no
// unvalidated string should reach the DB.
type StuckReason string

const (
	// StuckReasonPRReadyUnmerged: a pr_pending item's PR is green, mergeable,
	// and unmerged past the threshold (see prReadyToMergeSolo).
	StuckReasonPRReadyUnmerged StuckReason = "pr_ready_unmerged"
	// StuckReasonReworkCap: the auto-rework loop hit maxAutoReworkIterations
	// and parked the item for manual action.
	StuckReasonReworkCap StuckReason = "rework_cap"
	// StuckReasonAbandonedReview: a review-status item has a review verdict on
	// record but nothing active in flight.
	StuckReasonAbandonedReview StuckReason = "abandoned_review"
	// StuckReasonStaleWork: an in_progress item's active work session reported
	// no progress for longer than maxWorkSessionStaleness.
	StuckReasonStaleWork StuckReason = "stale_work"
	// StuckReasonBouncing: an item crossed in_progress <-> review >= bounceThreshold
	// times within bounceLookback with no PASS verdict.
	StuckReasonBouncing StuckReason = "bouncing"
	// StuckReasonPushFailed: pushAndCreatePR failed (push rejected / gh pr
	// create errored) leaving a post-review item with no pr_number.
	StuckReasonPushFailed StuckReason = "push_failed"
	// StuckReasonOrphanedTriage: an idea-status item's triage session ended
	// without ever transitioning the item to ready. Covers two shapes: the
	// session is still open and has gone stale (crashed, was killed, or the
	// server restarted mid-triage), or the session already ended cleanly (the
	// headless call errored, or returned output the triage parser rejected —
	// e.g. a premature "still working" status message instead of the final
	// JSON block, confirmed live 2026-07-29/30, see
	// docs/tasks/backlog-feature-improvement.md's 2026-07-30 entry) but
	// TriggerTriage never reached its idea->ready transition. Previously only
	// the first shape was detected, and only when a human manually
	// re-triggered triage (tombstoneOrphanTriageSessions); this reason lets the
	// periodic stuck sweep catch both shapes without a manual retry — see
	// reconcileOrphanedTriageItems (session/backlog_lifecycle.go).
	StuckReasonOrphanedTriage StuckReason = "orphaned_triage"
	// StuckReasonAutonomousStuck: an autonomous driver run stopped after
	// maxTurns without a DONE signal. Previously only surfaced as a one-off
	// ephemeral notification (onAutonomousDriverComplete), invisible to the
	// Unfinished tab's durable stuck-reason system.
	StuckReasonAutonomousStuck StuckReason = "autonomous_stuck"
	// StuckReasonSpawnFailed: AutoReopenAfterFailedReview transitioned an item
	// to in_progress, then SpawnSessionFromItem failed AND the scoped rollback
	// to "review" also failed (its precondition no longer matched — something
	// else touched the item in the interim). Previously this left the item
	// silently stranded at in_progress with no work session and no visible
	// error (server/services/backlog_service_triage.go's rollback branch only
	// logged it) — invisible to every other stuck detector, since none of them
	// check "in_progress with zero live sessions and no error surfaced."
	StuckReasonSpawnFailed StuckReason = "spawn_failed"
	// StuckReasonPlanNotApproved: DequeueNextQueuedItems' planning gate
	// (SkipPlanning=false, PlanApproved=false) refuses to claim a queued item
	// indefinitely — by design (see that function's doc comment) — with only a
	// per-tick WARNING log and no durable, human-visible signal. Confirmed live
	// 2026-07-22: three items sat queued for days, silently re-blocked on every
	// 60s tick, invisible on the kanban board (BUG-037) and with no "Approve
	// Plan" action anywhere in the UI to unblock them.
	StuckReasonPlanNotApproved StuckReason = "plan_not_approved"
	// StuckReasonPRPendingNoPR: an item is in pr_pending status but has no PR
	// reference (pr_number == 0, pr_url == ""). Every downstream reconciler
	// (ReconcilePRPending's FindPRPendingItems query, EnablePRAutoMerge, etc.)
	// requires a real PrNumber, so an item in this shape is invisible to
	// everything else and sits in pr_pending permanently with nothing left to
	// poll or retry (BUG-040). Detection-only backstop: two write-ordering
	// bugs (pushAndCreatePR's best-effort field persist; ReconcilePRPending's
	// closed-PR branch clearing fields before confirming the reopen actually
	// succeeded) were found and fixed as the direct cause of the live incident
	// this reason was added for, but this detector exists so any *future*
	// mistake with the same shape — "a write silently doesn't happen or
	// happens out of order, and nothing detects the resulting dead end" — is
	// still visible and retryable from /unfinished rather than a silent
	// permanent stall.
	StuckReasonPRPendingNoPR StuckReason = "pr_pending_no_pr"
	// StuckReasonReworkBlockedStale: a review-status item's failed-review
	// rework attempt is blocked because its prior work session is still alive
	// (hasActiveWorkSession's guard, in AutoReopenAfterFailedReview) but has
	// produced no output for longer than maxReworkBlockStaleness (15min — see
	// project_plans/review-gate-stale-session-rework/decisions/ADR-001-
	// staleness-threshold-recalibration.md). Set by
	// notifyIfActiveWorkSessionStale (server/services/backlog_service_triage.go),
	// resolved by ResolveReworkBlockedStaleIfRecovered once the session
	// produces output again, leaves review, or its work session ends.
	// Distinct from StuckReasonStaleWork, which covers the structurally
	// similar but different-status case of an in_progress item's active work
	// session going stale — the two are deliberately kept as separate reasons
	// (different item status, different threshold, different urgency) rather
	// than merged.
	StuckReasonReworkBlockedStale StuckReason = "rework_blocked_stale"
	// StuckReasonPRNeedsFix: a pr_pending item's PR has failing CI, blocking
	// reviews, or a merge conflict (ReconcilePRPending's spawn-fix branches).
	// Gates ReconcilePRPending's AutoReopenForPRFix dispatch through the
	// shared remediation backoff (Storage.RemediationDue) so a PR that keeps
	// failing CI doesn't get a fresh fix session spawned on every ~60s
	// reconciliation tick indefinitely — previously ungated, unlike every
	// sibling remediation call site in session/backlog_lifecycle.go (see
	// docs/tasks/backlog-feature-improvement.md's 2026-07-28 entry). Resolved
	// once the PR becomes healthy again or the item reaches done.
	StuckReasonPRNeedsFix StuckReason = "pr_needs_fix"
	// StuckReasonRespawnBlockedActive: an automated respawn attempt
	// (AutoRespawnAutonomousWork, AutoReopenForPRFix, or AutoRespawnReview —
	// server/services/backlog_service_triage.go) was skipped because the item
	// already has an active work or review session, per
	// findActiveWorkSession/findActiveReviewSession. Before this reason
	// existed, all three call sites only log.InfoLog.Printf'd the skip —
	// zero operator-visible signal and no audit record, strictly worse than
	// spawnSessionAfterGates' own 8b guard (activeWorkSessionBlockedError),
	// which at least returns a progress-enriched error to its synchronous
	// caller (docs/tasks/backlog-feature-improvement.md, 2026-07-31/
	// 2026-08-03 updates). Set by notifyRespawnBlockedByActiveSession, which
	// reuses workSessionStaleness for the same "still active" vs. "likely
	// stalled" distinction. Distinct from StuckReasonReworkBlockedStale
	// (review-status-only, staleness-gated) since this fires regardless of
	// staleness and covers three different item statuses (in_progress,
	// pr_pending, review). Resolved the next time the guarding function runs
	// past its active-session check (the block has cleared).
	StuckReasonRespawnBlockedActive StuckReason = "respawn_blocked_active"
	// StuckReasonLikelyFlaky: session.IsFlakyVerdictFlipFlop or
	// session.IsTestOnlyReworkCycle matched on this item's recent review
	// history — behavioral evidence (not a keyword match on title/description;
	// see project_plans/backlog-bounce-escalation/decisions/ADR-002) that the
	// review outcome may be non-deterministic rather than a real pass/fail
	// signal. Purely informational: set alongside AutoReopenAfterFailedReview's
	// existing reopen/park decision, never gating it — a misfiring heuristic
	// here cannot newly stall an item that would otherwise proceed. Both
	// predicates carry documented false-positive sources (see their doc
	// comments in session/stuck_decisions.go); the UI should present this as a
	// hint to verify, not a confident verdict.
	StuckReasonLikelyFlaky StuckReason = "likely_flaky"
	// StuckReasonBlockedByDependency: DequeueNextQueuedItems' dependency gate
	// (UnresolvedBlockerItemIDs) skipped this item because at least one of its
	// blocker items (session/ent/schema/backlog_item_dependency.go) has not yet
	// reached a resolved status (done or archived — see
	// project_plans/backlog-item-dependencies/decisions/ADR-001-dangling-
	// blocker-resolution.md). Purely detection/visibility: mirrors
	// StuckReasonPlanNotApproved's precedent of surfacing an indefinite,
	// by-design dequeue skip so it's visible on /unfinished and in the item
	// detail view (BlockerChip) instead of only a per-tick log line. Resolved
	// the next time UnresolvedBlockerItemIDs finds no unresolved blocker left
	// for this item.
	StuckReasonBlockedByDependency StuckReason = "blocked_by_dependency"
	// StuckReasonMultipleReasons: a synthetic, aggregate reason marking an
	// item that has multiReasonThreshold or more *other*, non-escalation
	// stuck reasons open simultaneously (session/stuck_decisions.go's
	// isMultiReasonEscalated). Set/resolved by reconcileMultiReasonEscalation
	// (session/backlog_lifecycle_stuck.go) as the count of open non-escalation
	// reasons crosses the threshold in either direction. Deliberately excludes
	// itself and StuckReasonBounceCapExhausted from its own count (ADR-001)
	// to avoid a self-reinforcing escalation loop. Unlike every other
	// StuckReason, this one carries no independent remediation action of its
	// own — it exists purely as an operator-visible severity signal that an
	// item is stuck for multiple simultaneous reasons, not a single narrow
	// one.
	StuckReasonMultipleReasons StuckReason = "multiple_reasons"
	// StuckReasonBounceCapExhausted: a synthetic, aggregate reason marking an
	// item whose bouncing remediation gate hit MaxRemediationAttempts while
	// StuckReasonBouncing itself is still open (autoReopenWithBackoffGate,
	// session/backlog_lifecycle.go). Signals that automated remediation has
	// given up retrying and the item now needs human intervention. Resolved
	// by reconcileBouncingItems once StuckReasonBouncing itself resolves, or
	// by the selfHealStuck backstop once the item's status leaves
	// in_progress/review entirely. Like StuckReasonMultipleReasons, this is a
	// meta/aggregate signal with no independent remediation action of its
	// own.
	StuckReasonBounceCapExhausted StuckReason = "bounce_cap_exhausted"
)

// AllStuckReasons lists every valid StuckReason constant.
var AllStuckReasons = []StuckReason{
	StuckReasonPRReadyUnmerged,
	StuckReasonReworkCap,
	StuckReasonAbandonedReview,
	StuckReasonStaleWork,
	StuckReasonBouncing,
	StuckReasonPushFailed,
	StuckReasonOrphanedTriage,
	StuckReasonAutonomousStuck,
	StuckReasonSpawnFailed,
	StuckReasonPlanNotApproved,
	StuckReasonPRPendingNoPR,
	StuckReasonReworkBlockedStale,
	StuckReasonPRNeedsFix,
	StuckReasonRespawnBlockedActive,
	StuckReasonLikelyFlaky,
	StuckReasonBlockedByDependency,
	StuckReasonMultipleReasons,
	StuckReasonBounceCapExhausted,
}

// IsValid reports whether r is a known stuck reason value.
func (r StuckReason) IsValid() bool {
	switch r {
	case StuckReasonPRReadyUnmerged, StuckReasonReworkCap, StuckReasonAbandonedReview,
		StuckReasonStaleWork, StuckReasonBouncing, StuckReasonPushFailed, StuckReasonOrphanedTriage,
		StuckReasonAutonomousStuck, StuckReasonSpawnFailed, StuckReasonPlanNotApproved,
		StuckReasonPRPendingNoPR, StuckReasonReworkBlockedStale, StuckReasonPRNeedsFix,
		StuckReasonRespawnBlockedActive, StuckReasonLikelyFlaky, StuckReasonBlockedByDependency,
		StuckReasonMultipleReasons, StuckReasonBounceCapExhausted:
		return true
	}
	return false
}

// AcStatus represents the status of a single acceptance criterion.
type AcStatus string

const (
	AcStatusPending    AcStatus = "pending"
	AcStatusInProgress AcStatus = "in_progress"
	AcStatusDone       AcStatus = "done"
	AcStatusFail       AcStatus = "fail"
)

// IsValid reports whether s is a known AC status value.
func (s AcStatus) IsValid() bool {
	switch s {
	case AcStatusPending, AcStatusInProgress, AcStatusDone, AcStatusFail:
		return true
	}
	return false
}

// BacklogCategory is a validated string-backed enum classifying a backlog
// item into a coarse bucket (bugfix/feature/chore/refactor). It is purely a
// frontend-defaulting hint — BacklogItemForm.tsx applies each category's
// automation-toggle defaults once, at creation time, into local form state;
// the server only persists and validates the string itself (see
// session.IsValidBacklogCategory), it does not resolve or apply any
// defaults. Unlike StuckReason above, the empty string is a valid value here
// too, meaning "uncategorized" — today's behavior for every existing item,
// preserved exactly.
type BacklogCategory string

const (
	BacklogCategoryBugfix   BacklogCategory = "bugfix"
	BacklogCategoryFeature  BacklogCategory = "feature"
	BacklogCategoryChore    BacklogCategory = "chore"
	BacklogCategoryRefactor BacklogCategory = "refactor"
)

// IsValid reports whether c is a known backlog category, or the empty string
// (uncategorized).
func (c BacklogCategory) IsValid() bool {
	switch c {
	case "", BacklogCategoryBugfix, BacklogCategoryFeature, BacklogCategoryChore, BacklogCategoryRefactor:
		return true
	}
	return false
}

// AcCriteriaJSON is the JSON-serialized form of []AcCriterion stored in the DB.
// Using a named type prevents silently passing Description or other string fields
// where serialized AC criteria are expected.
type AcCriteriaJSON string

// AcCriteriaJSONEmpty is the zero value — an empty criteria list.
const AcCriteriaJSONEmpty AcCriteriaJSON = ""

// Parse deserializes the criteria from JSON.
func (j AcCriteriaJSON) Parse() ([]AcCriterion, error) {
	return ParseAcCriteria(j)
}

// IsEmpty reports whether j contains no criteria JSON.
func (j AcCriteriaJSON) IsEmpty() bool { return j == "" }

// AcCriterion is a single acceptance criterion for a backlog item.
type AcCriterion struct {
	Index  int      `json:"index"`
	Text   string   `json:"text"`
	Status AcStatus `json:"status"` // pending, in_progress, done, fail
	Note   string   `json:"note,omitempty"`
}

// ParseAcCriteria deserializes acceptance criteria from a JSON string.
func ParseAcCriteria(raw AcCriteriaJSON) ([]AcCriterion, error) {
	if raw == "" {
		return nil, nil
	}
	var criteria []AcCriterion
	if err := json.Unmarshal([]byte(raw), &criteria); err != nil {
		return nil, err
	}
	return criteria, nil
}

// SerializeAcCriteria serializes acceptance criteria to an AcCriteriaJSON value.
func SerializeAcCriteria(criteria []AcCriterion) (AcCriteriaJSON, error) {
	b, err := json.Marshal(criteria)
	if err != nil {
		return "", err
	}
	return AcCriteriaJSON(b), nil
}

// ReviewOutcome is a typed verdict outcome value (PASS, FAIL, PARTIAL, UNVERIFIABLE).
type ReviewOutcome string

const (
	ReviewOutcomePass         ReviewOutcome = "PASS"
	ReviewOutcomeFail         ReviewOutcome = "FAIL"
	ReviewOutcomePartial      ReviewOutcome = "PARTIAL"
	ReviewOutcomeUnverifiable ReviewOutcome = "UNVERIFIABLE"
)

// IsValid reports whether o is a recognised review outcome.
func (o ReviewOutcome) IsValid() bool {
	switch o {
	case ReviewOutcomePass, ReviewOutcomeFail, ReviewOutcomePartial, ReviewOutcomeUnverifiable:
		return true
	}
	return false
}

// Backward-compatible aliases so callers can be migrated incrementally.
// Prefer ReviewOutcome* constants in new code.
const (
	ReviewVerdictPass         = ReviewOutcomePass
	ReviewVerdictFail         = ReviewOutcomeFail
	ReviewVerdictPartial      = ReviewOutcomePartial
	ReviewVerdictUnverifiable = ReviewOutcomeUnverifiable
)

// CriterionVerdict holds the review outcome for a single acceptance criterion.
type CriterionVerdict struct {
	CriterionIndex int           `json:"criterion_index"`
	Outcome        ReviewOutcome `json:"outcome"`
	Evidence       string        `json:"evidence"`
}

// AggregateOutcome computes the overall outcome from a slice of CriterionVerdicts.
// Priority (highest to lowest): FAIL > PARTIAL > UNVERIFIABLE > PASS.
// Returns FAIL when the slice is empty to prevent auto-approval of empty reviews.
func AggregateOutcome(verdicts []CriterionVerdict) ReviewOutcome {
	if len(verdicts) == 0 {
		// No criteria evaluated — treat as FAIL, not PASS, to prevent auto-approval
		// of reviews that somehow bypassed the non-empty validation in submit_review_verdict.
		return ReviewOutcomeFail
	}

	hasFail := false
	hasPartial := false
	hasUnverifiable := false

	for _, v := range verdicts {
		switch v.Outcome {
		case ReviewOutcomeFail:
			hasFail = true
		case ReviewOutcomePartial:
			hasPartial = true
		case ReviewOutcomeUnverifiable:
			hasUnverifiable = true
		}
	}

	switch {
	case hasFail:
		return ReviewOutcomeFail
	case hasPartial:
		return ReviewOutcomePartial
	case hasUnverifiable:
		return ReviewOutcomeUnverifiable
	default:
		return ReviewOutcomePass
	}
}

// validTransitions is the authoritative state machine transition table.
// idea→ready is a fast-track for items that already have AC written; items
// needing AC work should go idea→refining→ready instead.
var validTransitions = map[BacklogStatus]map[BacklogStatus]bool{
	BacklogStatusIdea: {
		BacklogStatusReady:    true,
		BacklogStatusRefining: true,
		BacklogStatusArchived: true,
	},
	BacklogStatusRefining: {
		BacklogStatusIdea:     true, // backward: re-triage
		BacklogStatusReady:    true,
		BacklogStatusArchived: true,
	},
	BacklogStatusReady: {
		BacklogStatusInProgress: true,
		BacklogStatusQueued:     true, // WIP cap hit at spawn time
		BacklogStatusIdea:       true, // backward: re-triage
		BacklogStatusRefining:   true, // backward: refine ACs
		BacklogStatusArchived:   true,
	},
	BacklogStatusQueued: {
		BacklogStatusInProgress: true, // dequeued: WIP slot freed up
		BacklogStatusReady:      true, // backward: manually un-queue
		BacklogStatusIdea:       true, // backward: re-triage from scratch
		BacklogStatusArchived:   true,
	},
	BacklogStatusInProgress: {
		BacklogStatusReview:   true,
		BacklogStatusReady:    true,
		BacklogStatusRefining: true, // backward: refine ACs/plan
		BacklogStatusIdea:     true, // backward: re-triage from scratch
	},
	BacklogStatusReview: {
		BacklogStatusPRPending:  true,
		BacklogStatusDone:       true,
		BacklogStatusInProgress: true,
		BacklogStatusReady:      true, // backward: re-spawn without re-triaging
		BacklogStatusRefining:   true, // backward: refine ACs/plan
		BacklogStatusIdea:       true, // backward: re-triage from scratch
	},
	BacklogStatusPRPending: {
		BacklogStatusDone:       true,
		BacklogStatusInProgress: true,
		BacklogStatusReview:     true,
		BacklogStatusReady:      true, // backward
		BacklogStatusRefining:   true, // backward
		BacklogStatusIdea:       true, // backward
	},
	BacklogStatusDone: {
		BacklogStatusReview:     true,
		BacklogStatusArchived:   true,
		BacklogStatusInProgress: true, // backward
		BacklogStatusReady:      true, // backward
		BacklogStatusRefining:   true, // backward
		BacklogStatusIdea:       true, // backward
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

// ValidTransitions returns a deep copy of the authoritative transition table.
// Callers that need a local snapshot (e.g. for concurrent reads without repeated
// map lookups) should call this once at construction time.
func ValidTransitions() map[BacklogStatus]map[BacklogStatus]bool {
	out := make(map[BacklogStatus]map[BacklogStatus]bool, len(validTransitions))
	for from, targets := range validTransitions {
		inner := make(map[BacklogStatus]bool, len(targets))
		for to, v := range targets {
			inner[to] = v
		}
		out[from] = inner
	}
	return out
}

// Sentinel errors for transition guards.
var (
	ErrACRequired                   = errors.New("acceptance criteria required before marking ready")
	ErrPlanRequired                 = errors.New("plan must be approved or skip_planning must be true before spawning work session")
	ErrPlanArtifactsRequired        = errors.New("plan artifacts path is required when planning is not skipped")
	ErrVerdictRequired              = errors.New("PASS verdict or manual override required before marking done")
	ErrCodeNotOnMain                = errors.New("code changes must actually be on main (merged locally or via a merged PR) before marking done; provide override_reason to bypass")
	ErrUnresolvedBlockers           = errors.New("item has one or more blockers that have not reached done")
	ErrVerdictClearRequiredForReady = errors.New("item has a recorded PASS verdict; provide override_reason to send it back to ready anyway")
)

// BacklogItemTransitionInput carries the fields needed by TransitionGuard.
type BacklogItemTransitionInput struct {
	Status            BacklogStatus
	AcCriteria        AcCriteriaJSON // serialized acceptance criteria
	PlanApproved      bool
	SkipPlanning      bool
	PlanArtifactsPath string        // path to plan artifacts written by triage session
	OverallOutcome    ReviewOutcome // from linked ReviewVerdict
	OverrideReason    string
	// HasUnshippedCode is true when a work session committed code
	// (LastCommitSha != "") that has not been verified to actually be on main —
	// locally (merged/committed directly) or remotely (merged PR, pulled or not).
	// A PrURL alone does NOT clear this: an open, unmerged, or later-reverted PR
	// still has PrURL set, so it was never proof the code shipped. The
	// review→done guard uses this to block premature done transitions.
	HasUnshippedCode bool
	// HasUnresolvedBlockers is true when this item has at least one
	// BacklogItemDependency where the blocker has not reached done. The
	// ready/queued->in_progress guard uses this to keep a gated item from
	// being dequeued/started ahead of its blocker. Callers populate this via
	// a batched query (see DequeueNextQueuedItems) rather than a per-item
	// lookup.
	HasUnresolvedBlockers bool
}

// TransitionGuard validates business rules before a status transition.
// It returns nil when the transition is allowed, or a sentinel error when a
// guard condition is violated. It does NOT check CanTransition — callers must
// invoke CanTransition separately if structural validity is also required.
func TransitionGuard(item BacklogItemTransitionInput, to BacklogStatus) error {
	from := item.Status

	switch {
	case from == BacklogStatusIdea && to == BacklogStatusReady:
		criteria, err := ParseAcCriteria(item.AcCriteria)
		if err != nil || len(criteria) == 0 {
			return ErrACRequired
		}
		return nil

	case from == BacklogStatusReady && to == BacklogStatusInProgress,
		from == BacklogStatusQueued && to == BacklogStatusInProgress:
		if item.HasUnresolvedBlockers {
			return ErrUnresolvedBlockers
		}
		if !item.PlanApproved && !item.SkipPlanning {
			return ErrPlanRequired
		}
		if item.PlanApproved && !item.SkipPlanning && item.PlanArtifactsPath == "" {
			return ErrPlanArtifactsRequired
		}
		return nil

	case from == BacklogStatusRefining && to == BacklogStatusReady:
		criteria, err := ParseAcCriteria(item.AcCriteria)
		if err != nil || len(criteria) == 0 {
			return ErrACRequired
		}
		return nil

	case (from == BacklogStatusReview || from == BacklogStatusPRPending) && to == BacklogStatusReady:
		// Backward "re-spawn without re-triaging" edge. Without this guard, an
		// item that already has a recorded PASS verdict can be sent back to
		// ready and get permanently stuck: report_pr_created only accepts
		// review/pr_pending, so the item can never complete that RPC again
		// without a status transition first. Scoped to review/pr_pending only —
		// must not affect the unrelated done->ready backward edge.
		if item.OverrideReason != "" {
			return nil
		}
		if item.OverallOutcome == ReviewOutcomePass {
			return ErrVerdictClearRequiredForReady
		}
		return nil

	case to == BacklogStatusDone:
		// Applies to both review->done and pr_pending->done — the only two edges
		// in validTransitions that reach "done". Previously this guard only
		// matched from == BacklogStatusReview, so a pr_pending item could be
		// marked done (e.g. via a manual "Approve" click) with no verdict/shipped
		// check at all: found live when a real backlog item reached done while
		// its GitHub PR was still open with merge conflicts, permanently
		// orphaning that PR from ReconcilePRPending's monitoring (which only
		// polls pr_pending-status items). The automated ReconcilePRPending path
		// that legitimately drives pr_pending->done already verifies
		// IsPRMerged() itself before calling this transition, so it always
		// carries a genuine PASS verdict and shipped code — this guard does not
		// change its behavior, only closes the gap for other callers.
		if item.OverrideReason != "" {
			return nil
		}
		if item.OverallOutcome != ReviewOutcomePass {
			return ErrVerdictRequired
		}
		if item.HasUnshippedCode {
			return ErrCodeNotOnMain
		}
		return nil

	default:
		// All other permitted transitions have no additional guards.
		return nil
	}
}
