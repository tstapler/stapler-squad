package streammetrics

import "go.opentelemetry.io/otel/metric"

// MustInt64Counter registers an Int64Counter or panics: an error is only
// returned for a malformed build-time-constant name/config, and panicking at
// package init surfaces it in tests instead of silently dropping the metric.
func MustInt64Counter(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		panic(err)
	}
	return counter
}

// MustFloat64Histogram registers a Float64Histogram or panics; see MustInt64Counter.
func MustFloat64Histogram(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return histogram
}
