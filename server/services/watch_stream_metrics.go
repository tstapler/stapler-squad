package services

// watch_stream_metrics.go instruments how many server-streaming RPC calls
// (the WatchSessions/WatchReviewQueue/WatchBacklogItems/WatchInsights/
// WatchUnfinishedWork family, plus the terminal WebSocket stream) are
// concurrently open, per RPC method. This exists to catch the class of bug
// root-caused in 2026-09: several of these streams held open at once, on a
// plain-HTTP/1.1 server, can exhaust a browser's 6-connections-per-origin
// budget and starve unrelated unary RPCs into a client-side
// deadline_exceeded before the server ever sees them (see
// docs/how-to/enable-opentelemetry.md's "Connection concurrency metrics"
// section). Graphing rpc.server_streams.open by method answers "how many
// Watch* streams does one browser tab hold open right now" directly, instead
// of inferring it after the fact from Tempo traces (which show nothing for a
// request that never reached the server).
//
// Follows the same package-level-atomics-plus-Observable-gauge-callback
// pattern as session/streamhub/observability.go's streamhub_active_hubs:
// registered once via sync.Once/init() against telemetry.GetMeter() (a
// no-op-safe delegating meter, safe to call before telemetry.Initialize).

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// openStreamCounts maps RPC method name (e.g. "WatchSessions") to a live
// count of currently-open streams for that method. sync.Map fits this
// better than xsync.MapOf here: writes only happen on stream open/close
// (low frequency relative to the hot paths xsync targets elsewhere in this
// repo), and the entry set is small and effectively static (one entry per
// Watch* method), which is exactly sync.Map's optimized case.
var openStreamCounts sync.Map // string -> *atomic.Int64

var (
	watchStreamMetricsRegisterOnce sync.Once
	watchStreamMetricsRegisterErr  error
)

func init() {
	watchStreamMetricsRegisterOnce.Do(func() {
		watchStreamMetricsRegisterErr = registerWatchStreamMetrics()
		if watchStreamMetricsRegisterErr != nil {
			log.Error("services: failed to register watch-stream OTel metrics", "error", watchStreamMetricsRegisterErr)
		}
	})
}

func registerWatchStreamMetrics() error {
	meter := telemetry.GetMeter()

	gauge, err := meter.Int64ObservableGauge("rpc.server_streams.open",
		metric.WithDescription("Count of currently-open server-streaming RPC calls, labeled by method (WatchSessions, WatchReviewQueue, WatchBacklogItems, WatchInsights, WatchUnfinishedWork, StreamTerminal) — a proxy for how many long-lived connections one client is holding open"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		openStreamCounts.Range(func(key, value any) bool {
			method := key.(string)
			count := value.(*atomic.Int64).Load()
			o.ObserveInt64(gauge, count, metric.WithAttributes(attribute.String("method", method)))
			return true
		})
		return nil
	}, gauge)
	return err
}

// TrackOpenStream marks one server-streaming RPC call named method as open,
// incrementing rpc.server_streams.open{method=...}. The caller must invoke
// the returned done func exactly once when the stream closes (typically via
// defer right after the call), which decrements the count back down.
//
// Call this at the top of each Watch* handler's body / the terminal stream's
// entry point, before the blocking send/receive loop starts:
//
//	done := services.TrackOpenStream("WatchSessions")
//	defer done()
func TrackOpenStream(method string) (done func()) {
	v, _ := openStreamCounts.LoadOrStore(method, new(atomic.Int64))
	counter := v.(*atomic.Int64)
	counter.Add(1)
	return func() { counter.Add(-1) }
}
