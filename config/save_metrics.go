package config

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// config.json saves block flag flips (and every other settings write) on an
// fsync that waits for all other disk writers; these histograms make that wait
// visible next to the db.* metrics.
var saveMetrics struct { //nolint:gochecknoglobals // process-wide OTel instruments
	once      sync.Once
	duration  metric.Float64Histogram
	fsyncWait metric.Float64Histogram
}

func initSaveMetrics() {
	m := telemetry.GetMeter()
	saveMetrics.duration, _ = m.Float64Histogram("config.save.duration",
		metric.WithDescription("Duration of one config.json save (marshal, write, fsync, rename)"), metric.WithUnit("ms"))
	saveMetrics.fsyncWait, _ = m.Float64Histogram("config.save.fsync.duration",
		metric.WithDescription("Duration of the fsync inside one config.json save"), metric.WithUnit("ms"))
}

func recordSave(total, fsync time.Duration) {
	saveMetrics.once.Do(initSaveMetrics)
	ctx := context.Background()
	if saveMetrics.duration != nil {
		saveMetrics.duration.Record(ctx, float64(total)/float64(time.Millisecond))
	}
	if saveMetrics.fsyncWait != nil {
		saveMetrics.fsyncWait.Record(ctx, float64(fsync)/float64(time.Millisecond))
	}
}
