package unfinished

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// scanCyclesTotal counts completed scan cycles (one worker task: scanRepo +
// publishResults) since process start. Diagnostic normalisation only: gives
// "scanner CPU per scan" a denominator for the baseline and A/B windows.
var scanCyclesTotal atomic.Int64

// recordScanCycle is called by Scanner.worker after each completed task.
func recordScanCycle() { scanCyclesTotal.Add(1) }

// ScanCyclesTotal returns the number of scan cycles completed since process start.
func ScanCyclesTotal() int64 { return scanCyclesTotal.Load() }

// redundancyOutcome labels the merge-base redundancy checks (isAncestorOfOther
// and repostate's reaches) that answered "not redundant" without finding the
// candidate: where a skewed committer clock can leave a redundant base in place.
type redundancyOutcome int

const (
	redundancyBound redundancyOutcome = iota
	redundancyPrunedMiss
	numRedundancyOutcomes
)

var redundancyOutcomeNames = [numRedundancyOutcomes]string{"bound", "pruned_miss"}

// mergeBaseRedundancyTotal counts the plain walk's outcomes; the id walk's live
// in repostate.GraphStats and are summed in by the repostate inputs.
var mergeBaseRedundancyTotal [numRedundancyOutcomes]atomic.Int64

func recordRedundancyOutcome(o redundancyOutcome) { mergeBaseRedundancyTotal[o].Add(1) }

// dirtyFullWalkTotal counts forced full walks per FullWalkReason
// (dirty_full_walk_total{reason}).
var dirtyFullWalkTotal [numFullWalkReasons]atomic.Int64

func recordDirtyFullWalk(r FullWalkReason) {
	if r > ReasonNone && r < numFullWalkReasons {
		dirtyFullWalkTotal[r].Add(1)
	}
}

// DirtyFullWalkTotal returns the forced-full-walk count for reason r.
func DirtyFullWalkTotal(r FullWalkReason) int64 {
	if r >= numFullWalkReasons {
		return 0
	}
	return dirtyFullWalkTotal[r].Load()
}

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

	scanCycles, err := meter.Int64ObservableCounter("unfinished.scan_cycles_total",
		metric.WithDescription("Cumulative scan cycles completed by the unfinished-work scanner (one per repo scan task)"))
	if err != nil {
		return err
	}

	dirtyFullWalks, err := meter.Int64ObservableCounter("unfinished.dirty_full_walk_total",
		metric.WithDescription("Cumulative forced full walks by reason (overflow|error|dir_event|age|unarmed)"))
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
		o.ObserveInt64(scanCycles, ScanCyclesTotal())
		for r := ReasonOverflow; r < numFullWalkReasons; r++ {
			o.ObserveInt64(dirtyFullWalks, DirtyFullWalkTotal(r),
				metric.WithAttributes(attribute.String("reason", r.String())))
		}
		return nil
	}, hits, misses, timeSavedMs, repoCacheSize, repoCacheColdOpens, repoCacheEvictions, scanCycles, dirtyFullWalks)
	if err != nil {
		return err
	}
	return registerRepostateMetrics(meter, collectRepostateInputs)
}
