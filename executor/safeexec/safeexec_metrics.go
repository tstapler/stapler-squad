//go:build !windows

package safeexec

import (
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// sigkillEscalations counts confirmed SIGKILL escalations from
// CommandContextPG's cmd.Cancel path — i.e. cases where the process group
// did not honor SIGTERM within sigkillGrace and had to be force-killed.
// "Confirmed" excludes ESRCH (the group already exited on its own); see the
// call site in safeexec_pg.go.
//
// Built against telemetry.GetMeter(), which is safe to call before
// telemetry.Initialize (returns a no-op meter that later starts exporting
// once a real MeterProvider is installed) — see session/unfinished/metrics.go
// for the established pattern this mirrors.
var sigkillEscalations = mustInt64Counter(telemetry.GetMeter(), "safeexec.sigkill_escalations",
	metric.WithDescription("Confirmed SIGKILL escalations after a process group ignored SIGTERM within CommandContextPG's grace period"))

func mustInt64Counter(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		// Only returned for a malformed instrument name/config, which is a
		// build-time-constant programmer error here, not a runtime
		// condition — panicking at package init surfaces it immediately in
		// tests rather than silently dropping the metric forever.
		panic(err)
	}
	return counter
}
