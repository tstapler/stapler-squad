package unfinished

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// RegisterMetrics wires blobCache effectiveness (see BlobCacheStatsSnapshot)
// into the process's OTel MeterProvider as observable gauges, so it shows up
// in Datadog/OTLP alongside every other metric — not just the /debug/blob-cache
// JSON endpoint (profiling.StartProfiling). Both read the same snapshot
// function, so they can never disagree with each other.
//
// Safe to call even when telemetry is disabled or before telemetry.Initialize
// has run: the OTel global Meter is a delegating proxy — instruments created
// against it now start exporting retroactively once a real MeterProvider is
// installed later (see telemetry.Initialize). Call once per process; calling
// it more than once registers duplicate instruments.
func RegisterMetrics() error {
	meter := telemetry.GetMeter()

	hits, err := meter.Int64ObservableGauge("unfinished.blob_cache.hits",
		metric.WithDescription("Cumulative blobCache hits across all repos this process has scanned"))
	if err != nil {
		return err
	}
	misses, err := meter.Int64ObservableGauge("unfinished.blob_cache.misses",
		metric.WithDescription("Cumulative blobCache misses across all repos this process has scanned"))
	if err != nil {
		return err
	}
	timeSavedMs, err := meter.Int64ObservableGauge("unfinished.blob_cache.estimated_time_saved_ms",
		metric.WithDescription("Estimated packfile decompression time avoided by blobCache hits (hits * average observed miss duration)"),
		metric.WithUnit("ms"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stats := BlobCacheStatsSnapshot()
		o.ObserveInt64(hits, stats.Hits)
		o.ObserveInt64(misses, stats.Misses)
		o.ObserveInt64(timeSavedMs, stats.EstimatedTimeSaved.Milliseconds())
		return nil
	}, hits, misses, timeSavedMs)
	return err
}
