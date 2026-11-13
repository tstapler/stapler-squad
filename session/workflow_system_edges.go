package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"sort"
)

// systemRequiredCore lists the non-archive edges (ADR-002 §3): engine-checked
// automation callers first, then storage-direct writers kept for UI honesty.
var systemRequiredCore = []domain.SystemEdge{
	// Group 1: engine-checked automation.
	{From: domain.BacklogStatusReady, To: domain.BacklogStatusQueued},
	{From: domain.BacklogStatusQueued, To: domain.BacklogStatusInProgress},
	{From: domain.BacklogStatusReady, To: domain.BacklogStatusInProgress},
	// AttachSessionToItem moves any status that can reach in_progress there.
	{From: domain.BacklogStatusDone, To: domain.BacklogStatusInProgress},
	// Group 2: storage-direct writers.
	{From: domain.BacklogStatusInProgress, To: domain.BacklogStatusReview},
	{From: domain.BacklogStatusReview, To: domain.BacklogStatusDone},
	{From: domain.BacklogStatusReview, To: domain.BacklogStatusPRPending},
	{From: domain.BacklogStatusPRPending, To: domain.BacklogStatusDone},
	{From: domain.BacklogStatusReview, To: domain.BacklogStatusInProgress},
	{From: domain.BacklogStatusPRPending, To: domain.BacklogStatusInProgress},
	{From: domain.BacklogStatusInProgress, To: domain.BacklogStatusReady},
	{From: domain.BacklogStatusIdea, To: domain.BacklogStatusReady},
	// request_review re-requests from pr_pending (Task 1.5.1b audit).
	{From: domain.BacklogStatusPRPending, To: domain.BacklogStatusReview},
	// report_blocked from review.
	{From: domain.BacklogStatusReview, To: domain.BacklogStatusReady},
	// respawnTriage resets a queued item before retriage.
	{From: domain.BacklogStatusQueued, To: domain.BacklogStatusIdea},
}

// systemRequiredOffGraph lists registered edges that validTransitions lacks
// but real callers write (Task 1.5.1b audit, ADR-007 D7): SkipReviewGate
// completion, the pushAndCreatePR fallback, and drift recovery to pr_pending.
// Like the archive group they never reach the UI-facing allowed list.
var systemRequiredOffGraph = []domain.SystemEdge{
	{From: domain.BacklogStatusInProgress, To: domain.BacklogStatusDone},
	{From: domain.BacklogStatusReady, To: domain.BacklogStatusDone},
	{From: domain.BacklogStatusIdea, To: domain.BacklogStatusPRPending},
	{From: domain.BacklogStatusRefining, To: domain.BacklogStatusPRPending},
	{From: domain.BacklogStatusReady, To: domain.BacklogStatusPRPending},
	{From: domain.BacklogStatusQueued, To: domain.BacklogStatusPRPending},
	{From: domain.BacklogStatusInProgress, To: domain.BacklogStatusPRPending},
}

// SystemRequiredEdges returns the enumerable system-required set: the core
// edges, the archive group (X->archived for every built-in status X, and
// archived->archived) and the off-graph recovery edges. It does not list
// custom-stage archive edges; use IsSystemRequired for those. validTransitions
// is deliberately unchanged, so Off and Shadow keep answering exactly as before.
func SystemRequiredEdges() []domain.SystemEdge {
	edges := make([]domain.SystemEdge, 0, len(systemRequiredCore)+len(systemRequiredOffGraph)+len(BuiltInStageOrder))
	edges = append(edges, systemRequiredCore...)
	edges = append(edges, systemRequiredOffGraph...)
	for _, from := range BuiltInStageOrder {
		edges = append(edges, domain.SystemEdge{From: from, To: domain.BacklogStatusArchived})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges
}

// IsSystemRequired reports whether (from,to) is system-required. Any
// transition into archived counts, so an item stranded on a custom or deleted
// stage can still be archived.
func IsSystemRequired(from, to domain.BacklogStatus) bool {
	if to == domain.BacklogStatusArchived {
		return true
	}
	for _, group := range [][]domain.SystemEdge{systemRequiredCore, systemRequiredOffGraph} {
		for _, e := range group {
			if e.From == from && e.To == to {
				return true
			}
		}
	}
	return false
}

// systemRequiredTargets returns the non-archive system-required destinations
// from from. The archive group is excluded so the UI-facing allowed list never
// offers archive via the engine (Story 1.5.1 AC 3).
func SystemRequiredTargets(from domain.BacklogStatus) []domain.BacklogStatus {
	var out []domain.BacklogStatus
	for _, e := range systemRequiredCore {
		if e.From == from {
			out = append(out, e.To)
		}
	}
	return out
}
