package tmux

// control_mode_observability.go wires OTel metrics and a span to every tmux
// control-mode command round trip (session/tmux/control_mode.go's
// enqueueCMCommand/sendCMCommand), following the same register-once pattern
// exec_gate_observability.go and session/streamhub/observability.go already
// established in this repo.
//
// Investigation trigger (2026-09-06): after instrumenting the exec gate (see
// exec_gate_observability.go) to diagnose recurring "exec gate: context
// deadline exceeded" errors, a live-reproduced trace showed the exec gate
// itself was NOT the bottleneck — both exec-gate child spans landed in the
// final fraction of a millisecond of a 3-second parent span, meaning nearly
// the entire budget was consumed *before* the exec gate was ever reached.
// GetPaneDimensionsPriority/resize try tmux control-mode first (sendCMCommand)
// and only fall back to the exec-gate subprocess path if that fails — and
// control-mode itself (a per-session, single-threaded command queue whose
// reader goroutine also has to process every %output line of live terminal
// scrollback) had zero instrumentation. This file closes that gap: it makes
// "how long did this control-mode command actually take, and how backed up
// was the response queue when we sent it" a queryable signal instead of a
// black box.
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
	cmRegisterOnce sync.Once
	cmRegisterErr  error

	cmCommandDurationHist metric.Int64Histogram
	cmTimeoutCtr          metric.Int64Counter
	cmPendingDepthHist    metric.Int64Histogram
)

func init() {
	if err := RegisterControlModeMetrics(); err != nil {
		log.Error("tmux: failed to register control-mode OTel metrics", "error", err)
	}
}

// RegisterControlModeMetrics registers the tmux_control_mode_* instruments
// against telemetry.GetMeter(). Idempotent via sync.Once — package init
// already calls this once; exported so a test can call it again safely.
func RegisterControlModeMetrics() error {
	cmRegisterOnce.Do(func() {
		cmRegisterErr = registerControlModeMetricsOnce()
	})
	return cmRegisterErr
}

func registerControlModeMetricsOnce() error {
	meter := telemetry.GetMeter()

	var err error
	if cmCommandDurationHist, err = meter.Int64Histogram("tmux_control_mode_command_duration_ms",
		metric.WithDescription("End-to-end latency of one control-mode command round trip (enqueue + tmux response), by command and outcome"),
		metric.WithUnit("ms")); err != nil {
		return err
	}
	if cmTimeoutCtr, err = meter.Int64Counter("tmux_control_mode_timeouts_total",
		metric.WithDescription("Count of control-mode commands that never got a response before ctx expired, by command")); err != nil {
		return err
	}
	if cmPendingDepthHist, err = meter.Int64Histogram("tmux_control_mode_pending_depth",
		metric.WithDescription("Number of control-mode commands already awaiting a response when this one was enqueued (FIFO backlog depth)")); err != nil {
		return err
	}
	return nil
}

// recordControlModeCommand records one full sendCMCommand/enqueueCMCommand
// round trip. command is the tmux subcommand (args[0], e.g. "display-message",
// "resize-window", "capture-pane") — never the full argument list, which can
// carry pane content and would blow up cardinality.
func recordControlModeCommand(command string, dur time.Duration, timedOut bool) {
	attrs := metric.WithAttributes(attribute.String("command", command))
	if cmCommandDurationHist != nil {
		cmCommandDurationHist.Record(context.Background(), dur.Milliseconds(), attrs)
	}
	if timedOut && cmTimeoutCtr != nil {
		cmTimeoutCtr.Add(context.Background(), 1, attrs)
	}
}

// recordControlModePendingDepth samples the FIFO backlog depth (len(pendingCmds))
// at the moment a new command is enqueued — the direct answer to "was the
// response queue backed up when this command was sent," independent of
// whether this specific command ends up slow itself.
func recordControlModePendingDepth(depth int) {
	if cmPendingDepthHist != nil {
		cmPendingDepthHist.Record(context.Background(), int64(depth))
	}
}
