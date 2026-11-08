package escapedb

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	writerMu          sync.Mutex
	runningWriters    = map[*EscapeEventBatchWriter]struct{}{}
	writerMetricsOnce sync.Once
)

func registerWriterMetrics(w *EscapeEventBatchWriter) {
	writerMetricsOnce.Do(initWriterMetrics)
	writerMu.Lock()
	runningWriters[w] = struct{}{}
	writerMu.Unlock()
}

func unregisterWriterMetrics(w *EscapeEventBatchWriter) {
	writerMu.Lock()
	delete(runningWriters, w)
	writerMu.Unlock()
}

// initWriterMetrics registers observable gauges against the global (delegating) meter.
func initWriterMetrics() {
	meter := telemetry.GetMeter()
	var failed bool
	gauge := func(name, desc string) metric.Float64ObservableGauge {
		g, err := meter.Float64ObservableGauge(name, metric.WithDescription(desc))
		failed = failed || err != nil
		return g
	}
	depth := gauge("escape_events.queue_depth", "Escape events waiting in the writer queue")
	dropped := gauge("escape_events.dropped_total", "Escape events dropped because the queue was full")
	written := gauge("escape_events.rows_written_total", "Escape event rows written")
	failedRows := gauge("escape_events.failed_rows_total", "Escape event rows lost to failed flushes")
	flushes := gauge("escape_events.flushes_total", "Successful escape event flush transactions")
	lastFlush := gauge("escape_events.last_flush_seconds", "Duration of the last flush transaction")
	maxFlush := gauge("escape_events.max_flush_seconds", "Longest flush transaction since start")
	if failed {
		return
	}
	_, _ = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		writerMu.Lock()
		defer writerMu.Unlock()
		for w := range runningWriters {
			s := w.Stats()
			o.ObserveFloat64(depth, float64(s.QueueDepth))
			o.ObserveFloat64(dropped, float64(s.Dropped))
			o.ObserveFloat64(written, float64(s.RowsWritten))
			o.ObserveFloat64(failedRows, float64(s.FailedRows))
			o.ObserveFloat64(flushes, float64(s.Flushes))
			o.ObserveFloat64(lastFlush, s.LastFlush.Seconds())
			o.ObserveFloat64(maxFlush, s.MaxFlush.Seconds())
		}
		return nil
	}, depth, dropped, written, failedRows, flushes, lastFlush, maxFlush)
}
