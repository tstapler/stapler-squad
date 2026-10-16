package session

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// findRecordByMessage returns the first record in records whose msg matches,
// or nil if none does.
func findRecordByMessage(records []capturedRecord, msg string) *capturedRecord {
	for i := range records {
		if records[i].msg == msg {
			return &records[i]
		}
	}
	return nil
}

// scrollForwardAttemptLabels bundles the three attributes
// scroll_forward_attempts_total is labeled by, so sumScrollForwardAttempts
// takes one value instead of a same-typed string pile.
type scrollForwardAttemptLabels struct {
	adapter, outcome, blockedReason string
}

func (want scrollForwardAttemptLabels) matches(attrs attribute.Set) bool {
	a, aok := attrs.Value(attribute.Key("adapter"))
	o, ook := attrs.Value(attribute.Key("outcome"))
	b, bok := attrs.Value(attribute.Key("blocked_reason"))
	return aok && a.AsString() == want.adapter &&
		ook && o.AsString() == want.outcome &&
		bok && b.AsString() == want.blockedReason
}

// sumScrollForwardAttempts sums scroll_forward_attempts_total's current data
// points matching want, via the package's shared testPiMetricReader
// (pi_status_source_metrics_test.go) -- the single real MeterProvider
// installed for this test binary.
func sumScrollForwardAttempts(t *testing.T, want scrollForwardAttemptLabels) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, testPiMetricReader.Collect(context.Background(), &rm))
	m := findMetricByName(rm, "scroll_forward_attempts_total")
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)

	var total int64
	for _, dp := range sum.DataPoints {
		if want.matches(dp.Attributes) {
			total += dp.Value
		}
	}
	return total
}

// findMetricByName returns the first metric in rm matching name, or nil.
func findMetricByName(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// TestForwardScroll_should_LogPathAndOutcome_When_AnyRequestCompletes is
// REQ-15's happy-path observability test (Epic 1.5's Observability Plan): a
// structured log line records which path served the request -- DELIVERED
// maps to "app_forwarded" -- on every ForwardScroll call, not just errors.
func TestForwardScroll_should_LogPathAndOutcome_When_AnyRequestCompletes(t *testing.T) {
	h := installRecordingHandler(t)
	fakePM := &fakeScrollForwardProcessManager{captureSeq: []string{"settled", "settled"}}
	inst := newForwardScrollTestInstance(t, fakePM)

	outcome, _, _, err := inst.ForwardScroll(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		t.Fatalf("outcome = %v, want DELIVERED", outcome)
	}

	found := findRecordByMessage(h.recordsAtLevel(slog.LevelInfo), "scroll_forward: request completed")
	if found == nil {
		t.Fatalf("expected a %q log.Info record", "scroll_forward: request completed")
	}
	if found.attrs["path"] != "app_forwarded" {
		t.Fatalf("path = %v, want app_forwarded", found.attrs["path"])
	}
	if found.attrs["outcome"] != "delivered" {
		t.Fatalf("outcome = %v, want delivered", found.attrs["outcome"])
	}
	if found.attrs["adapter"] != "claude" {
		t.Fatalf("adapter = %v, want claude", found.attrs["adapter"])
	}
}

// TestForwardScroll_should_LogCoverageGapWarningOnlyOnce_When_ProgramHasNoRegisteredScrollAdapter
// is REQ-15's coverage-gap test: a session whose program has no registered
// ScrollAdapter gets exactly one log.Warn, not one per scroll attempt.
func TestForwardScroll_should_LogCoverageGapWarningOnlyOnce_When_ProgramHasNoRegisteredScrollAdapter(t *testing.T) {
	h := installRecordingHandler(t)
	program := "bash-" + t.Name() // unique per test: scrollForwardCoverageGapLogged is a package-level, cross-test sync.Map
	inst := &Instance{Title: t.Name(), Program: program}

	for range 2 {
		outcome, _, _, err := inst.ForwardScroll(context.Background(), 1, nil)
		if err != nil {
			t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
		}
		if outcome != sessionv1.ScrollForwardOutcome_BLOCKED {
			t.Fatalf("outcome = %v, want BLOCKED (no capability)", outcome)
		}
	}

	const wantMsg = "scroll_forward: no ScrollAdapter registered for program, scroll-forwarding unavailable"
	var matches int
	for _, r := range h.recordsAtLevel(slog.LevelWarn) {
		if r.msg == wantMsg && r.attrs["program"] == program {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("got %d coverage-gap log.Warn records for program %q across 2 ForwardScroll calls, want exactly 1", matches, program)
	}
}

// TestScrollForwardAttemptsTotal_should_IncrementWithAdapterOutcomeAndBlockedReasonLabels_When_ForwardScrollCompletes
// verifies REQ-15's metric label set (adapter/outcome/blocked_reason) matches
// the Observability Plan's cardinality claim, including a BLOCKED outcome's
// blocked_reason label (as opposed to the "n/a" every other outcome gets).
func TestScrollForwardAttemptsTotal_should_IncrementWithAdapterOutcomeAndBlockedReasonLabels_When_ForwardScrollCompletes(t *testing.T) {
	labels := scrollForwardAttemptLabels{adapter: "claude", outcome: "blocked", blockedReason: "multiple_viewers"}
	before := sumScrollForwardAttempts(t, labels)

	fakePM := &fakeScrollForwardProcessManager{}
	inst := newForwardScrollTestInstance(t, fakePM)
	outcome, blockedReason, _, err := inst.ForwardScroll(context.Background(), 2, nil)
	if err != nil {
		t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_BLOCKED || blockedReason != sessionv1.ScrollBlockedReason_MULTIPLE_VIEWERS {
		t.Fatalf("outcome/blockedReason = %v/%v, want BLOCKED/MULTIPLE_VIEWERS", outcome, blockedReason)
	}

	after := sumScrollForwardAttempts(t, labels)
	if after != before+1 {
		t.Fatalf("scroll_forward_attempts_total{adapter=claude,outcome=blocked,blocked_reason=multiple_viewers} = %d, want %d", after, before+1)
	}
}
