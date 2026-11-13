package backlogflow

import (
	"context"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/flow"
	"github.com/tstapler/stapler-squad/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	metricFlowAggregateDuration = "flow.aggregate.duration_ms"
	metricFlowCacheHit          = "flow.cache.hit"
)

var (
	flowDurationHist = sync.OnceValue(func() metric.Int64Histogram {
		h, err := telemetry.GetMeter().Int64Histogram(metricFlowAggregateDuration,
			metric.WithUnit("ms"), metric.WithDescription("Uncached flow summary compute time"))
		if err != nil {
			log.Error("services: failed to register "+metricFlowAggregateDuration, "error", err)
			return nil
		}
		return h
	})
	flowCacheCounter = telemetry.CounterNamed(metricFlowCacheHit, "Flow summary lookups by cache outcome (the aggregator reports computes, so only misses are observed)")
)

// NewFlowObserver returns the flow.Observer that logs one debug line and
// records the compute duration and cache outcome per uncached compute. It
// never sees item titles: ComputeStats carries counts and timings only.
func NewFlowObserver() flow.Observer {
	return func(cs flow.ComputeStats) {
		log.Debug("flow aggregate", "window_days", cs.Window.Days, "all_time", cs.Window.AllTime,
			"rows", cs.Rows, "query_ms", cs.QueryMs, "cache", cs.Cache, "failed_sections", len(cs.Failures))
		if h := flowDurationHist(); h != nil {
			h.Record(context.Background(), cs.QueryMs)
		}
		telemetry.AddOne(flowCacheCounter(), attribute.String("cache", cs.Cache))
	}
}
