package session

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// sessionCreationWorktreeTypeMismatchTotal is the pre-mortem P1 remediation counter
// (worktree-envvars-hijack Epic 1.5, Story 1.5.1): research/architecture.md's Q1/Q2
// re-verification already establishes that no code path upstream of startLocked can
// resolve a non-remote SESSION_TYPE_NEW_WORKTREE request to anything other than
// SessionTypeNewWorktree, so this should be structurally impossible -- a nonzero
// count in production is itself the signal that an unanticipated third trigger
// exists. Registered once, package-level, via telemetry.GetMeter() -- the same
// idiom session/scroll_forward_metrics.go and
// server/services/session_creation_metrics.go already use.
var sessionCreationWorktreeTypeMismatchTotal = mustInt64CounterWorktreeTypeMismatch(telemetry.GetMeter(),
	"session_creation_worktree_type_mismatch_total",
	metric.WithDescription("Count of non-remote SESSION_TYPE_NEW_WORKTREE requests whose Instance resolved to a SessionType other than SessionTypeNewWorktree by the time startLocked runs -- should never be nonzero"))

func mustInt64CounterWorktreeTypeMismatch(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		// Only returned for a malformed instrument name/config, a build-time-constant
		// programmer error -- panicking at package init surfaces it immediately in
		// tests rather than silently dropping the metric forever.
		panic(err)
	}
	return counter
}

// recordSessionCreationWorktreeTypeMismatch increments
// sessionCreationWorktreeTypeMismatchTotal, labeled by the SessionType the
// instance actually resolved to, for one detected mismatch.
func recordSessionCreationWorktreeTypeMismatch(resolvedSessionType SessionType) {
	sessionCreationWorktreeTypeMismatchTotal.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("resolved_session_type", string(resolvedSessionType)),
	))
}
