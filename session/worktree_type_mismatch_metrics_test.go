package session

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// attributeKeyResolvedSessionType is the attribute key
// session_creation_worktree_type_mismatch_total's data points are tagged
// with (session/worktree_type_mismatch_metrics.go).
const attributeKeyResolvedSessionType = attribute.Key("resolved_session_type")

// collectWorktreeMismatchMetric returns
// session_creation_worktree_type_mismatch_total's current collected data, or
// nil if it has no recorded data points yet. Reuses testPiMetricReader (the
// single process-lifetime manual reader installed by
// pi_status_source_metrics_test.go's init() — only the first
// otel.SetMeterProvider call in this test binary wins, per that file's own
// doc comment), rather than installing a second, competing one.
func collectWorktreeMismatchMetric(t *testing.T) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testPiMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == "session_creation_worktree_type_mismatch_total" {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumForResolvedSessionType sums the counter's data points whose
// "resolved_session_type" attribute equals resolvedSessionType.
func sumForResolvedSessionType(t *testing.T, m *metricdata.Metrics, resolvedSessionType string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		return 0
	}
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyResolvedSessionType); ok && v.AsString() == resolvedSessionType {
			total += dp.Value
		}
	}
	return total
}

// TestStartLocked_should_RecordInvariantMetricAndWarn_When_NewWorktreeRequestResolvesToDirectory
// is worktree-envvars-hijack Epic 1.5's regression test (pre-mortem P1
// remediation): research/architecture.md's Q1/Q2 re-verification establishes
// this mismatch should be structurally impossible in production, but the
// check itself -- session_creation_worktree_type_mismatch_total incrementing
// and a Warn log firing -- had no test forcing the condition to prove it
// actually fires rather than being silently dead code. Forces the mismatch
// directly via RequestedNewWorktree=true + SessionType=SessionTypeDirectory
// (a value NewInstance would never normally combine with
// RequestedNewWorktree=true, but is directly constructible for this test),
// which is exactly what Story 1.5.1's AC describes as the unanticipated
// "SessionType flipped upstream" class this check exists to catch.
func TestStartLocked_should_RecordInvariantMetricAndWarn_When_NewWorktreeRequestResolvesToDirectory(t *testing.T) {
	before := collectWorktreeMismatchMetric(t)
	baseline := sumForResolvedSessionType(t, before, string(SessionTypeDirectory))

	handler := installRecordingHandler(t)
	repoDir := t.TempDir()
	now := time.Now().UnixNano()
	title := fmt.Sprintf("worktree-type-mismatch-%d", now)
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:                title,
		Path:                 repoDir,
		Program:              "sh",
		SessionType:          SessionTypeDirectory,
		RequestedNewWorktree: true, // the mismatch: the original request asked for SESSION_TYPE_NEW_WORKTREE
		TmuxPrefix:           fmt.Sprintf("test_worktreemismatch_%d_", now),
		TmuxServerSocket:     coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() { _ = cleanup() }()

	_ = inst.Start(true) // the invariant check runs before firstTimeSetup's worktree work; the eventual Start outcome is irrelevant here

	after := collectWorktreeMismatchMetric(t)
	got := sumForResolvedSessionType(t, after, string(SessionTypeDirectory))
	require.Greater(t, got, baseline, "session_creation_worktree_type_mismatch_total must increment for the resolved SessionTypeDirectory label")

	warnLines := handler.recordsAtLevel(slog.LevelWarn)
	var found bool
	for _, rec := range warnLines {
		if rec.msg == "session_creation_worktree_type_mismatch: SESSION_TYPE_NEW_WORKTREE request resolved to a non-NewWorktree SessionType" &&
			rec.attrs["uuid"] == inst.UUID {
			found = true
			break
		}
	}
	require.True(t, found, "a Warn-level log line naming the instance's UUID must fire for the unanticipated mismatch, never silently; got: %+v", warnLines)
}
