package session

import "sort"

// SystemEdge is a (from,to) transition that automation or storage-direct
// writers depend on, so no operator edit may remove it.
type SystemEdge struct {
	From BacklogStatus
	To   BacklogStatus
}

// systemRequiredCore lists the non-archive edges (ADR-002 §3): engine-checked
// automation callers first, then storage-direct writers kept for UI honesty.
var systemRequiredCore = []SystemEdge{
	// Group 1: engine-checked automation.
	{BacklogStatusReady, BacklogStatusQueued},
	{BacklogStatusQueued, BacklogStatusInProgress},
	{BacklogStatusReady, BacklogStatusInProgress},
	// AttachSessionToItem moves any status that can reach in_progress there.
	{BacklogStatusDone, BacklogStatusInProgress},
	// Group 2: storage-direct writers.
	{BacklogStatusInProgress, BacklogStatusReview},
	{BacklogStatusReview, BacklogStatusDone},
	{BacklogStatusReview, BacklogStatusPRPending},
	{BacklogStatusPRPending, BacklogStatusDone},
	{BacklogStatusReview, BacklogStatusInProgress},
	{BacklogStatusPRPending, BacklogStatusInProgress},
	{BacklogStatusInProgress, BacklogStatusReady},
	{BacklogStatusIdea, BacklogStatusReady},
}

// SystemRequiredEdges returns the enumerable system-required set: the core
// edges plus the archive group (X->archived for every built-in status X, and
// archived->archived). It does not list custom-stage archive edges; use
// IsSystemRequired for those. validTransitions is deliberately unchanged, so
// Off and Shadow keep answering exactly as before.
func SystemRequiredEdges() []SystemEdge {
	edges := make([]SystemEdge, 0, len(systemRequiredCore)+len(builtInStageOrder))
	edges = append(edges, systemRequiredCore...)
	for _, from := range builtInStageOrder {
		edges = append(edges, SystemEdge{from, BacklogStatusArchived})
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
func IsSystemRequired(from, to BacklogStatus) bool {
	if to == BacklogStatusArchived {
		return true
	}
	for _, e := range systemRequiredCore {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

// systemRequiredTargets returns the non-archive system-required destinations
// from from. The archive group is excluded so the UI-facing allowed list never
// offers archive via the engine (Story 1.5.1 AC 3).
func systemRequiredTargets(from BacklogStatus) []BacklogStatus {
	var out []BacklogStatus
	for _, e := range systemRequiredCore {
		if e.From == from {
			out = append(out, e.To)
		}
	}
	return out
}
