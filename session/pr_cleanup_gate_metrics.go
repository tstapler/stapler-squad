package session

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Cleanup-gate metric names (observability plan, T5.4.2).
const (
	MetricCleanupGateBlockedTotal = "ssq_cleanup_gate_blocked_total"
	MetricCleanupGateBlockedItems = "ssq_cleanup_gate_blocked_items"
)

type cleanupGateMetricSet struct {
	totals *counterByLabel
	otel   metric.Int64Counter
	gauge  metric.Int64Gauge

	mu    sync.Mutex
	items map[string]int
}

var cleanupGateMetrics = newCleanupGateMetricSet()

func newCleanupGateMetricSet() *cleanupGateMetricSet {
	m := &cleanupGateMetricSet{totals: newCounterByLabel(), items: map[string]int{}}
	meter := telemetry.GetMeter()
	m.otel, _ = meter.Int64Counter(MetricCleanupGateBlockedTotal)
	m.gauge, _ = meter.Int64Gauge(MetricCleanupGateBlockedItems)
	return m
}

// RecordCleanupBlocked counts one newly blocked (path, item) pair.
func RecordCleanupBlocked(path string) {
	cleanupGateMetrics.totals.Add(path)
	if cleanupGateMetrics.otel != nil {
		cleanupGateMetrics.otel.Add(context.Background(), 1, metric.WithAttributes(attribute.String("path", path)))
	}
}

// SetCleanupBlockedItems records the gauge: how many items are blocked on path now.
func SetCleanupBlockedItems(path string, n int) {
	cleanupGateMetrics.mu.Lock()
	cleanupGateMetrics.items[path] = n
	cleanupGateMetrics.mu.Unlock()
	if cleanupGateMetrics.gauge != nil {
		cleanupGateMetrics.gauge.Record(context.Background(), int64(n), metric.WithAttributes(attribute.String("path", path)))
	}
}

// CleanupBlockedCount is ssq_cleanup_gate_blocked_total{path} in this process.
func CleanupBlockedCount(path string) int64 { return cleanupGateMetrics.totals.Get(path) }

// CleanupBlockedItemsGauge reads ssq_cleanup_gate_blocked_items{path} back.
func CleanupBlockedItemsGauge(path string) int {
	cleanupGateMetrics.mu.Lock()
	defer cleanupGateMetrics.mu.Unlock()
	return cleanupGateMetrics.items[path]
}
