package telemetry

// goruntime.go exports the Go runtime signals that tell "handler is slow" apart
// from "runtime could not run the handler": goroutine count, scheduler run-queue
// latency, GC pause and heap size. Latency quantiles cover the window since the
// previous collection (the cumulative runtime histograms would hide a stall
// inside hours of healthy history).

import (
	"context"
	"math"
	"runtime"
	"runtime/metrics"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

const (
	metricGoroutines   = "/sched/goroutines:goroutines"
	metricSchedLatency = "/sched/latencies:seconds"
	metricGCPauses     = "/gc/pauses:seconds"
	metricHeapObjects  = "/memory/classes/heap/objects:bytes"
	metricMemoryTotal  = "/memory/classes/total:bytes"
)

var goRuntimeQuantiles = []float64{0.5, 0.99}

type goRuntimeCollector struct {
	mu        sync.Mutex
	samples   []metrics.Sample
	prevSched *metrics.Float64Histogram
	prevGC    *metrics.Float64Histogram
}

var goRuntimeOnce sync.Once

func init() {
	goRuntimeOnce.Do(func() {
		if err := registerGoRuntimeMetrics(GetMeter()); err != nil {
			log.Error("telemetry: failed to register Go runtime metrics", "error", err)
		}
	})
}

func registerGoRuntimeMetrics(meter metric.Meter) error {
	c := &goRuntimeCollector{samples: []metrics.Sample{
		{Name: metricGoroutines}, {Name: metricSchedLatency}, {Name: metricGCPauses},
		{Name: metricHeapObjects}, {Name: metricMemoryTotal},
	}}
	goroutines, err := meter.Int64ObservableGauge("go.runtime.goroutines",
		metric.WithDescription("Live goroutines"))
	if err != nil {
		return err
	}
	procs, err := meter.Int64ObservableGauge("go.runtime.gomaxprocs",
		metric.WithDescription("GOMAXPROCS"))
	if err != nil {
		return err
	}
	sched, err := meter.Float64ObservableGauge("go.runtime.sched_latency_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Time goroutines spent runnable before running, by quantile over the last collection interval"))
	if err != nil {
		return err
	}
	gcPause, err := meter.Float64ObservableGauge("go.runtime.gc_pause_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Stop-the-world GC pause, by quantile over the last collection interval"))
	if err != nil {
		return err
	}
	heap, err := meter.Int64ObservableGauge("go.runtime.memory_bytes",
		metric.WithUnit("By"),
		metric.WithDescription("Go runtime memory: kind=heap_objects (live objects) or total (mapped by the runtime)"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		c.observe(o, goroutines, procs, sched, gcPause, heap)
		return nil
	}, goroutines, procs, sched, gcPause, heap)
	return err
}

func (c *goRuntimeCollector) observe(o metric.Observer, goroutines, procs metric.Int64Observable,
	sched, gcPause metric.Float64Observable, heap metric.Int64Observable) {
	c.mu.Lock()
	defer c.mu.Unlock()
	metrics.Read(c.samples)
	if v := c.samples[0].Value; v.Kind() == metrics.KindUint64 {
		o.ObserveInt64(goroutines, int64(min(v.Uint64(), math.MaxInt64))) // #nosec G115 -- clamped
	}
	o.ObserveInt64(procs, int64(runtime.GOMAXPROCS(0)))
	c.prevSched = observeQuantiles(o, sched, c.samples[1].Value, c.prevSched)
	c.prevGC = observeQuantiles(o, gcPause, c.samples[2].Value, c.prevGC)
	for i, kind := range []string{"heap_objects", "total"} {
		if v := c.samples[3+i].Value; v.Kind() == metrics.KindUint64 {
			o.ObserveInt64(heap, int64(min(v.Uint64(), math.MaxInt64)), // #nosec G115 -- clamped
				metric.WithAttributes(attribute.String("kind", kind)))
		}
	}
}

func observeQuantiles(o metric.Observer, g metric.Float64Observable, v metrics.Value, prev *metrics.Float64Histogram) *metrics.Float64Histogram {
	if v.Kind() != metrics.KindFloat64Histogram {
		return prev
	}
	cur := cloneHistogram(v.Float64Histogram())
	for _, q := range goRuntimeQuantiles {
		if val, ok := deltaQuantile(prev, cur, q); ok {
			o.ObserveFloat64(g, val, metric.WithAttributes(attribute.Float64("quantile", q)))
		}
	}
	return cur
}

func cloneHistogram(h *metrics.Float64Histogram) *metrics.Float64Histogram {
	return &metrics.Float64Histogram{
		Counts:  append([]uint64(nil), h.Counts...),
		Buckets: h.Buckets, // immutable per runtime docs
	}
}

// deltaQuantile returns the q-quantile (upper bucket bound) of the samples
// recorded between prev and cur; ok is false when the window is empty. A nil
// prev means "since process start".
func deltaQuantile(prev, cur *metrics.Float64Histogram, q float64) (float64, bool) {
	delta := make([]uint64, len(cur.Counts))
	var total uint64
	for i, n := range cur.Counts {
		if prev != nil && i < len(prev.Counts) {
			n -= min(n, prev.Counts[i])
		}
		delta[i], total = n, total+n
	}
	if total == 0 {
		return 0, false
	}
	target := uint64(math.Ceil(q * float64(total)))
	var run uint64
	for i, n := range delta {
		if run += n; run >= target {
			upper := cur.Buckets[i+1]
			if math.IsInf(upper, 1) {
				upper = cur.Buckets[i] // overflow bucket: report its lower bound
			}
			return upper, true
		}
	}
	return 0, false
}
