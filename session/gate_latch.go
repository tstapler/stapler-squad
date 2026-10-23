package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
)

// gate_latch.go — a structural gate with stateful=true is one-shot: once a
// transition commits through it, its satisfaction is recorded as a
// GateSatisfactionRecord and later evaluations answer from that record
// without re-checking the item's state.

const latchedGateDescription = "previously satisfied (stateful gate latched)"

// evaluateStructuralOrLatched evaluates a structural gate fresh, except that an
// effectively-stateful one answers from its latch record when present.
func (e *ConfiguredWorkflowEngine) evaluateStructuralOrLatched(g resolvedGate, item BacklogItemTransitionInput) GateStatus {
	if g.effectiveStateful() && e.isLatched(g, item.ItemID) {
		return GateStatus{GateID: g.ID.String(), Kind: GateKindStructural, Satisfied: true, Description: latchedGateDescription}
	}
	return evaluateStructuralGate(g, item)
}

// isLatched reads the gate's record under the bounded lookup; any failure
// means "not latched", so the gate is evaluated fresh.
func (e *ConfiguredWorkflowEngine) isLatched(g resolvedGate, itemID string) bool {
	itemUUID, ok := e.latchKey(itemID)
	if !ok {
		return false
	}
	record, err := e.lookupGateRecord(itemUUID, g.ID)
	return err == nil && record.Satisfied
}

func (e *ConfiguredWorkflowEngine) latchKey(itemID string) (uuid.UUID, bool) {
	if e.gateSatisfactionRepo == nil || itemID == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(itemID)
	return id, err == nil
}

// RecordLatchesAfterCommit records satisfaction for each effectively-stateful
// structural gate on the item.Status->to edge that is satisfied on item. Call
// it only after the transition committed and outside any transaction. A failed
// write is logged and never undoes the committed transition; the gate then
// stays unlatched and is simply re-evaluated next time.
func (e *ConfiguredWorkflowEngine) RecordLatchesAfterCommit(item BacklogItemTransitionInput, to BacklogStatus) {
	itemUUID, ok := e.latchKey(item.ItemID)
	if !ok {
		return
	}
	edge, found := e.cache.Get(item.Status, to)
	if !found {
		return
	}
	for _, g := range edge.Gates {
		if g.Kind != GateKindStructural || !g.effectiveStateful() || !evaluateStructuralGate(g, item).Satisfied {
			continue
		}
		if err := e.recordLatch(itemUUID, g.ID); err != nil {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] latch write failed for item %s gate %s (transition already committed): %v", itemUUID, g.ID, err)
		}
	}
}

// gateLatchClearer is the optional repository capability ClearLatches needs; a
// repository without it leaves latches in place.
type gateLatchClearer interface {
	DeleteByItemAndGate(ctx context.Context, itemID, gateID uuid.UUID) error
}

// ClearLatches removes the latch of each effectively-stateful structural gate on
// the from->to edge, so an undone transition does not leave the gate answering
// "previously satisfied". A failure is logged; the stale latch then stays.
func (e *ConfiguredWorkflowEngine) ClearLatches(itemID string, from, to BacklogStatus) {
	itemUUID, ok := e.latchKey(itemID)
	clearer, canClear := e.gateSatisfactionRepo.(gateLatchClearer)
	if !ok || !canClear {
		return
	}
	edge, found := e.cache.Get(from, to)
	if !found {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.lookupTimeout())
	defer cancel()
	for _, g := range edge.Gates {
		if g.Kind != GateKindStructural || !g.effectiveStateful() {
			continue
		}
		if err := clearer.DeleteByItemAndGate(ctx, itemUUID, g.ID); err != nil {
			log.WarningLog().Printf("[ConfiguredWorkflowEngine] latch clear failed for item %s gate %s: %v", itemUUID, g.ID, err)
		}
	}
}

func (e *ConfiguredWorkflowEngine) lookupTimeout() time.Duration {
	if e.gateLookupTimeout > 0 {
		return e.gateLookupTimeout
	}
	return defaultGateLookupTimeout
}

// ClearLatches undoes RecordLatchesAfterCommit for the from->to edge; a no-op
// without a usable Configured engine.
func (s *SwitchableWorkflowEngine) ClearLatches(itemID string, from, to BacklogStatus) {
	if cfg, _ := s.enf.usableConfigured(); cfg != nil {
		cfg.ClearLatches(itemID, from, to)
	}
}

// recordLatch is idempotent: losing a create race to an existing record is
// success.
func (e *ConfiguredWorkflowEngine) recordLatch(itemID, gateID uuid.UUID) error {
	timeout := e.gateLookupTimeout
	if timeout <= 0 {
		timeout = defaultGateLookupTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	now := time.Now()
	_, err := e.gateSatisfactionRepo.Create(ctx, GateSatisfactionCreateInput{
		ItemID: itemID, GateID: gateID, Satisfied: true, SatisfiedBy: "transition", SatisfiedAt: &now,
	})
	if errors.Is(err, ErrConflict) {
		return nil
	}
	return err
}

// commitObserver is implemented by the class-aware view so the gate can latch
// one-shot gates after a transition commits.
type commitObserver interface {
	TransitionCommitted(item BacklogItemTransitionInput, to BacklogStatus)
}

// TransitionCommitted latches the edge's stateful structural gates, but only
// where the Configured gates actually decided: Enforce mode, usable config, and
// no operator override.
func (v engineView) TransitionCommitted(item BacklogItemTransitionInput, to BacklogStatus) {
	if v.mode() != ModeEnforce || v.pol.bypassesOperatorGates(item) {
		return
	}
	if cfg, _ := v.s.enf.usableConfigured(); cfg != nil {
		cfg.RecordLatchesAfterCommit(item, to)
	}
}

func notifyTransitionCommitted(view WorkflowEngine, item BacklogItemTransitionInput, to BacklogStatus) {
	if o, ok := view.(commitObserver); ok {
		o.TransitionCommitted(item, to)
	}
}
