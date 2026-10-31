package telemetry

import (
	"context"
	"runtime/metrics"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

// mutexWaitSample is the runtime's cumulative time goroutines spent blocked on
// sync.Mutex/RWMutex. Unlike the mutex pprof profile it is always on and costs
// nothing, so it shows lock-contention trends even when profiling is off.
const mutexWaitSample = "/sync/mutex/wait/total:seconds"

var runtimeContentionOnce sync.Once

func init() {
	runtimeContentionOnce.Do(func() {
		if err := RegisterRuntimeContentionMetrics(GetMeter()); err != nil {
			log.Error("telemetry: failed to register runtime contention metrics", "error", err)
		}
	})
}

// RegisterRuntimeContentionMetrics exposes go.runtime.mutex_wait (seconds,
// cumulative) on meter. Query its rate() to see how much goroutine time per
// second is lost to lock waits.
func RegisterRuntimeContentionMetrics(meter metric.Meter) error {
	_, err := meter.Float64ObservableCounter("go.runtime.mutex_wait",
		metric.WithDescription("Cumulative seconds goroutines spent blocked on sync.Mutex/RWMutex (runtime/metrics "+mutexWaitSample+")"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(readMutexWaitSeconds())
			return nil
		}))
	return err
}

func readMutexWaitSeconds() float64 {
	s := []metrics.Sample{{Name: mutexWaitSample}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return s[0].Value.Float64()
}
