package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
)

// resolvedGate is a deep-copied-at-load-time snapshot of one enabled
// TransitionGate row, held inside a resolvedTransitionEdge. Mirrors
// resolvedPipelineMode's copy-on-load discipline (session/pipeline_engine.go)
// so concurrent readers never observe a partially-updated ent object during a
// cache swap.
type resolvedGate struct {
	ID       uuid.UUID
	Kind     GateKind
	Stateful bool
}

// resolvedTransitionEdge is a deep-copied-at-load-time snapshot of one legal,
// enabled (from,to) StageTransition edge plus its ordered gates, held inside
// stageConfigCache.
type resolvedTransitionEdge struct {
	FromSlug string
	ToSlug   string
	Gates    []resolvedGate
}

// stageConfigSnapshot is the immutable value swapped into stageConfigCache on
// every Load/Invalidate. edges[fromSlug][toSlug] holds the resolved edge iff
// that transition is legal and enabled, mirroring DefaultWorkflowEngine's
// transitions map shape (session/workflow_engine.go).
type stageConfigSnapshot struct {
	edges map[string]map[string]resolvedTransitionEdge
}

// stageConfigCache is an in-process, copy-on-write cache of the enabled
// stage/transition/gate graph — a near-verbatim copy of pipelineModeCache's
// structure (session/pipeline_engine.go), copied again per Task 2.3.1b rather
// than shared, matching this repo's existing precedent of one cache type per
// engine.
//
// Get/AllowedTransitions are lock-free: a single atomic Load + map lookup,
// and neither touches writeMu — so a reader is never blocked behind a
// concurrent writer.
//
// Load/Invalidate share one writer-serialized sequence (acquire writeMu → DB
// read → build new snapshot → atomic Store → release writeMu). Holding
// writeMu across the DB read itself (not just around the Store) prevents a
// slower concurrent caller's Store from landing after a faster, later-started
// caller's Store and silently reverting the cache to stale data — see
// pipelineModeCache's doc comment for the full rationale.
type stageConfigCache struct {
	ptr     atomic.Pointer[stageConfigSnapshot]
	writeMu sync.Mutex
}

// Load populates the cache from repo. Shares its implementation (and
// writeMu serialization) with Invalidate — see refresh.
func (c *stageConfigCache) Load(ctx context.Context, repo StageConfigRepository) error {
	return c.refresh(ctx, repo)
}

// Invalidate re-fetches the graph from repo and swaps the cache wholesale.
// Exposed as a distinct method name from Load for call-site clarity at the
// Epic 2.7 RPC write handlers that must invalidate the cache after every
// Create/Update/Delete of a stage, transition, or gate.
func (c *stageConfigCache) Invalidate(ctx context.Context, repo StageConfigRepository) error {
	return c.refresh(ctx, repo)
}

// refresh is the shared, writer-serialized read-then-store sequence used by
// both Load and Invalidate.
func (c *stageConfigCache) refresh(ctx context.Context, repo StageConfigRepository) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	stages, err := repo.ListEnabledStages(ctx)
	if err != nil {
		return fmt.Errorf("stageConfigCache: ListEnabledStages: %w", err)
	}
	transitions, err := repo.ListEnabledTransitions(ctx)
	if err != nil {
		return fmt.Errorf("stageConfigCache: ListEnabledTransitions: %w", err)
	}

	next := &stageConfigSnapshot{edges: make(map[string]map[string]resolvedTransitionEdge, len(transitions))}
	edgeCount := 0
	gateCount := 0
	for _, t := range transitions {
		from := t.Edges.FromStage
		to := t.Edges.ToStage
		if from == nil || to == nil || !from.Enabled || !to.Enabled {
			// Defensive: skip an edge whose endpoint stage is missing or
			// disabled — WithFromStage/WithToStage always populate the edge
			// for a well-formed FK, and ListEnabledTransitions only loads
			// enabled StageTransition rows, but an endpoint stage can still
			// be independently disabled without disabling the edge itself.
			continue
		}

		gates := make([]resolvedGate, 0, len(t.Edges.Gates))
		for _, g := range t.Edges.Gates {
			gates = append(gates, resolvedGate{
				ID:       g.ID,
				Kind:     GateKind(g.Kind),
				Stateful: g.Stateful,
			})
			gateCount++
		}

		if next.edges[from.Slug] == nil {
			next.edges[from.Slug] = make(map[string]resolvedTransitionEdge)
		}
		next.edges[from.Slug][to.Slug] = resolvedTransitionEdge{
			FromSlug: from.Slug,
			ToSlug:   to.Slug,
			Gates:    gates,
		}
		edgeCount++
	}

	c.ptr.Store(next)
	log.DebugLog().Printf("[ConfiguredWorkflowEngine] cache refreshed: %d stages, %d transitions, %d gates", len(stages), edgeCount, gateCount)
	return nil
}

// Get returns the resolved edge for (from,to), or (resolvedTransitionEdge{},
// false) if no such legal/enabled edge is present in the current cache
// snapshot — including the case where the cache has never been loaded at
// all. Lock-free: a single atomic Load + two map lookups. Takes BacklogStatus
// (the widened open stage-slug type, per plan.md's Domain Glossary) rather
// than a bare string so from/to can't be transposed by mistake at call sites.
func (c *stageConfigCache) Get(from, to BacklogStatus) (resolvedTransitionEdge, bool) {
	snap := c.ptr.Load()
	if snap == nil {
		return resolvedTransitionEdge{}, false
	}
	targets, ok := snap.edges[string(from)]
	if !ok {
		return resolvedTransitionEdge{}, false
	}
	edge, ok := targets[string(to)]
	return edge, ok
}

// AllowedTransitions returns the stages reachable from from in the current
// cache snapshot, in no particular order (callers sort as needed).
func (c *stageConfigCache) AllowedTransitions(from BacklogStatus) []BacklogStatus {
	snap := c.ptr.Load()
	if snap == nil {
		return nil
	}
	targets, ok := snap.edges[string(from)]
	if !ok {
		return nil
	}
	result := make([]BacklogStatus, 0, len(targets))
	for to := range targets {
		result = append(result, BacklogStatus(to))
	}
	return result
}
