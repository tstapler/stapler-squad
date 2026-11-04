package session

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Values of session_backend_info's "source" label.
const (
	backendInfoSourceGlobal          = "global"
	backendInfoSourceSessionOverride = "session_override"
)

// backendInfoState is the last-observed backend configuration, republished
// by RecordBackendInfo. The gauge callback reads it instead of loading
// config itself: LoadConfig can write a default file, which a metrics
// collection must never do.
type backendInfoState struct {
	global        ProcessManagerBackend
	overrideTymux bool // some session pinned onto tymux
	overrideTmux  bool // some session pinned onto tmux
}

var latestBackendInfo atomic.Pointer[backendInfoState]

func init() {
	gauge, err := telemetry.GetMeter().Int64ObservableGauge("session_backend_info",
		metric.WithDescription("Info-style gauge (value 1): the backend in effect (tmux/tymux/native) and where it came from (global default or per-session override). Labels are low cardinality; never per-session."),
		metric.WithInt64Callback(observeBackendInfo))
	if err != nil {
		log.Error("session: failed to register session_backend_info", "error", err)
	}
	_ = gauge
}

// RecordBackendInfo refreshes session_backend_info from cfg. Call wherever
// the backend choice is resolved or reported (ResolveSessionBackend, the
// tymux rollout status RPC).
func RecordBackendInfo(cfg *config.Config) {
	if cfg == nil {
		return
	}
	global := selectedBackendFromConfig(cfg)
	if global == "" {
		global = BackendTmux
	}
	st := &backendInfoState{global: global}
	for _, forceTymux := range cfg.TymuxSessionOverrides {
		if forceTymux {
			st.overrideTymux = true
		} else {
			st.overrideTmux = true
		}
	}
	latestBackendInfo.Store(st)
}

func observeBackendInfo(_ context.Context, o metric.Int64Observer) error {
	st := latestBackendInfo.Load()
	if st == nil {
		return nil
	}
	observe := func(backend ProcessManagerBackend, source string) {
		o.Observe(1, metric.WithAttributes(
			attribute.String("backend", string(backend)),
			attribute.String("source", source)))
	}
	observe(st.global, backendInfoSourceGlobal)
	if st.overrideTymux {
		observe(BackendTymux, backendInfoSourceSessionOverride)
	}
	if st.overrideTmux {
		observe(BackendTmux, backendInfoSourceSessionOverride)
	}
	return nil
}
