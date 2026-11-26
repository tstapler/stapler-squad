package searchrpc

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Metric names of the cross-session artifact search (observability plan, T9.2.1).
const (
	MetricArtifactSearchSeconds = "ssq_artifact_search_seconds"
	MetricArtifactSearchQueries = "ssq_artifact_search_queries_total"
)

type ArtifactSearchMetrics struct {
	seconds metric.Float64Histogram
	queries metric.Int64Counter

	mu     sync.Mutex
	counts map[[2]string]int64 // in-process tally so tests need no exporter
}

var ArtifactSearchStats = func() *ArtifactSearchMetrics {
	m := &ArtifactSearchMetrics{counts: map[[2]string]int64{}}
	meter := telemetry.GetMeter()
	m.seconds, _ = meter.Float64Histogram(MetricArtifactSearchSeconds, metric.WithUnit("s"))
	m.queries, _ = meter.Int64Counter(MetricArtifactSearchQueries)
	return m
}()

// Observe records one finished query: kind is the query shape (text, pr, sha,
// branch), hit says whether it returned any session.
func (m *ArtifactSearchMetrics) Observe(kind string, hit bool, took time.Duration) {
	result := "miss"
	if hit {
		result = "hit"
	}
	m.mu.Lock()
	m.counts[[2]string{kind, result}]++
	m.mu.Unlock()
	ctx := context.Background()
	if m.seconds != nil {
		m.seconds.Record(ctx, took.Seconds(), metric.WithAttributes(attribute.String("kind", kind)))
	}
	if m.queries != nil {
		m.queries.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind), attribute.String("result", result)))
	}
}

// ArtifactSearchQueryCount is ssq_artifact_search_queries_total{kind,result} in this process.
func ArtifactSearchQueryCount(kind, result string) int64 {
	ArtifactSearchStats.mu.Lock()
	defer ArtifactSearchStats.mu.Unlock()
	return ArtifactSearchStats.counts[[2]string{kind, result}]
}
