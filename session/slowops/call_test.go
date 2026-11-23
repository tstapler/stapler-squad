package slowops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type namedSession string

func (n namedSession) GetSessionIdentifier() string { return string(n) }

func TestReportIfSlow_should_ReportOnlyCallsAtOrOverTheThreshold(t *testing.T) {
	t.Parallel()
	const threshold = 20 * time.Millisecond
	var got []CallReport
	report := func(_ context.Context, r CallReport) { got = append(got, r) }
	start := time.Unix(100, 0)

	ReportIfSlow(context.Background(), namedSession("sess-1"), "capture_pane", "tymux", start, threshold-1, threshold, report)
	assert.Empty(t, got, "a call under the threshold must not be reported")

	ReportIfSlow(context.Background(), namedSession("sess-1"), "capture_pane", "tymux", start, 40*time.Millisecond, threshold, report)
	require.Len(t, got, 1)
	assert.Equal(t, CallReport{Op: "capture_pane", Backend: "tymux", SessionID: "sess-1", Start: start, Duration: 40 * time.Millisecond}, got[0])

	ReportIfSlow(context.Background(), namedSession("sess-1"), "capture_pane", "tymux", start, threshold, threshold, report)
	assert.Len(t, got, 2, "a call exactly at the threshold is slow")
}

func TestReportIfSlow_should_ReportAnEmptySessionID_When_NoSessionIsBound(t *testing.T) {
	t.Parallel()
	var got []CallReport
	ReportIfSlow(context.Background(), nil, "resize", "tmux", time.Unix(1, 0), time.Second, time.Millisecond,
		func(_ context.Context, r CallReport) { got = append(got, r) })
	require.Len(t, got, 1)
	assert.Empty(t, got[0].SessionID)
}

func TestReportCall_should_NotPanic_When_TheSessionLabelIsNew(t *testing.T) {
	t.Parallel()
	ReportCall(context.Background(), CallReport{Op: "resize", Backend: "tmux", SessionID: "report-call-sess", Start: time.Now(), Duration: time.Second})
}

func TestCallThreshold_should_BePositive(t *testing.T) {
	t.Parallel()
	assert.Positive(t, CallThreshold())
}

func TestSlowCallSessionLabel_should_StayBoundedUnderManySessions(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := range 500 {
		seen[slowCallSessionLabel(string(rune('a'+i%26))+string(rune('a'+i/26%26))+"-session")] = true
	}
	assert.LessOrEqual(t, len(seen), 101, "distinct labels are capped at the lifetime cap plus other")
}
