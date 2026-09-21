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

	// repoCacheColdOpens/repoCacheEvictions (see RepoCacheStats' doc comment):
	// root-causing whether blobCache's low hit rate above comes from repoCache
	// eviction churn (blobCache lives inside *cachedRepo, so an eviction wipes
	// it) versus the workload genuinely rarely revisiting the same blob needs
	// a trend over time, not a single /debug/blob-cache snapshot — these
	// gauges are that trend.
	repoCacheSize, err := meter.Int64ObservableGauge("unfinished.repo_cache.current_size",
		metric.WithDescription("Current number of *cachedRepo entries held in repoCache"))
	if err != nil {
		return err
	}
	repoCacheColdOpens, err := meter.Int64ObservableGauge("unfinished.repo_cache.cold_opens",
		metric.WithDescription("Cumulative count of repoCache entries opened from disk (cache miss on the repo path itself)"))
	if err != nil {
		return err
	}
	repoCacheEvictions, err := meter.Int64ObservableGauge("unfinished.repo_cache.evictions",
		metric.WithDescription("Cumulative count of *cachedRepo entries evicted via pruneRepoCache's TTL/LRU pass or ClearCache"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		stats := BlobCacheStatsSnapshot()
		o.ObserveInt64(hits, stats.Hits)
		o.ObserveInt64(misses, stats.Misses)
		o.ObserveInt64(timeSavedMs, stats.EstimatedTimeSaved.Milliseconds())

		rc := RepoCacheStatsSnapshot()
		o.ObserveInt64(repoCacheSize, rc.CurrentSize)
		o.ObserveInt64(repoCacheColdOpens, rc.ColdOpens)
		o.ObserveInt64(repoCacheEvictions, rc.Evictions)
		return nil
	}, hits, misses, timeSavedMs, repoCacheSize, repoCacheColdOpens, repoCacheEvictions)
	return err
}
