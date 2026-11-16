package workflowengine

import (
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Seams for white-box tests in package session, which need its ent-backed
// repositories and so cannot sit in this package (session imports it).

// NewConfiguredWorkflowEngineEmptyCache returns an engine whose cache was never loaded.
func NewConfiguredWorkflowEngineEmptyCache() *ConfiguredWorkflowEngine {
	return &ConfiguredWorkflowEngine{cache: &stageConfigCache{}}
}

// CachedGateIDs returns the IDs of the gates cached on the from->to edge.
func (e *ConfiguredWorkflowEngine) CachedGateIDs(from, to domain.BacklogStatus) ([]uuid.UUID, bool) {
	edge, ok := e.cache.Get(from, to)
	if !ok {
		return nil, false
	}
	ids := make([]uuid.UUID, len(edge.Gates))
	for i, g := range edge.Gates {
		ids[i] = g.ID
	}
	return ids, true
}

// NewReplayGateOverlay wraps base so every read of the edge's transition carries one unsatisfied gate.
func NewReplayGateOverlay(base StageConfigRepository, edge domain.SystemEdge) StageConfigRepository {
	return replayGateOverlay{base, edge}
}
