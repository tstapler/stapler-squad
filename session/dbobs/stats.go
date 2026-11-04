package dbobs

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	regMu       sync.Mutex
	registered  = map[*collector]struct{}{}
	metricsOnce sync.Once
	histogram   metric.Float64Histogram
	waitHist    metric.Float64Histogram
	busyCounter metric.Int64Counter
)

func register(c *collector) {
	metricsOnce.Do(initMetrics)
	regMu.Lock()
	registered[c] = struct{}{}
	regMu.Unlock()
}

func unregister(c *collector) {
	regMu.Lock()
	delete(registered, c)
	regMu.Unlock()
}

// Stats is one pool's snapshot: database/sql.DBStats plus query counters.
type Stats struct {
	Name            string  `json:"name"`
	OpenConnections int     `json:"open_connections"`
	InUse           int     `json:"in_use"`
	Idle            int     `json:"idle"`
	MaxOpen         int     `json:"max_open"`
	WaitCount       int64   `json:"wait_count"`
	WaitDurationMs  float64 `json:"wait_duration_ms"`
	Queries         int64   `json:"queries_total"`
	SlowQueries     int64   `json:"slow_queries_total"`
	Errors          int64   `json:"errors_total"`
	BusyErrors      int64   `json:"busy_errors_total"`
	TotalDurationMs float64 `json:"total_duration_ms"`
	MaxDurationMs   float64 `json:"max_duration_ms"`
	SlowThresholdMs int64   `json:"slow_threshold_ms"`
	// TopStatements ranks (statement, caller) pairs by total time, so write
	// amplification from one caller is visible without a profiler.
	TopStatements []StatementStat `json:"top_statements,omitempty"`
	// DroppedStatementKeys counts distinct (statement, caller) pairs the bounded
	// table refused; non-zero means TopStatements is missing late-arriving callers.
	DroppedStatementKeys int64 `json:"dropped_statement_keys,omitempty"`
}

func ms(nanos int64) float64 { return float64(nanos) / float64(time.Millisecond) }

func (c *collector) snapshot() Stats {
	p := c.db.Stats()
	return Stats{
		Name: c.cfg.Name, OpenConnections: p.OpenConnections, InUse: p.InUse, Idle: p.Idle,
		MaxOpen: p.MaxOpenConnections, WaitCount: p.WaitCount,
		WaitDurationMs: ms(int64(p.WaitDuration)),
		Queries:        c.stat.queries.Load(), SlowQueries: c.stat.slow.Load(), Errors: c.stat.errors.Load(), BusyErrors: c.stat.busy.Load(),
		TotalDurationMs: ms(c.stat.totalNanos.Load()), MaxDurationMs: ms(c.stat.maxNanos.Load()),
		SlowThresholdMs: c.cfg.SlowThreshold.Milliseconds(),
		TopStatements:   c.stmts.top(topStatementsReported),

		DroppedStatementKeys: c.stmts.droppedKeys(),
	}
}

// Snapshot returns stats for every live instrumented driver, for /actuator/db.
func Snapshot() []Stats {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]Stats, 0, len(registered))
	for c := range registered {
		out = append(out, c.snapshot())
	}
	return out
}

func nameAttr(name string) attribute.KeyValue { return attribute.String("db.name", name) }

func recordHistogram(name, caller string, d time.Duration) {
	if histogram != nil {
		histogram.Record(context.Background(), float64(d)/float64(time.Millisecond),
			metric.WithAttributes(nameAttr(name), callerAttr(caller)))
	}
}

// recordPoolWait records the connection-queue wait attributed to a query that had
// to wait (zero waits are skipped). It is a diff of the pool-wide WaitDuration,
// so concurrent queries over-attribute each other's wait. Kept next to the
// cumulative db.pool.wait_* gauges because only a histogram gives p99 per query.
func recordPoolWait(name, caller string, d time.Duration) {
	if waitHist != nil && d > 0 {
		waitHist.Record(context.Background(), float64(d)/float64(time.Millisecond),
			metric.WithAttributes(nameAttr(name), callerAttr(caller)))
	}
}

func recordBusy(name string) {
	if busyCounter != nil {
		busyCounter.Add(context.Background(), 1, metric.WithAttributes(nameAttr(name)))
	}
}

// initMetrics registers OTel instruments against the global (delegating) meter,
// so they export once telemetry.Initialize installs a real provider.
func initMetrics() {
	meter := telemetry.GetMeter()
	histogram, _ = meter.Float64Histogram("db.query.duration",
		metric.WithDescription("Duration of ent/SQLite queries"), metric.WithUnit("ms"))
	waitHist, _ = meter.Float64Histogram("db.query.pool_wait",
		metric.WithDescription("Connection-queue wait attributed to queries that had to wait (pool-wide, over-attributed under concurrency)"),
		metric.WithUnit("ms"))
	busyCounter, _ = meter.Int64Counter("db.busy_errors_total",
		metric.WithDescription("Queries that failed with SQLITE_BUSY or SQLITE_LOCKED"))
	gauges := newPoolGauges(meter)
	if gauges == nil {
		return
	}
	_, _ = meter.RegisterCallback(gauges.observe, gauges.instruments()...)
}
