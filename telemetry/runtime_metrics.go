package telemetry

// runtime_metrics.go exports Go-runtime saturation signals that explain
// "everything is slow at once": scheduler latency (time runnable goroutines wait
// for a P — the direct measure of CPU starvation), GC pause, goroutine count,
// heap, and cumulative sync.Mutex wait. Percentiles are computed over the delta
// since the previous scrape, so a gauge reflects the last scrape interval rather
// than the process lifetime. Read from runtime/metrics in one callback.

import (
	"context"
	"math"
	"runtime/metrics"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

const (
	rtSchedLatency = "/sched/latencies:seconds"
	rtGCPauses     = "/gc/pauses:seconds"
	rtGoroutines   = "/sched/goroutines:goroutines"
	rtHeapObjects  = "/memory/classes/heap/objects:bytes"
	rtMutexWait    = "/sync/mutex/wait/total:seconds"
)

var runtimeMetricsOnce sync.Once

func init() {
	runtimeMetricsOnce.Do(func() {
		if err := registerRuntimeMetrics(); err != nil {
			log.Warn("telemetry: failed to register runtime metrics", "error", err)
		}
	})
}

// histDelta tracks a cumulative runtime/metrics histogram between scrapes.
type histDelta struct {
	prev []uint64
}

// quantilesSince returns the qs-quantiles (upper bucket bounds) of observations
// added since the previous call, or ok=false if there were none.
func (h *histDelta) quantilesSince(cur *metrics.Float64Histogram, qs ...float64) ([]float64, bool) {
	delta := make([]uint64, len(cur.Counts))
	var total uint64
	for i, c := range cur.Counts {
		var p uint64
		if i < len(h.prev) {
			p = h.prev[i]
		}
		if c >= p {
			delta[i] = c - p
		}
		total += delta[i]
	}
	h.prev = append(h.prev[:0], cur.Counts...)
	if total == 0 {
		return nil, false
	}
	out := make([]float64, len(qs))
	for i, q := range qs {
		out[i] = bucketQuantile(cur.Buckets, delta, total, q)
	}
	return out, true
}

// bucketQuantile returns the upper bound of the bucket holding the q-quantile.
// buckets has len(counts)+1 boundaries; the last may be +Inf, in which case the
// finite lower bound is reported.
func bucketQuantile(buckets []float64, counts []uint64, total uint64, q float64) float64 {
	rank := uint64(math.Ceil(q * float64(total)))
	if rank == 0 {
		rank = 1
	}
	var cum uint64
	for i, c := range counts {
		cum += c
		if cum >= rank {
			upper := buckets[i+1]
			if math.IsInf(upper, 1) {
				return buckets[i]
			}
			return upper
		}
	}
	return buckets[len(buckets)-1]
}

func registerRuntimeMetrics() error {
	meter := GetMeter()
	goroutines, err := meter.Int64ObservableGauge("go.sched.goroutines", metric.WithDescription("Live goroutines"))
	if err != nil {
		return err
	}
	schedP99, err := meter.Float64ObservableGauge("go.sched.latency_p99_seconds",
		metric.WithDescription("p99 time runnable goroutines waited for a processor over the last scrape interval (CPU starvation)"))
	if err != nil {
		return err
	}
	schedP50, err := meter.Float64ObservableGauge("go.sched.latency_p50_seconds",
		metric.WithDescription("p50 scheduler latency over the last scrape interval"))
	if err != nil {
		return err
	}
	gcP99, err := meter.Float64ObservableGauge("go.gc.pause_p99_seconds",
		metric.WithDescription("p99 stop-the-world GC pause over the last scrape interval"))
	if err != nil {
		return err
	}
	heap, err := meter.Int64ObservableGauge("go.heap.objects_bytes", metric.WithDescription("Bytes occupied by live and not-yet-swept heap objects"))
	if err != nil {
		return err
	}
	mutexWait, err := meter.Float64ObservableCounter("go.sync.mutex_wait_seconds_total",
		metric.WithDescription("Cumulative time goroutines spent blocked on sync.Mutex/RWMutex"))
	if err != nil {
		return err
	}

	samples := []metrics.Sample{{Name: rtSchedLatency}, {Name: rtGCPauses}, {Name: rtGoroutines}, {Name: rtHeapObjects}, {Name: rtMutexWait}}
	var mu sync.Mutex
	var sched, gc histDelta
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		mu.Lock()
		defer mu.Unlock()
		metrics.Read(samples)
		for _, s := range samples {
			switch s.Name {
			case rtGoroutines:
				if s.Value.Kind() == metrics.KindUint64 {
					o.ObserveInt64(goroutines, int64(s.Value.Uint64())) //nolint:gosec // goroutine count fits int64
				}
			case rtHeapObjects:
				if s.Value.Kind() == metrics.KindUint64 {
					o.ObserveInt64(heap, int64(s.Value.Uint64())) //nolint:gosec // bytes fit int64
				}
			case rtMutexWait:
				if s.Value.Kind() == metrics.KindFloat64 {
					o.ObserveFloat64(mutexWait, s.Value.Float64())
				}
			case rtSchedLatency:
				if s.Value.Kind() == metrics.KindFloat64Histogram {
					h := s.Value.Float64Histogram()
					if v, ok := sched.quantilesSince(h, 0.5, 0.99); ok {
						o.ObserveFloat64(schedP50, v[0])
						o.ObserveFloat64(schedP99, v[1])
					}
				}
			case rtGCPauses:
				if s.Value.Kind() == metrics.KindFloat64Histogram {
					if v, ok := gc.quantilesSince(s.Value.Float64Histogram(), 0.99); ok {
						o.ObserveFloat64(gcP99, v[0])
					}
				}
			}
		}
		return nil
	}, goroutines, schedP99, schedP50, gcP99, heap, mutexWait)
	return err
}
