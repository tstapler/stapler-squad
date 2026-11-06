package unfinished

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// slowOpThreshold is the duration above which an operation counts as slow in
// the actuator and the over_100ms counters.
const slowOpThreshold = 100 * time.Millisecond

// opDuration records one operation's wall time into an OTel histogram (once
// registered) and into atomics the actuator reads without OpenTelemetry.
type opDuration struct {
	hist  atomic.Pointer[metric.Float64Histogram]
	count atomic.Int64
	slow  atomic.Int64 // observations > slowOpThreshold
	sumNs atomic.Int64
}

// OpDurationStats is a snapshot of an opDuration.
type OpDurationStats struct {
	Count, SlowOver100ms int64
	Total                time.Duration
}

func (o *opDuration) record(d time.Duration) {
	o.count.Add(1)
	o.sumNs.Add(int64(d))
	if d > slowOpThreshold {
		o.slow.Add(1)
	}
	if h := o.hist.Load(); h != nil {
		(*h).Record(context.Background(), d.Seconds())
	}
}

func (o *opDuration) stats() OpDurationStats {
	return OpDurationStats{Count: o.count.Load(), SlowOver100ms: o.slow.Load(), Total: time.Duration(o.sumNs.Load())}
}

// register creates the histogram on meter under the repostate prefix.
func (o *opDuration) register(meter metric.Meter, name, desc string) error {
	h, err := meter.Float64Histogram(repostateMetricPrefix+name, metric.WithDescription(desc), metric.WithUnit("s"))
	if err != nil {
		return err
	}
	o.hist.Store(&h)
	return nil
}

// graphExtensionDuration times walks that decoded at least one commit, that is
// walks that extended the shared graph (Observability Plan: operations >100 ms).
// No first-tree-map-build histogram exists: the shared tree maps were not built.
var graphExtensionDuration opDuration

// GraphExtensionDurations returns the graph-extension timing snapshot.
func GraphExtensionDurations() OpDurationStats { return graphExtensionDuration.stats() }

// done records the walk's duration when it extended the graph; call it once,
// deferred, after the walk finishes.
func (s *graphSource) done() {
	if s.decodes > 0 {
		graphExtensionDuration.record(time.Since(s.born))
	}
}
