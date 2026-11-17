package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

// CounterNamed resolves the instrument on every use (the SDK returns the same
// one per meter), so a test that swaps the meter provider is never stuck with a
// counter bound to an earlier one.
func CounterNamed(name, desc string) func() metric.Int64Counter {
	return func() metric.Int64Counter {
		c, err := GetMeter().Int64Counter(name, metric.WithDescription(desc))
		if err != nil {
			log.Error("telemetry: failed to register "+name, "error", err)
			return nil
		}
		return c
	}
}

// AddOne increments c by one; a nil counter (failed registration) is a no-op.
func AddOne(c metric.Int64Counter, attrs ...attribute.KeyValue) {
	if c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}
