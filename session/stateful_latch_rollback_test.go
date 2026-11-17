package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/gatefixturetest"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

const (
	latchACDone    = `[{"index":0,"text":"c","status":"done"}]`
	latchACPending = `[{"index":0,"text":"c","status":"pending"}]`
)

func TestRollback_ClearsStatefulLatchOfUndoneEdge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newGateHarness(t, workflowengine.ModeEnforce)
	from, to := gatefixturetest.NewCustomTransitionWithGates(t, h.repo.Client, h.cfg.InvalidateCache, []domain.GateKind{domain.GateKindStructural}, []bool{true})
	item := h.newItem(t, from)
	satisfied := domain.BacklogItemTransitionInput{Status: from, ItemID: item.ID, AcCriteria: latchACDone}
	regressed := domain.BacklogItemTransitionInput{Status: from, ItemID: item.ID, AcCriteria: latchACPending}

	h.cfg.RecordLatchesAfterCommit(satisfied, to)
	require.NoError(t, h.cfg.ValidateGates(regressed, to, nil), "latched")
	h.moveRaw(t, item.ID, to)

	_, err := h.storage.Rollback(ctx, item.ID, from, &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(to)}, instance.TriggeredBySystem)
	require.NoError(t, err)
	require.Error(t, h.cfg.ValidateGates(regressed, to, nil), "a rolled-back transition must not leave the gate latched")
}
