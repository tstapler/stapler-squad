//go:build !race

package services

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// Measures the handler's own CPU work with instant fakes; it never waits.
// Excluded from -race runs because the race detector inflates wall time.
func TestNudgeSessionForPR_should_FinishUnder100msAtP95_When_InstantFakes50Runs(t *testing.T) {
	const runs = 50
	const budget = 100 * time.Millisecond
	const steerCost = 7 * time.Millisecond // fake-clock time the fake steer "takes"

	durations := make([]time.Duration, 0, runs)
	var lastLatencyMS any
	for i := 0; i < runs; i++ {
		f := newNudgeFixture(t)
		f.nudger.onSteer = func() { f.clock.Advance(steerCost) }

		start := time.Now()
		resp, err := f.call("fix-ci")
		durations = append(durations, time.Since(start))

		require.NoError(t, err)
		require.Equal(t, sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, resp.Outcome)
		exit := f.logs.byMsg("nudge_outcome")
		require.Len(t, exit, 1)
		lastLatencyMS = exit[0].fields["latency_ms"]
	}

	sort.Slice(durations, func(a, b int) bool { return durations[a] < durations[b] })
	p95 := durations[(runs*95+99)/100-1]
	assert.Less(t, p95, budget, "p95 handler latency")
	assert.EqualValues(t, steerCost.Milliseconds(), lastLatencyMS, "latency_ms comes from the injected clock")
}
