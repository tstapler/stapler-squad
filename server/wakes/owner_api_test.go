package wakes

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestCancel_should_CancelOwnPendingWake_And_RefuseAnotherOwnersOrUnknown(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	ctx := context.Background()
	at := t0.Add(time.Hour)
	row := newRow(t, r.repo, rowOpts{owner: "owner-1", spec: domain.AtSpec{At: at}, nextFire: &at})

	_, _, err := r.d.Cancel(ctx, "owner-2", row.ID)
	require.ErrorIs(t, err, ErrWakeNotOwner)
	assert.Equal(t, domain.WakeStatusPending, r.status(row.ID), "a foreign cancel changes nothing")
	_, _, err = r.d.Cancel(ctx, "owner-1", uuid.NewString())
	require.ErrorIs(t, err, ErrWakeNotFound)
	_, _, err = r.d.Cancel(ctx, "owner-1", "not-a-uuid")
	require.ErrorIs(t, err, ErrWakeNotFound)

	got, cancelled, err := r.d.Cancel(ctx, "owner-1", row.ID)
	require.NoError(t, err)
	assert.True(t, cancelled)
	assert.Equal(t, domain.WakeStatusCancelled, got.Status)
	assert.EqualValues(t, 1, r.d.Metrics().Value(MetricTrigger, "at", "cancelled"))

	_, again, err := r.d.Cancel(ctx, "owner-1", row.ID)
	require.NoError(t, err)
	assert.False(t, again, "an ended wake reports cancelled=false, not an error")
}

func TestNextGeneration_should_ChainOnlyFromAWakeDeliveredWithinTheWindow(t *testing.T) {
	t.Parallel()
	r := newDispatcherRig(t)
	ctx := context.Background()

	gen, err := r.d.NextGeneration(ctx, "owner-1")
	require.NoError(t, err)
	assert.Zero(t, gen, "no wake delivered yet")

	r.start(r.d)
	due := t0
	row := newRow(t, r.repo, rowOpts{owner: "owner-1", generation: 2, spec: domain.AtSpec{At: due}, nextFire: &due})
	r.eventually(func() bool { return r.status(row.ID) == domain.WakeStatusDelivered })

	gen, err = r.d.NextGeneration(ctx, "owner-1")
	require.NoError(t, err)
	assert.Equal(t, 3, gen, "the woken turn's next wake is one generation deeper")

	r.clock.Advance(chainWindow + time.Minute)
	gen, err = r.d.NextGeneration(ctx, "owner-1")
	require.NoError(t, err)
	assert.Zero(t, gen, "outside the window it is a fresh chain")
}
