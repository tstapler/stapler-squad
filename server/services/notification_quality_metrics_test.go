package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/analytics"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// The tests below read the process-wide ManualReader, so they stay serial: Go
// runs a package's serial tests before resuming any t.Parallel test.

func counterValue(t *testing.T, name string, want map[string]string) int64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var total int64
	for _, dp := range sum.DataPoints {
		match := true
		for k, v := range want {
			if got, ok := dp.Attributes.Value(attributeKey(k)); !ok || got.AsString() != v {
				match = false
			}
		}
		if match {
			total += dp.Value
		}
	}
	return total
}

func histogramTotals(t *testing.T, name string) (count uint64, sum float64) {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0, 0
	}
	h, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	for _, dp := range h.DataPoints {
		count += dp.Count
		sum += dp.Sum
	}
	return count, sum
}

func attributeKey(k string) attribute.Key { return attribute.Key(k) }

type nopAnalytics struct{}

func (nopAnalytics) Record(context.Context, analytics.Event) error { return nil }

func TestScrape_ShouldCountApplyAndStale_WhenApproveThenStaleAnswer(t *testing.T) {
	f := newDispatchFixture()
	f.d.metrics = defaultActionMetrics()
	web := map[string][]string{actionClientHeader: {actionClientWebUI}}
	approved := map[string]string{"kind": "approve", "tier": "a", "outcome": "applied", "channel": channelWebLocal}
	stale := map[string]string{"kind": "answer_question", "outcome": "stale", "channel": channelWebLocal}
	const total = "notification_inline_action_total"
	beforeApproved, beforeStale := counterValue(t, total, approved), counterValue(t, total, stale)
	beforeN, _ := histogramTotals(t, "notification_inline_action_duration_seconds")

	f.invoke(t, localCtx(t), approveReq("k-a"), web)
	f.questions.known = false
	f.invoke(t, localCtx(t), answerReq("k-q"), web)

	assert.EqualValues(t, 1, counterValue(t, total, approved)-beforeApproved)
	assert.EqualValues(t, 1, counterValue(t, total, stale)-beforeStale)
	n, _ := histogramTotals(t, "notification_inline_action_duration_seconds")
	assert.EqualValues(t, 2, n-beforeN)
}

func TestScrape_ShouldYieldDeliveryFailureRateOfOneTenth_WhenTenAnswersOneNotSentOneUndoOneReopen(t *testing.T) {
	f := newDispatchFixture()
	f.d.metrics = defaultActionMetrics()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	tracker := newReopenTracker(func() time.Time { return now })
	f.d.reopen = tracker
	web := map[string][]string{actionClientHeader: {actionClientWebUI}}
	answers := map[string]string{"kind": "answer_question", "channel": channelWebLocal}
	failures := []string{"not_sent", "send_indeterminate", "stale"}
	failing := func() (n int64) {
		for _, o := range failures {
			n += counterValue(t, "notification_inline_action_total", map[string]string{"kind": "answer_question", "outcome": o, "channel": channelWebLocal})
		}
		return n
	}
	beforeAll, beforeFail := counterValue(t, "notification_inline_action_total", answers), failing()
	beforeUndo := counterValue(t, "inline_action_undo_total", map[string]string{"kind": "approve"})
	beforeReopen := counterValue(t, "reopen_after_action_total", map[string]string{"kind": "answer_question"})

	for i := range 10 {
		req := answerReq(string(rune('a' + i)))
		req.SessionId = "sess-" + string(rune('a'+i))
		if i == 4 {
			f.questions.result = outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, "")
		} else {
			f.questions.result = ActionResult{}
		}
		f.invoke(t, localCtx(t), req, web)
	}
	require.NoError(t, analytics.WithQualityMetrics(nopAnalytics{}).Record(context.Background(),
		analytics.Event{EventName: analytics.EventInlineActionUndo, Labels: map[string]string{"kind": "approve"}}))
	now = now.Add(45 * time.Second)
	tracker.attached("sess-b-title", "sess-b") // opened inside 60 s of its answer
	tracker.attached("sess-e")                 // its answer was NOT_SENT: nothing pending
	now = now.Add(time.Hour)
	tracker.attached("sess-c") // opened after the window

	attempted, failed := counterValue(t, "notification_inline_action_total", answers)-beforeAll, failing()-beforeFail
	assert.EqualValues(t, 10, attempted)
	assert.InDelta(t, 0.1, float64(failed)/float64(attempted), 1e-9)
	assert.EqualValues(t, 1, counterValue(t, "inline_action_undo_total", map[string]string{"kind": "approve"})-beforeUndo)
	assert.EqualValues(t, 1, counterValue(t, "reopen_after_action_total", map[string]string{"kind": "answer_question"})-beforeReopen)
}

func TestScrape_ShouldSampleWakeFiresAndCountScheduleWakeup_WhenSessionEndsAfterThreeWakesAndTwoReceipts(t *testing.T) {
	m := wakes.NewMetrics()
	beforeN, beforeSum := histogramTotals(t, wakes.HistogramFiresPerSession)
	beforeCalls := counterValue(t, metricScheduleWakeupCalls, map[string]string{"role": "unlinked"})

	for range 3 {
		m.WakeDelivered("sess-w")
	}
	svc := &SessionService{}
	svc.ObservePostToolUse(context.Background(), "sess-w", scheduleWakeupTool)
	svc.ObservePostToolUse(context.Background(), "sess-w", scheduleWakeupTool)
	svc.ObservePostToolUse(context.Background(), "sess-w", "Bash")
	m.SessionEnded("sess-w")

	n, sum := histogramTotals(t, wakes.HistogramFiresPerSession)
	assert.EqualValues(t, 1, n-beforeN)
	assert.InDelta(t, 3, sum-beforeSum, 1e-9)
	assert.EqualValues(t, 2, counterValue(t, metricScheduleWakeupCalls, map[string]string{"role": "unlinked"})-beforeCalls)
}

type fakeRoleLookup struct {
	row session.ItemSessionSummary
	err error
}

func (f fakeRoleLookup) GetItemSessionBySessionUUID(context.Context, string) (session.ItemSessionSummary, error) {
	return f.row, f.err
}

func TestScheduleWakeupRole_ShouldNameTheBacklogRole_WhenSessionIsLinked(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "work", scheduleWakeupRoles{lookup: fakeRoleLookup{row: session.ItemSessionSummary{Role: "work"}}}.roleOf(ctx, "s"))
	assert.Equal(t, roleUnlinked, scheduleWakeupRoles{lookup: fakeRoleLookup{err: session.ErrNotFound}}.roleOf(ctx, "s"))
	assert.Equal(t, roleUnknown, scheduleWakeupRoles{lookup: fakeRoleLookup{err: assert.AnError}}.roleOf(ctx, "s"))
}

func TestInjectedPostToolUseGroup_ShouldHaveNoMatcher_SoScheduleWakeupReceiptsArrive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InjectHooksConfig(dir, "sess", []HookName{HookPostToolLogging}))
	var top struct {
		Hooks map[string][]hookMatcherGroup `json:"hooks"`
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.local.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &top))
	require.NotEmpty(t, top.Hooks["PostToolUse"])
	for _, g := range top.Hooks["PostToolUse"] {
		assert.Empty(t, g.Matcher, "an empty matcher runs the hook for every tool, ScheduleWakeup included")
	}
}

func TestScrape_ShouldCountUnconfirmed_WhenWriteIsIndeterminate(t *testing.T) {
	d := NewGuidanceAckDetector(nil, nil, nil, nil)
	before := counterValue(t, metricGuidanceUnconfirmed, nil)
	d.Sent(SentDelivery{RowID: "r1"})
	d.Sent(SentDelivery{RowID: "r2", Indeterminate: true})
	assert.EqualValues(t, 1, counterValue(t, metricGuidanceUnconfirmed, nil)-before)
}
