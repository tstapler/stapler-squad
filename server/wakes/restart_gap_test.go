package wakes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func (p *paneDouble) setBusy(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.busy = v
}

// ADR-006 rules 1 and 2: a claim is not a delivery. A process killed after the
// claim but before the pane write leaves a claimed row the next boot re-queues.
func TestRestartGap_should_deliverOnce_When_killedAfterClaimBeforePaneWrite(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	ctx := context.Background()

	first := h.boot()
	row := newRow(t, first.repo, rowOpts{})
	h.steerer.pane.setBusy(true) // the pane write cannot happen before the kill
	first.d.timerClaim(ctx, row)
	require.Equal(t, domain.WakeStatusClaimed, first.status(t, row.ID))
	require.Empty(t, h.steerer.pane.written())
	first.kill()

	h.steerer.pane.setBusy(false)
	h.clock.Advance(3 * time.Minute) // past the claim lease
	second := h.boot()
	h.start(second)

	assert.Equal(t, domain.WakeStatusDelivered, second.status(t, row.ID))
	assert.Len(t, h.steerer.pane.written(), 1)
	assert.EqualValues(t, 1, second.d.Metrics().Value(MetricReconcile, ReconcileRequeuedClaimed))
	assert.EqualValues(t, 0, second.d.Metrics().Value(MetricDuplicate), "nothing had been written, so this is not a re-send")
}

func TestRestartGap_should_notResend_When_rowAlreadyDelivered(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	ctx := context.Background()

	first := h.boot()
	row := newRow(t, first.repo, rowOpts{})
	first.d.timerClaim(ctx, row)
	require.Equal(t, domain.WakeStatusDelivered, first.status(t, row.ID))
	require.Len(t, h.steerer.pane.written(), 1)
	first.kill()

	for range 2 {
		h.clock.Advance(10 * time.Minute)
		p := h.boot()
		h.start(p)
		p.d.Reconcile(ctx)
		p.kill()
	}

	assert.Len(t, h.steerer.pane.written(), 1, "a delivered row is never sent again")
}

// A busy owner at restart keeps the claim held; when it idles the wake is
// delivered once, however many retries ran meanwhile.
func TestRestartGap_should_deliverOnceOnIdle_When_busyOwnerAtRestart(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	ctx := context.Background()

	first := h.boot()
	row := claimedRow(t, first.repo, t0, rowOpts{})
	first.kill()

	h.steerer.setReady("owner-1", false)
	h.clock.Advance(3 * time.Minute)
	second := h.boot()
	h.start(second)
	require.Equal(t, domain.WakeStatusClaimed, second.status(t, row.ID), "held while the owner is busy")
	require.Empty(t, h.steerer.pane.written())
	require.True(t, second.d.Delivery().Holds(row.ID))

	second.d.Delivery().AttemptAll(ctx)
	require.Empty(t, h.steerer.pane.written(), "still busy: nothing written")

	h.steerer.setReady("owner-1", true)
	second.d.Delivery().AttemptAll(ctx)
	second.d.Delivery().AttemptAll(ctx)

	assert.Equal(t, domain.WakeStatusDelivered, second.status(t, row.ID))
	assert.Len(t, h.steerer.pane.written(), 1)
}
