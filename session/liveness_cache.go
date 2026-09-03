package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
)

// resolvedLivenessDefinition is an unexported, immutable snapshot of one
// enabled LivenessDefinition row, held inside livenessCache. LivenessDefinition
// itself is already a plain value type (no pointers/slices to defensively
// copy), but this is kept as its own named type — mirroring
// resolvedPipelineMode's pattern (session/pipeline_engine.go) — so callers
// never confuse a stored ent row with a resolved cache value.
type resolvedLivenessDefinition struct {
	LivenessDefinition
}

// livenessCacheKey builds the map key livenessCache is keyed by:
// stage_slug + "\x00" + mode. "\x00" is used as the separator (rather than a
// printable character like ":") because it can never appear in either a
// stage slug or a PipelineMode slug, so no legitimate combination of the two
// can collide with a different pair.
func livenessCacheKey(stageSlug string, mode PipelineMode) string {
	return stageSlug + "\x00" + string(mode)
}

// livenessCache is an in-process, copy-on-write cache of enabled
// LivenessDefinition rows keyed by (stage_slug, pipeline_mode). Structurally
// copied near-verbatim from pipelineModeCache (session/pipeline_engine.go) —
// see that type's doc comment for the full concurrency rationale (Get is
// lock-free; Load/Invalidate share one writer-serialized refresh sequence
// that holds writeMu across the DB read itself, not just the Store).
//
// Get is additionally the SINGLE OWNER of the (stage, mode) -> (stage, nil)
// sparse-override fallback in this project (Story 1.3.1's corrected AC) —
// EntLivenessRepository.GetByStageAndMode performs only an exact-match query.
type livenessCache struct {
	ptr     atomic.Pointer[map[string]resolvedLivenessDefinition]
	writeMu sync.Mutex
}

// Load populates the cache from repo.ListAll, keeping only enabled rows.
// Shares its implementation (and writeMu serialization) with Invalidate.
func (c *livenessCache) Load(ctx context.Context, repo LivenessRepository) error {
	return c.refresh(ctx, repo)
}

// Invalidate re-fetches rows from repo and swaps the cache wholesale.
// Exposed as a distinct method name from Load for call-site clarity at the
// CRUD RPC write handlers (Story 1.3.2) — the implementation is identical to
// Load beyond both sharing the writeMu-guarded refresh sequence.
func (c *livenessCache) Invalidate(ctx context.Context, repo LivenessRepository) error {
	return c.refresh(ctx, repo)
}

// refresh is the shared, writer-serialized read-then-store sequence used by
// both Load and Invalidate. writeMu is held for the full sequence, including
// the DB read — see pipelineModeCache.refresh's doc comment for why.
//
// A row with an unrecognized Kind or a malformed field/Kind pairing is
// skipped with a Warn log rather than aborting the whole refresh — one
// corrupt row must never take down liveness resolution for every other
// stage/mode pair.
func (c *livenessCache) refresh(ctx context.Context, repo LivenessRepository) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	rows, err := repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("livenessCache: ListAll: %w", err)
	}

	next := make(map[string]resolvedLivenessDefinition, len(rows))
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		def, err := livenessDefinitionFromRecord(row)
		if err != nil {
			log.WarningLog().Printf("[LivenessEngine] skipping malformed stage_liveness_definitions row id=%s: %v", row.ID, err)
			continue
		}
		mode := PipelineModeDefault
		if row.PipelineMode != nil {
			mode = PipelineMode(*row.PipelineMode)
		}
		next[livenessCacheKey(row.StageSlug, mode)] = resolvedLivenessDefinition{LivenessDefinition: def}
	}

	c.ptr.Store(&next)
	log.DebugLog().Printf("[LivenessEngine] cache refreshed: %d enabled liveness definitions", len(next))
	return nil
}

// Get returns the resolved definition for (stageSlug, mode), applying the
// (stage, mode) -> (stage, nil) mode-less fallback described in this type's
// doc comment. Lock-free: at most two atomic-Load-backed map lookups, never
// touching writeMu — including the miss case (nil cache, i.e. never loaded).
// Never logs; the caller (CachingLivenessEngine.LivenessFor) is responsible
// for any Warn-log-and-fallback-to-DefaultLivenessEngine behavior when Get
// itself returns a miss.
func (c *livenessCache) Get(stageSlug string, mode PipelineMode) (resolvedLivenessDefinition, bool) {
	m := c.ptr.Load()
	if m == nil {
		return resolvedLivenessDefinition{}, false
	}

	if mode != PipelineModeDefault {
		if rd, ok := (*m)[livenessCacheKey(stageSlug, mode)]; ok {
			return rd, true
		}
	}

	// Mode-less fallback: a (stageSlug, mode) row is absent (or mode was
	// already PipelineModeDefault, making this the same lookup) — try
	// (stageSlug, nil) before reporting a miss.
	rd, ok := (*m)[livenessCacheKey(stageSlug, PipelineModeDefault)]
	return rd, ok
}
