package session

import (
	"errors"
	"fmt"
	"strings"
)

// Package graph_validator.go implements Epic 2.6's bespoke DFS graph
// validator (Story 2.6.1/2.6.2), per
// project_plans/backlog-custom-workflow-stages/implementation/plan.md's
// Pattern Decisions table ("bespoke DFS-based validator, with mandatory
// adversarial test coverage" — build-vs-buy.md §3 verdict) and its Domain
// Glossary's StageDefinition/TransitionDefinition entries.
//
// Task 2.6.1a — validation trigger point: this package never calls into the
// DB itself and is invoked synchronously, in-process, from every mutating
// stage/transition-graph RPC handler (CreateStage, CreateStageTransition,
// UpdateStageTransition — Epic 2.7), immediately before that handler's
// persist write, not from a separate explicit "validate graph" RPC. This
// matches the Pattern Decisions table's Service Layer CRUD convention
// (Story 2.7.2's own AC: "CreateStageTransition invokes the Epic 2.6 graph
// validator before committing") and means a caller always sees a
// validation error at the moment of the save that would have caused it,
// never a later out-of-band check.
//
// This file is deliberately storage-free: StageDefinition/TransitionDefinition
// are plain data views (mirroring the ent BacklogStage/StageTransition/
// TransitionGate schemas' field shapes, never *ent.X types directly) so this
// package stays pure and unit-testable without a DB fixture. Epic 2.7's RPC
// handlers — which do have DB access — are responsible for loading the
// current graph (and, for Task 2.6.1e's live-item-aware check, querying
// live-item counts per stage) and translating ent rows into these structs
// before calling ValidateGraph/ValidateDisableTransition.

// StageDefinition is this package's plain-data view of one stage graph node.
// Field names/semantics mirror ent's BacklogStage schema
// (session/ent/schema/backlog_stage.go) exactly.
type StageDefinition struct {
	Slug       string
	IsEntry    bool
	IsTerminal bool
}

// TransitionDefinition is this package's plain-data view of one (fromStage,
// toStage) edge, mirroring ent's StageTransition schema
// (session/ent/schema/stage_transition.go). GateCount is the number of gates
// attached to this edge — Story 2.6.2's cycle-escape lint only needs "does
// this edge have at least one gate," never gate kind/config, so a count
// (rather than a slice of gate detail) keeps this struct storage-free and
// small.
type TransitionDefinition struct {
	FromSlug  string
	ToSlug    string
	Enabled   bool
	GateCount int
}

var (
	// ErrStageDeadEnd is wrapped by ValidateGraph's Task 2.6.1c dead-end/trap
	// check: a non-terminal stage with zero enabled outgoing transitions.
	ErrStageDeadEnd = errors.New("non-terminal stage has no enabled outgoing transition")
	// ErrStageUnreachable is wrapped by ValidateGraph's Task 2.6.1b3
	// reachability check: a stage with no path from any IsEntry stage.
	ErrStageUnreachable = errors.New("stage is unreachable from any entry stage")
	// ErrDisableWouldStrandLiveItems is wrapped by
	// ValidateDisableTransition's Task 2.6.1e live-item-aware check.
	ErrDisableWouldStrandLiveItems = errors.New("disabling this transition would strand live items with no legal outgoing transition")
)

// buildAdjacency builds a directed adjacency map from stages to their
// enabled outgoing transitions (Task 2.6.1b1). Disabled edges are excluded —
// matching stageConfigCache's own "only enabled edges are legal" semantics
// (session/stage_config_cache.go) — so the reachability/dead-end checks
// below operate on the graph as it would actually behave at runtime, not the
// union of every row ever saved. Every known stage gets an entry (possibly
// nil) even with zero outgoing edges, so callers can distinguish "no edges"
// from "unknown stage."
func buildAdjacency(stages []StageDefinition, transitions []TransitionDefinition) map[string][]string {
	adjacency := make(map[string][]string, len(stages))
	for _, s := range stages {
		if _, ok := adjacency[s.Slug]; !ok {
			adjacency[s.Slug] = nil
		}
	}
	for _, t := range transitions {
		if !t.Enabled {
			continue
		}
		adjacency[t.FromSlug] = append(adjacency[t.FromSlug], t.ToSlug)
	}
	return adjacency
}

// reachableFrom runs a DFS from every entry stage, returning the set of
// stage slugs reachable from at least one of them (Task 2.6.1b2).
func reachableFrom(entryStages []string, adjacency map[string][]string) map[string]bool {
	visited := make(map[string]bool, len(adjacency))
	var stack []string
	for _, s := range entryStages {
		if !visited[s] {
			visited[s] = true
			stack = append(stack, s)
		}
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		for _, next := range adjacency[cur] {
			if !visited[next] {
				visited[next] = true
				stack = append(stack, next)
			}
		}
	}
	return visited
}

// ValidateGraph runs Story 2.6.1's DFS reachability + dead-end/trap checks,
// followed by Story 2.6.2's cycle-with-no-escape lint, over the full
// stage/transition graph. It returns the first hard validation error found
// (nil if the graph is valid), plus a slice of non-blocking warning strings
// — a gate-free cycle is a warning, never a rejection, since cycles are
// legitimate (e.g. review looping back to in_progress).
//
// Checks run in this fixed order: Task 2.6.1c's dead-end check first, then
// Task 2.6.1b3's reachability check, then Task 2.6.2a's cycle-escape lint.
// The first hard error short-circuits and returns immediately (warnings are
// only computed for an otherwise-valid graph); this mirrors ValidateGraph's
// single-error-return shape used by every other validator in this package
// (e.g. TransitionGuard).
func ValidateGraph(stages []StageDefinition, transitions []TransitionDefinition) ([]string, error) {
	adjacency := buildAdjacency(stages, transitions)

	// Task 2.6.1c: every non-terminal stage must have >=1 enabled outgoing
	// transition — a trap has no way out. This is checked independent of
	// reachability: a stage can be perfectly reachable and still be a dead
	// end once an item lands on it.
	for _, s := range stages {
		if s.IsTerminal {
			continue
		}
		if len(adjacency[s.Slug]) == 0 {
			return nil, fmt.Errorf("%w: stage %q has no enabled outgoing transition and is not marked terminal", ErrStageDeadEnd, s.Slug)
		}
	}

	// Task 2.6.1b2/b3: every stage must be reachable from at least one entry
	// stage.
	var entrySlugs []string
	for _, s := range stages {
		if s.IsEntry {
			entrySlugs = append(entrySlugs, s.Slug)
		}
	}
	reachable := reachableFrom(entrySlugs, adjacency)
	for _, s := range stages {
		if !reachable[s.Slug] {
			return nil, fmt.Errorf("%w: stage %q has no path from any entry stage", ErrStageUnreachable, s.Slug)
		}
	}

	// Task 2.6.2a: cycle-with-no-escape lint — soft warning, never blocks.
	warnings := detectUngatedCycles(stages, transitions, adjacency)
	return warnings, nil
}

// ValidateDisableTransition implements Task 2.6.1e's live-item-aware disable
// check — a distinct rule from ValidateGraph's Task 2.6.1c dead-end check,
// which only runs at stage/transition *creation* time, when nothing yet
// depends on the stage. This check instead runs whenever an existing edge is
// about to be disabled, when live items may already be sitting on
// fromStage: it must not be allowed to strand them with zero legal outgoing
// transitions.
//
// candidateEdges must reflect the graph as it would exist AFTER the proposed
// disable (i.e. the edge under consideration already has Enabled: false) —
// this function only counts remaining enabled outgoing edges from fromStage,
// it does not flip anything itself. liveItemCountByStage is supplied by the
// caller (Epic 2.7's RPC handler, which has DB access via the storage
// layer's live-item-count-per-stage query) — this package makes no storage
// calls of its own, keeping it pure/testable per the Pattern Decisions
// table.
func ValidateDisableTransition(candidateEdges []TransitionDefinition, fromStage string, liveItemCountByStage map[string]int) error {
	liveCount := liveItemCountByStage[fromStage]
	if liveCount == 0 {
		return nil
	}

	remaining := 0
	for _, t := range candidateEdges {
		if t.FromSlug == fromStage && t.Enabled {
			remaining++
		}
	}
	if remaining == 0 {
		return fmt.Errorf("%w: disabling this transition would leave %d live item(s) on %q with no legal outgoing transition", ErrDisableWouldStrandLiveItems, liveCount, fromStage)
	}
	return nil
}

// detectUngatedCycles finds simple cycles reachable via DFS over the
// enabled-edge adjacency and returns one warning string per cycle none of
// whose edges carries a gate (Story 2.6.2's "cycle with no escape" lint). A
// cycle with at least one gated edge is a legitimate looping workflow (e.g.
// review -> in_progress) and produces no warning.
//
// This uses standard white/gray/black DFS cycle detection, which finds every
// cycle reachable from the traversal order given — sufficient for this
// lint's soft-warning purpose (operator-authored graphs of at most dozens of
// nodes, per the Pattern Decisions table) without needing a full
// enumerate-every-simple-cycle algorithm (e.g. Johnson's).
func detectUngatedCycles(stages []StageDefinition, transitions []TransitionDefinition, adjacency map[string][]string) []string {
	gateCounts := make(map[[2]string]int, len(transitions))
	for _, t := range transitions {
		if t.Enabled {
			gateCounts[[2]string{t.FromSlug, t.ToSlug}] = t.GateCount
		}
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(stages))
	seenCycle := make(map[string]bool)
	var path []string
	var warnings []string

	var visit func(node string)
	visit = func(node string) {
		color[node] = gray
		path = append(path, node)
		for _, next := range adjacency[node] {
			switch color[next] {
			case white:
				visit(next)
			case gray:
				idx := indexOfSlug(path, next)
				if idx < 0 {
					continue
				}
				cycle := append([]string{}, path[idx:]...)
				sig := cycleSignature(cycle)
				if seenCycle[sig] {
					continue
				}
				seenCycle[sig] = true
				if !cycleHasGate(cycle, gateCounts) {
					warnings = append(warnings, fmt.Sprintf(
						"cycle with no gate on any edge: %s", strings.Join(append(append([]string{}, cycle...), cycle[0]), " -> ")))
				}
			case black:
				// Already fully explored from here; no new cycle to find.
			}
		}
		path = path[:len(path)-1]
		color[node] = black
	}

	for _, s := range stages {
		if color[s.Slug] == white {
			visit(s.Slug)
		}
	}
	return warnings
}

// indexOfSlug returns the index of node in path, or -1 if absent.
func indexOfSlug(path []string, node string) int {
	for i, p := range path {
		if p == node {
			return i
		}
	}
	return -1
}

// cycleHasGate reports whether any edge along cycle (wrapping from the last
// node back to the first) has at least one gate attached.
func cycleHasGate(cycle []string, gateCounts map[[2]string]int) bool {
	for i := range cycle {
		from := cycle[i]
		to := cycle[(i+1)%len(cycle)]
		if gateCounts[[2]string{from, to}] > 0 {
			return true
		}
	}
	return false
}

// cycleSignature returns a canonical, rotation-independent signature for
// cycle so the same cycle discovered from different DFS starting nodes is
// only reported once.
func cycleSignature(cycle []string) string {
	minIdx := 0
	for i, s := range cycle {
		if s < cycle[minIdx] {
			minIdx = i
		}
	}
	rotated := append(append([]string{}, cycle[minIdx:]...), cycle[:minIdx]...)
	return strings.Join(rotated, ">")
}
