package backend

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/tstapler/stapler-squad/log"
)

// MetricCLISpawnTotal counts every git process the CLI backend starts, by the operation it
// served and why the CLI served it ({operation,reason}). It is the zero-spawn gate metric: no
// URL, path, argument or call site is ever a label.
const MetricCLISpawnTotal = "git_backend_cli_spawn_total"

// SpawnDumpEnv names the test-only file that receives one line per counted spawn, with the
// caller's file:line. The line number is a diagnostic, never a metric label (it changes on
// every edit and collapses to the CLI helper after migration).
const SpawnDumpEnv = "SSQ_GIT_SPAWN_DUMP"

// SpawnReasonUnattributed labels a spawn that reached the CLI without a CallInfo, i.e. a
// caller that bypassed the Router. It is outside the FallbackReason enum on purpose: the router
// never produces it, so a nonzero count always means a missing attribution.
const SpawnReasonUnattributed FallbackReason = "unattributed"

// SpawnReasons is the closed set of reason labels git_backend_cli_spawn_total may carry.
func SpawnReasons() []FallbackReason {
	return append(ReasonPrecedence(), SpawnReasonUnattributed)
}

// SpawnCounter increments git_backend_cli_spawn_total. The zero value and nil count nothing.
type SpawnCounter struct {
	counter metric.Int64Counter
}

// NewSpawnCounter registers the counter on meter. A registration failure is logged and
// disables counting rather than failing construction.
func NewSpawnCounter(meter metric.Meter) *SpawnCounter {
	c, err := meter.Int64Counter(MetricCLISpawnTotal,
		metric.WithDescription("git processes started by the CLI backend, by operation and reason"))
	if err != nil {
		log.Warn("git backend metric unavailable, counting disabled", "metric", MetricCLISpawnTotal, "err", err)
		c = noop.Int64Counter{}
	}
	return &SpawnCounter{counter: c}
}

// SpawnLabels resolves the (operation, reason) labels for one spawn. The Router's CallInfo
// wins. Without one, a remote run is still reason remote_host (it never starts a local git, so
// the gate must not see it as an unattributed local spawn) and a local run is attributed to
// servedOp under SpawnReasonUnattributed. A CallInfo naming an operation or reason outside the
// closed enums is treated as absent, so a label can never carry free text.
func SpawnLabels(ctx context.Context, servedOp OperationName, remote bool) (OperationName, FallbackReason) {
	if info, ok := CallInfoFrom(ctx); ok && HasCohort(info.Op) && info.Reason.Known() {
		return info.Op, info.Reason
	}
	reason := SpawnReasonUnattributed
	if remote {
		reason = ReasonRemoteHost
	}
	if !HasCohort(servedOp) {
		return "unknown", reason
	}
	return servedOp, reason
}

// Count records one git process start for a call to servedOp. It is called just before the
// Runner runs, so a Run that fails to start still counts: the counter means "git was asked to
// start", and the gate compares it with the backstop, which counts at the same point.
func (s *SpawnCounter) Count(ctx context.Context, servedOp OperationName, remote bool) {
	if s == nil || s.counter == nil {
		return
	}
	op, reason := SpawnLabels(ctx, servedOp, remote)
	s.counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", string(op)),
		attribute.String("reason", string(reason))))
	if path := os.Getenv(SpawnDumpEnv); path != "" {
		dumpSpawn(path, op, reason)
	}
}

const backendPkgPrefix = "github.com/tstapler/stapler-squad/session/git/backend"

func dumpSpawn(path string, op OperationName, reason FallbackReason) {
	line := fmt.Sprintf("backend %s operation=%s reason=%s\n", callerOutsideBackend(), op, reason)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- SSQ_GIT_SPAWN_DUMP is a developer-set, test-only variable; anyone who controls the service environment already controls the process
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// callerOutsideBackend is the file:line of the first frame outside the backend packages (and
// outside testing's own plumbing), which is whoever asked the Backend for the operation.
func callerOutsideBackend() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		fr, more := frames.Next()
		if !strings.HasPrefix(fr.Function, backendPkgPrefix) || strings.Contains(fr.Function, ".Test") {
			return fmt.Sprintf("%s:%d", fr.File, fr.Line)
		}
		if !more {
			return "unknown:0"
		}
	}
}
