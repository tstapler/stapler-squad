package backlogdto

import "errors"

// ErrPRReassignmentNotAllowed is returned by SetBacklogItemPRAndTransition
// when a caller attempts to reassign an already-pr_pending item's tracked
// PR to a different PR number without a valid PRReassignmentGuard.
var ErrPRReassignmentNotAllowed = errors.New("PR reassignment not allowed")

// ErrPRDetached is returned when a writer tries to attach a PR the owner detached.
var ErrPRDetached = errors.New("PR was detached from this owner")

// ErrDependencyCycle is returned by AddBacklogItemDependency when the new
// blocker->blocked edge would create a cycle in the dependency graph.
var ErrDependencyCycle = errors.New("backlog item dependency would create a cycle")

// ErrAcCriterionWaived is returned when an agent-driven status update targets a
// criterion that was waived; only the audited waiver path changes it.
var ErrAcCriterionWaived = errors.New("acceptance criterion was waived")

// ErrAcCriterionOutOfRange is returned when a criterion index does not exist on the item.
var ErrAcCriterionOutOfRange = errors.New("acceptance criterion index out of range")

// ErrWaiverReasonRequired is returned when a waiver carries a blank reason.
var ErrWaiverReasonRequired = errors.New("a waiver needs a non-empty reason")

// ErrAcCriterionAlreadyWaived is returned when a waiver targets a criterion that is already waived.
var ErrAcCriterionAlreadyWaived = errors.New("acceptance criterion is already waived")

// ErrAcWaiverViaUpdate is returned when an item update tries to introduce a waived criterion;
// waivers are recorded only through WaiveAcCriterion so they are audited.
var ErrAcWaiverViaUpdate = errors.New("a waiver can only be recorded through WaiveAcCriterion")

// ErrAcCriterionAlreadyDone is returned when a waiver targets a criterion already done.
var ErrAcCriterionAlreadyDone = errors.New("acceptance criterion is already done; nothing to waive")
