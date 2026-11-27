package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/server/services/approvals"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// The tests below read the process-wide ManualReader, so they stay serial: Go
// runs a package's serial tests before resuming any t.Parallel test.

func TestScrape_ShouldSampleWakeFiresAndCountScheduleWakeup_WhenSessionEndsAfterThreeWakesAndTwoReceipts(t *testing.T) {
	m := wakes.NewMetrics()
	beforeN, beforeSum := histogramTotals(t, wakes.HistogramFiresPerSession)
	beforeCalls := counterValue(t, questionkit.MetricScheduleWakeupCalls, map[string]string{"role": "unlinked"})

	for range 3 {
		m.WakeDelivered("sess-w")
	}
	svc := &SessionService{}
	svc.ObservePostToolUse(context.Background(), "sess-w", questionkit.ScheduleWakeupTool)
	svc.ObservePostToolUse(context.Background(), "sess-w", questionkit.ScheduleWakeupTool)
	svc.ObservePostToolUse(context.Background(), "sess-w", "Bash")
	m.SessionEnded("sess-w")

	n, sum := histogramTotals(t, wakes.HistogramFiresPerSession)
	assert.EqualValues(t, 1, n-beforeN)
	assert.InDelta(t, 3, sum-beforeSum, 1e-9)
	assert.EqualValues(t, 2, counterValue(t, questionkit.MetricScheduleWakeupCalls, map[string]string{"role": "unlinked"})-beforeCalls)
}

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

type fakeRoleLookup struct {
	row backlogdto.ItemSessionSummary
	err error
}

func (f fakeRoleLookup) GetItemSessionBySessionUUID(context.Context, string) (backlogdto.ItemSessionSummary, error) {
	return f.row, f.err
}

func TestScheduleWakeupRole_ShouldNameTheBacklogRole_WhenSessionIsLinked(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "work", questionkit.ScheduleWakeupRoles{Lookup: fakeRoleLookup{row: backlogdto.ItemSessionSummary{Role: "work"}}}.RoleOf(ctx, "s"))
	assert.Equal(t, questionkit.RoleUnlinked, questionkit.ScheduleWakeupRoles{Lookup: fakeRoleLookup{err: domain.ErrNotFound}}.RoleOf(ctx, "s"))
	assert.Equal(t, questionkit.RoleUnknown, questionkit.ScheduleWakeupRoles{Lookup: fakeRoleLookup{err: assert.AnError}}.RoleOf(ctx, "s"))
}

func TestInjectedPostToolUseGroup_ShouldHaveNoMatcher_SoScheduleWakeupReceiptsArrive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, approvals.InjectHooksConfig(dir, "sess", []approvals.HookName{approvals.HookPostToolLogging}))
	var top struct {
		Hooks map[string][]approvals.HookMatcherGroup `json:"hooks"`
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
	d := guidance.NewGuidanceAckDetector(nil, nil, nil, nil)
	before := counterValue(t, questionkit.MetricGuidanceUnconfirmed, nil)
	d.Sent(questionkit.SentDelivery{RowID: "r1"})
	d.Sent(questionkit.SentDelivery{RowID: "r2", Indeterminate: true})
	assert.EqualValues(t, 1, counterValue(t, questionkit.MetricGuidanceUnconfirmed, nil)-before)
}
