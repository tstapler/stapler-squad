package dbobs

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

type poolGauges struct {
	inUse, open, waitCount, waitMs, slow metric.Float64ObservableGauge
}

func newPoolGauges(m metric.Meter) *poolGauges {
	var failed bool
	mk := func(opts ...metric.Float64ObservableGaugeOption) func(string) metric.Float64ObservableGauge {
		return func(name string) metric.Float64ObservableGauge {
			g, err := m.Float64ObservableGauge(name, opts...)
			failed = failed || err != nil
			return g
		}
	}
	g := &poolGauges{
		inUse:     mk(metric.WithDescription("Connections currently in use"))("db.pool.in_use"),
		open:      mk(metric.WithDescription("Open connections"))("db.pool.open"),
		waitCount: mk(metric.WithDescription("Cumulative number of connection waits"))("db.pool.wait_count"),
		waitMs:    mk(metric.WithDescription("Cumulative time blocked waiting for a connection"))("db.pool.wait_duration_ms"),
		slow:      mk(metric.WithDescription("Cumulative queries over the slow-query threshold"))("db.slow_queries_total"),
	}
	if failed {
		return nil
	}
	return g
}

func (g *poolGauges) instruments() []metric.Observable {
	return []metric.Observable{g.inUse, g.open, g.waitCount, g.waitMs, g.slow}
}

func (g *poolGauges) observe(_ context.Context, o metric.Observer) error {
	for _, s := range Snapshot() {
		a := metric.WithAttributes(nameAttr(s.Name))
		o.ObserveFloat64(g.inUse, float64(s.InUse), a)
		o.ObserveFloat64(g.open, float64(s.OpenConnections), a)
		o.ObserveFloat64(g.waitCount, float64(s.WaitCount), a)
		o.ObserveFloat64(g.waitMs, s.WaitDurationMs, a)
		o.ObserveFloat64(g.slow, float64(s.SlowQueries), a)
	}
	return nil
}
