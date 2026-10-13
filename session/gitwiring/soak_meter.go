package gitwiring

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// soakMeter is the meter every git backend counter is registered on. It forwards to the process
// telemetry meter (so OTLP export is unchanged) and also to a private in-process reader, which
// is the only way to read the soak counters when OTLP is off (the default).
var (
	soakReader = sdkmetric.NewManualReader()
	soakMeter  = sync.OnceValue(func() metric.Meter {
		local := sdkmetric.NewMeterProvider(sdkmetric.WithReader(soakReader)).Meter("git-backend-soak")
		return teeMeter{primary: telemetry.GetMeter(), local: local}
	})
)

// teeMeter implements only the instruments the git backend uses; any other instrument is a
// no-op rather than silently half-registered.
type teeMeter struct {
	noop.Meter
	primary, local metric.Meter
}

func (t teeMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	p, perr := t.primary.Int64Counter(name, opts...)
	l, lerr := t.local.Int64Counter(name, opts...)
	switch {
	case lerr != nil:
		return p, perr
	case perr != nil:
		return l, nil
	}
	return teeCounter{p: p, l: l}, nil
}

func (t teeMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	p, perr := t.primary.Float64Histogram(name, opts...)
	l, lerr := t.local.Float64Histogram(name, opts...)
	switch {
	case lerr != nil:
		return p, perr
	case perr != nil:
		return l, nil
	}
	return teeHistogram{p: p, l: l}, nil
}

type teeCounter struct {
	noop.Int64Counter
	p, l metric.Int64Counter
}

func (c teeCounter) Add(ctx context.Context, v int64, opts ...metric.AddOption) {
	c.p.Add(ctx, v, opts...)
	c.l.Add(ctx, v, opts...)
}

type teeHistogram struct {
	noop.Float64Histogram
	p, l metric.Float64Histogram
}

func (h teeHistogram) Record(ctx context.Context, v float64, opts ...metric.RecordOption) {
	h.p.Record(ctx, v, opts...)
	h.l.Record(ctx, v, opts...)
}
