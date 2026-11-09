package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type namedSession string

func (n namedSession) GetSessionIdentifier() string { return string(n) }

// Mutates package globals (threshold, reporter), so it is not parallel.
func TestTimeBackendOp_should_ReportOnlyCallsOverTheSlowThreshold(t *testing.T) {
	prevThreshold, prevReporter := slowCallThreshold, slowCallReporter
	t.Cleanup(func() { slowCallThreshold, slowCallReporter = prevThreshold, prevReporter })
	slowCallThreshold = 20 * time.Millisecond
	var got []slowCallReport
	slowCallReporter = func(_ context.Context, r slowCallReport) { got = append(got, r) }
	op := tymuxIOOps.capturePane.with(namedSession("sess-1"))

	_, err := timeBackendOp(op, func() (string, error) { return "fast", nil })
	require.NoError(t, err)
	assert.Empty(t, got, "a fast call must not be reported")

	_, err = timeBackendOp(op, func() (string, error) {
		time.Sleep(40 * time.Millisecond) //nolint:notimesleeptest the op must really exceed the slow threshold measured by wall clock
		return "slow", nil
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, opCapturePane, got[0].op)
	assert.Equal(t, backendLabelTymux, got[0].backend)
	assert.Equal(t, "sess-1", got[0].sessionID)
	assert.GreaterOrEqual(t, got[0].duration, 40*time.Millisecond)
}

func TestSlowCallSessionLabel_should_StayBoundedUnderManySessions(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := range 500 {
		seen[slowCallSessionLabel(string(rune('a'+i%26))+string(rune('a'+i/26%26))+"-session")] = true
	}
	assert.LessOrEqual(t, len(seen), 101, "distinct labels are capped at the lifetime cap plus other")
}
