package procbackend

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/slowops"
)

type namedSession string

func (n namedSession) GetSessionIdentifier() string { return string(n) }

func TestTimeBackendOp_should_ReportOnlyCallsAtOrOverTheSlowThreshold(t *testing.T) {
	t.Parallel()
	const threshold = 20 * time.Millisecond
	var got []slowops.CallReport
	report := func(_ context.Context, r slowops.CallReport) { got = append(got, r) }
	op := tymuxIOOps.capturePane.with(namedSession("sess-1"))
	start := time.Unix(100, 0)

	op.reportIfSlow(context.Background(), start, threshold-1, threshold, report)
	assert.Empty(t, got, "a call under the threshold must not be reported")

	op.reportIfSlow(context.Background(), start, 40*time.Millisecond, threshold, report)
	require.Len(t, got, 1)
	assert.Equal(t, opCapturePane, got[0].Op)
	assert.Equal(t, string(backendLabelTymux), got[0].Backend)
	assert.Equal(t, "sess-1", got[0].SessionID)
	assert.Equal(t, start, got[0].Start)
	assert.Equal(t, 40*time.Millisecond, got[0].Duration)

	op.reportIfSlow(context.Background(), start, threshold, threshold, report)
	assert.Len(t, got, 2, "a call exactly at the threshold is slow")
}
