package wakes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestExpireOverdue_should_MarkTimeAndEventRowsMissedOnce_When_DispatcherIsStopped(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	p := h.boot() // built, never started
	due := t0.Add(10 * time.Minute)
	soon, later := t0.Add(30*time.Minute), t0.Add(6*time.Hour)
	timeRow := newRow(t, p.repo, rowOpts{owner: "owner-1", payload: "timed", spec: domain.AtSpec{At: due}, nextFire: &due, expiresAt: &soon})
	eventRow := backlogRow(t, &dispatcherRig{repo: p.repo}, itemX, "verdict_recorded", rowOpts{owner: "owner-2", payload: "evented", expiresAt: &soon})
	liveRow := newRow(t, p.repo, rowOpts{owner: "owner-3", payload: "still valid", spec: domain.AtSpec{At: due}, nextFire: &due, expiresAt: &later})
	h.clock.Advance(time.Hour)

	n, err := ExpireOverdue(context.Background(), p.repo, h.notifier, h.clock.Now())

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, domain.WakeStatusMissed, p.status(t, timeRow.ID))
	assert.Equal(t, domain.WakeStatusMissed, p.status(t, eventRow.ID))
	assert.Equal(t, domain.WakeStatusPending, p.status(t, liveRow.ID))
	notes := h.notifier.all()
	require.Len(t, notes, 2)
	for _, note := range notes {
		assert.Equal(t, NotifyFailure, note.Kind)
	}
	assert.Contains(t, notes[0].Message+notes[1].Message, "timed")
	assert.Contains(t, notes[0].Message+notes[1].Message, "evented")

	again, err := ExpireOverdue(context.Background(), p.repo, h.notifier, h.clock.Now())
	require.NoError(t, err)
	assert.Zero(t, again, "a second sweep ends nothing")

	h.start(p)
	assert.Len(t, h.notifier.all(), 2, "the later start does not report the swept rows again")
}

func TestRestart_should_MarkExpiredEventRowMissedAndNotify_When_StartedAfterDowntimeWithoutSweep(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	first := h.boot()
	soon := t0.Add(30 * time.Minute)
	row := backlogRow(t, &dispatcherRig{repo: first.repo}, itemX, "verdict_recorded", rowOpts{owner: "owner-2", payload: "evented", expiresAt: &soon})
	first.kill()

	h.clock.Advance(2 * time.Hour)
	second := h.boot()
	h.start(second)

	assert.Equal(t, domain.WakeStatusMissed, second.status(t, row.ID))
	require.Len(t, h.notifier.all(), 1)
	assert.Equal(t, NotifyFailure, h.notifier.all()[0].Kind)
	assert.Contains(t, h.notifier.all()[0].Message, "owner-2")
}
