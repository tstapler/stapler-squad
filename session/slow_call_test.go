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

func TestTimeBackendOp_should_ReportOnlyCallsAtOrOverTheSlowThreshold(t *testing.T) {
	t.Parallel()
	const threshold = 20 * time.Millisecond
	var got []slowCallReport
	report := func(_ context.Context, r slowCallReport) { got = append(got, r) }
	op := tymuxIOOps.capturePane.with(namedSession("sess-1"))
	start := time.Unix(100, 0)

	op.reportIfSlow(context.Background(), start, threshold-1, threshold, report)
	assert.Empty(t, got, "a call under the threshold must not be reported")

	op.reportIfSlow(context.Background(), start, 40*time.Millisecond, threshold, report)
	require.Len(t, got, 1)
	assert.Equal(t, opCapturePane, got[0].op)
	assert.Equal(t, backendLabelTymux, got[0].backend)
	assert.Equal(t, "sess-1", got[0].sessionID)
	assert.Equal(t, start, got[0].start)
	assert.Equal(t, 40*time.Millisecond, got[0].duration)

	op.reportIfSlow(context.Background(), start, threshold, threshold, report)
	assert.Len(t, got, 2, "a call exactly at the threshold is slow")
}

func TestSlowCallSessionLabel_should_StayBoundedUnderManySessions(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := range 500 {
		seen[slowCallSessionLabel(string(rune('a'+i%26))+string(rune('a'+i/26%26))+"-session")] = true
	}
	assert.LessOrEqual(t, len(seen), 101, "distinct labels are capped at the lifetime cap plus other")
}
