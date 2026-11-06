package wakes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Story 5.1: a restart that misses a trigger raises a failure-class alert
// that names the trigger id, so the operator can find the row.
func TestRestart_should_referenceTheTriggerIDInTheFailureNotification_When_expiryPassedDuringDowntime(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	first := h.boot()
	due := t0.Add(10 * time.Minute)
	expires := t0.Add(30 * time.Minute)
	row := newRow(t, first.repo, rowOpts{spec: domain.AtSpec{At: due}, nextFire: &due, expiresAt: &expires})
	first.kill()

	h.clock.Advance(2 * time.Hour)
	second := h.boot()
	h.start(second)

	require.Len(t, h.notifier.all(), 1)
	n := h.notifier.all()[0]
	assert.Equal(t, NotifyFailure, n.Kind)
	assert.Contains(t, n.Message, row.ID)
}

func TestMetrics_should_sampleDeliveriesWhenSessionEnds_IncludingZero(t *testing.T) {
	t.Parallel()
	m := NewMetrics()
	for range 3 {
		m.WakeDelivered("s1")
	}
	m.SessionEnded("s1")
	m.SessionEnded("s2")
	m.SessionEnded("s1")
	assert.Equal(t, []float64{3, 0, 0}, m.FiresPerSession())
}
