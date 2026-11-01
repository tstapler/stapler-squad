package dbmaint

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	regMu       sync.Mutex
	registered  = map[*Maintainer]struct{}{}
	metricsOnce sync.Once
)

func register(m *Maintainer) {
	metricsOnce.Do(initMetrics)
	regMu.Lock()
	registered[m] = struct{}{}
	regMu.Unlock()
}

func unregister(m *Maintainer) {
	regMu.Lock()
	delete(registered, m)
	regMu.Unlock()
}

// Snapshot returns the maintenance stats of every running Maintainer, for /actuator/db.
func Snapshot() []Stats {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]Stats, 0, len(registered))
	for m := range registered {
		out = append(out, m.snapshot())
	}
	return out
}

// initMetrics registers observable gauges against the global (delegating) meter,
// so they export once telemetry.Initialize installs a real provider.
func initMetrics() {
	meter := telemetry.GetMeter()
	var failed bool
	gauge := func(name, desc string) metric.Float64ObservableGauge {
		g, err := meter.Float64ObservableGauge(name, metric.WithDescription(desc))
		failed = failed || err != nil
		return g
	}
	free := gauge("db.maint.free_pages", "SQLite freelist pages (reclaimable)")
	total := gauge("db.maint.page_count", "SQLite total pages")
	reclaimed := gauge("db.maint.reclaimed_pages_total", "Cumulative pages returned by incremental_vacuum")
	lastRun := gauge("db.maint.last_run_timestamp_seconds", "Unix time of the last maintenance tick")
	lastErr := gauge("db.maint.last_error", "1 when the last maintenance tick failed")
	mode := gauge("db.maint.auto_vacuum_mode", "PRAGMA auto_vacuum: 0 none, 1 full, 2 incremental")
	if failed {
		return
	}
	_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for _, s := range Snapshot() {
			a := metric.WithAttributes(attribute.String("db.name", s.Name))
			o.ObserveFloat64(free, float64(s.FreePages), a)
			o.ObserveFloat64(total, float64(s.PageCount), a)
			o.ObserveFloat64(reclaimed, float64(s.ReclaimedPagesTotal), a)
			o.ObserveFloat64(mode, float64(s.AutoVacuumModeValue), a)
			var errVal float64
			if s.LastError != "" {
				errVal = 1
			}
			o.ObserveFloat64(lastErr, errVal, a)
			if !s.LastRun.IsZero() {
				o.ObserveFloat64(lastRun, float64(s.LastRun.Unix()), a)
			}
		}
		return nil
	}, free, total, reclaimed, lastRun, lastErr, mode)
}
