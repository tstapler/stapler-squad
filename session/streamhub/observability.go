package streamhub

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// This file wires plan.md's Observability Plan metrics to real OTel
// instruments via telemetry.GetMeter(), following the exact pattern
// session/unfinished.RegisterMetrics already established in this repo for
// its own custom counters/gauges — as opposed to the per-RPC tracing
// otelconnect.NewInterceptor already provides in server/server.go, which
// covers ConnectRPC method spans/latency, not hub-internal metrics like
// these. All six metrics are also backed by plain atomics (or, for
// streamhub_slow_subscriber_drops_total, StreamHub.slowSubscriberDropsTotal's
// existing per-hub field, Task 1.4.2) so callers/tests can read the current
// value directly rather than requiring an OTel test SDK/manual metric reader
// — matching this repo's existing bar for this kind of test
// (session/unfinished/metrics_test.go's TestRegisterMetrics_NoError is a
// smoke check that registration doesn't error, not a metricdata assertion).
// The three added here (subscribersPerHubLast, resizeNegotiationsTotal,
// batchFlushFramesCoalescedLast) round out that coverage — a
// spec-compliance sweep found only streamhub_active_hubs and
// streamhub_overlap_invariant_violations_total had this parallel accessor,
// even though all six instruments were already registered and wired at
// their call sites.
var (
	activeHubsCount                 atomic.Int64
	overlapInvariantViolationsCount atomic.Int64

	// subscribersPerHubLast and batchFlushFramesCoalescedLast track the most
	// recently recorded histogram observation, not a running total — these
	// two metrics are per-event measurements (fan-out width on one
	// AttachSubscriber call, frames folded into one batch flush), so "most
	// recent value" is the meaningful testable signal, mirroring what the
	// histogram itself last observed.
	subscribersPerHubLast         atomic.Int64
	resizeNegotiationsTotal       atomic.Int64
	batchFlushFramesCoalescedLast atomic.Int64
)

var (
	registerOnce sync.Once
	registerErr  error

	subscribersPerHubHist      metric.Int64Histogram
	resizeNegotiationsCounter  metric.Int64Counter
	batchFlushFramesHist       metric.Int64Histogram
	slowSubscriberDropsCounter metric.Int64Counter
	overlapInvariantCounter    metric.Int64Counter
)

func init() {
	if err := RegisterMetrics(); err != nil {
		log.Error("streamhub: failed to register OTel metrics", "error", err)
	}
}

// RegisterMetrics registers the six streamhub_* instruments named in
// plan.md's Observability Plan against the process's OTel MeterProvider
// (telemetry.GetMeter(), a delegating proxy safe to call before
// telemetry.Initialize — see session/unfinished.RegisterMetrics's identical
// pattern). Idempotent via sync.Once: package init already calls this once;
// it is exported so a caller (or a smoke test, matching
// session/unfinished/metrics_test.go) can call it again safely.
func RegisterMetrics() error {
	registerOnce.Do(func() {
		registerErr = registerMetricsOnce()
	})
	return registerErr
}

func registerMetricsOnce() error {
	meter := telemetry.GetMeter()

	activeHubsGauge, err := meter.Int64ObservableGauge("streamhub_active_hubs",
		metric.WithDescription("Count of live StreamHubs in this process"))
	if err != nil {
		return err
	}
	if _, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(activeHubsGauge, activeHubsCount.Load())
		return nil
	}, activeHubsGauge); err != nil {
		return err
	}

	if subscribersPerHubHist, err = meter.Int64Histogram("streamhub_subscribers_per_hub",
		metric.WithDescription("Fan-out width: subscriber count observed on each AttachSubscriber call")); err != nil {
		return err
	}

	if resizeNegotiationsCounter, err = meter.Int64Counter("streamhub_resize_negotiations_total",
		metric.WithDescription("Count of resize negotiations, tagged changed=true/false")); err != nil {
		return err
	}

	if batchFlushFramesHist, err = meter.Int64Histogram("streamhub_batch_flush_frames_coalesced",
		metric.WithDescription("Number of underlying output events folded into a single hub-side batch flush")); err != nil {
		return err
	}

	if slowSubscriberDropsCounter, err = meter.Int64Counter("streamhub_slow_subscriber_drops_total",
		metric.WithDescription("Count of slow-subscriber evictions across all hubs")); err != nil {
		return err
	}

	if overlapInvariantCounter, err = meter.Int64Counter("streamhub_overlap_invariant_violations_total",
		metric.WithDescription("Count of OverlapInvariant violations — must stay 0 after cutover")); err != nil {
		return err
	}

	return nil
}

// --- internal recording helpers, called from hub.go/batch.go/resize.go/ownership.go ---

func incActiveHubs() { activeHubsCount.Add(1) }
func decActiveHubs() { activeHubsCount.Add(-1) }

// ActiveHubs returns the current count of live StreamHubs in this process —
// the same value streamhub_active_hubs' gauge callback observes.
func ActiveHubs() int64 { return activeHubsCount.Load() }

func recordSubscribersPerHub(count int) {
	subscribersPerHubLast.Store(int64(count))
	if subscribersPerHubHist != nil {
		subscribersPerHubHist.Record(context.Background(), int64(count))
	}
}

// SubscribersPerHub returns the subscriber count observed on the most recent
// AttachSubscriber call across every hub in this process — the same value
// streamhub_subscribers_per_hub's histogram most recently recorded.
func SubscribersPerHub() int64 { return subscribersPerHubLast.Load() }

func recordResizeNegotiation(changed bool) {
	resizeNegotiationsTotal.Add(1)
	if resizeNegotiationsCounter != nil {
		resizeNegotiationsCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.Bool("changed", changed)))
	}
}

// ResizeNegotiationsTotal returns the process-wide count of resize
// negotiations recorded so far (every RequestResize call from a
// CanResize-eligible subscriber, regardless of whether it actually changed
// the negotiated size) — the same total streamhub_resize_negotiations_total
// accumulates.
func ResizeNegotiationsTotal() int64 { return resizeNegotiationsTotal.Load() }

func recordBatchFlushFramesCoalesced(frames int) {
	batchFlushFramesCoalescedLast.Store(int64(frames))
	if batchFlushFramesHist != nil {
		batchFlushFramesHist.Record(context.Background(), int64(frames))
	}
}

// BatchFlushFramesCoalesced returns the frames-coalesced count from the most
// recent batch flush across every hub in this process — the same value
// streamhub_batch_flush_frames_coalesced's histogram most recently recorded.
func BatchFlushFramesCoalesced() int64 { return batchFlushFramesCoalescedLast.Load() }

func recordSlowSubscriberDrop() {
	if slowSubscriberDropsCounter != nil {
		slowSubscriberDropsCounter.Add(context.Background(), 1)
	}
}

// recordOverlapInvariantViolation is called by OverlapInvariant
// (ownership.go) on every detected violation.
func recordOverlapInvariantViolation() {
	overlapInvariantViolationsCount.Add(1)
	if overlapInvariantCounter != nil {
		overlapInvariantCounter.Add(context.Background(), 1)
	}
}

// OverlapInvariantViolationsTotal returns the process-wide count of
// OverlapInvariant violations detected so far — the same value
// streamhub_overlap_invariant_violations_total's counter observes. Must
// stay 0 across the whole dark-launch window (plan.md's Observability
// Plan / Success Metric).
func OverlapInvariantViolationsTotal() int64 { return overlapInvariantViolationsCount.Load() }
