package tmux

// exec_gate_observability.go wires OTel metrics and a span to every exec-gate
// acquire/run cycle (session/tmux/exec_gate.go), following the same
// register-once-via-telemetry.GetMeter() pattern session/streamhub/observability.go
// already established in this repo.
//
// Investigation trigger (2026-09-06): "[streamViaControlMode] failed to handle
// mid-stream current pane request ... exec gate: context deadline exceeded" kept
// recurring even after fixing the resize step's unbounded latency (see
// Instance.ResizePTYContext). Diagnosing it required manually sampling flock()
// state on disk with a shell loop — there was no queryable signal for "how long did
// this call actually wait for a slot" vs "how long did the slot-holder actually run
// once acquired" vs "is the pool saturated or nearly idle." This file makes that a
// permanent, per-pool, per-outcome metric and span instead of a one-off shell probe.

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	execGateRegisterOnce sync.Once
	execGateRegisterErr  error

	execGateWaitHist    metric.Int64Histogram
	execGateExecHist    metric.Int64Histogram
	execGateTimeoutCtr  metric.Int64Counter
	execGateAcquiredCtr metric.Int64Counter
)

func init() {
	if err := RegisterExecGateMetrics(); err != nil {
		log.Error("tmux: failed to register exec-gate OTel metrics", "error", err)
	}
}

// RegisterExecGateMetrics registers the tmux_exec_gate_* instruments against
// telemetry.GetMeter(). Idempotent via sync.Once — package init already calls
// this once; exported so a test can call it again safely.
func RegisterExecGateMetrics() error {
	execGateRegisterOnce.Do(func() {
		execGateRegisterErr = registerExecGateMetricsOnce()
	})
	return execGateRegisterErr
}

func registerExecGateMetricsOnce() error {
	meter := telemetry.GetMeter()

	var err error
	if execGateWaitHist, err = meter.Int64Histogram("tmux_exec_gate_wait_duration_ms",
		metric.WithDescription("Time spent waiting for a free exec-gate slot, by pool and outcome (acquired/timeout)"),
		metric.WithUnit("ms")); err != nil {
		return err
	}
	if execGateExecHist, err = meter.Int64Histogram("tmux_exec_gate_exec_duration_ms",
		metric.WithDescription("Time spent running the guarded subprocess call once a slot was acquired, by pool"),
		metric.WithUnit("ms")); err != nil {
		return err
	}
	if execGateTimeoutCtr, err = meter.Int64Counter("tmux_exec_gate_timeouts_total",
		metric.WithDescription("Count of exec-gate acquires that gave up waiting for a free slot, by pool")); err != nil {
		return err
	}
	if execGateAcquiredCtr, err = meter.Int64Counter("tmux_exec_gate_acquired_total",
		metric.WithDescription("Count of exec-gate acquires that got a free slot, by pool")); err != nil {
		return err
	}
	return nil
}

// recordExecGateWait records how long a call waited to acquire a slot from
// pool (before running anything). timedOut distinguishes "gave up waiting"
// from "got a slot" — the two have very different causes (pool saturation vs.
// normal queueing) and must not be averaged together.
func recordExecGateWait(pool string, wait time.Duration, timedOut bool) {
	attrs := metric.WithAttributes(attribute.String("pool", pool))
	if execGateWaitHist != nil {
		execGateWaitHist.Record(context.Background(), wait.Milliseconds(), attrs)
	}
	if timedOut {
		if execGateTimeoutCtr != nil {
			execGateTimeoutCtr.Add(context.Background(), 1, attrs)
		}
		return
	}
	if execGateAcquiredCtr != nil {
		execGateAcquiredCtr.Add(context.Background(), 1, attrs)
	}
}

// recordExecGateExec records how long the guarded subprocess call itself took
// to run, once a slot was already acquired — the piece "how fast is the exec
// gate" alone can't answer: a call can wait 0ms for a slot and still be slow
// because the subprocess/tmux round-trip itself is slow, which is a
// completely different problem (tmux server health, not gate capacity) from a
// long wait (too many concurrent callers for too few slots).
func recordExecGateExec(pool string, exec time.Duration) {
	if execGateExecHist != nil {
		execGateExecHist.Record(context.Background(), exec.Milliseconds(), metric.WithAttributes(attribute.String("pool", pool)))
	}
}
