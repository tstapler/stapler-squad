package wakes

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Metric names from the plan's Observability list.
const (
	MetricTrigger         = "wake_trigger_total"            // type,state
	MetricReconcile       = "wake_reconcile_total"          // result
	MetricLimitHit        = "wake_limit_hit_total"          // limit,phase
	MetricDuplicate       = "wake_duplicate_delivery_total" // (none)
	MetricReconcileDigest = "wake_reconcile_digest_total"   // (none)

	MetricIdentityUnverified = "mcp_identity_unverified_total" // tool
	MetricCallerGateDenied   = "mcp_caller_gate_denied_total"  // tool,role
	MetricPeerHelp           = "peer_help_total"               // state

	GaugeNeedsReviveAge = "wake_needs_revive_age_seconds" // oldest needs_revive row

	HistogramFireLag = "wake_fire_lag_seconds" // type: claim time minus next_fire_at
)

// Reconcile result label values.
const (
	ReconcileRequeuedClaimed = "requeued_claimed"
	ReconcileFired           = "fired"
	ReconcileMissed          = "missed"
	ReconcileNeedsRevive     = "needs_revive"
)

var metricLabelKeys = map[string][]string{
	MetricTrigger:         {"type", "state"},
	MetricReconcile:       {"result"},
	MetricLimitHit:        {"limit", "phase"},
	MetricDuplicate:       nil,
	MetricReconcileDigest: nil,

	MetricIdentityUnverified: {"tool"},
	MetricCallerGateDenied:   {"tool", "role"},
	MetricPeerHelp:           {"state"},
}

// Metrics is an in-process counter mirror that also feeds OTel (a no-op when
// telemetry is not initialised). The zero value is not usable; use NewMetrics.
// A nil *Metrics ignores every call.
type Metrics struct {
	mu     sync.Mutex
	counts map[string]int64
	otel   map[string]metric.Int64Counter
	gauges map[string]float64
	gauge  map[string]metric.Float64Gauge
	lag    metric.Float64Histogram
	lags   map[string][]float64 // trigger type -> observed lags in seconds
	fires  sessionFires
}

// NewMetrics returns an empty mirror.
func NewMetrics() *Metrics {
	return &Metrics{counts: map[string]int64{}, otel: map[string]metric.Int64Counter{}, gauges: map[string]float64{}, gauge: map[string]metric.Float64Gauge{}, lags: map[string][]float64{}}
}

func seriesKey(name string, labels []string) string {
	return name + "|" + strings.Join(labels, "|")
}

// Inc adds one to name for the given label values (in metricLabelKeys order).
func (m *Metrics) Inc(name string, labels ...string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.counts[seriesKey(name, labels)]++
	inst := m.instrumentLocked(name)
	m.mu.Unlock()
	if inst == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, len(labels))
	for i, l := range labels {
		if keys := metricLabelKeys[name]; i < len(keys) {
			attrs = append(attrs, attribute.String(keys[i], l))
		}
	}
	inst.Add(context.Background(), 1, metric.WithAttributes(attrs...))
}

func (m *Metrics) instrumentLocked(name string) metric.Int64Counter {
	if c, ok := m.otel[name]; ok {
		return c
	}
	c, err := telemetry.GetMeter().Int64Counter(name)
	if err != nil {
		c = nil
	}
	m.otel[name] = c
	return c
}

// Value returns the current count for name and label values.
func (m *Metrics) Value(name string, labels ...string) int64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counts[seriesKey(name, labels)]
}

// SetGauge records the current value of a gauge.
func (m *Metrics) SetGauge(name string, v float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.gauges[name] = v
	inst, ok := m.gauge[name]
	if !ok {
		g, err := telemetry.GetMeter().Float64Gauge(name)
		if err == nil {
			inst = g
		}
		m.gauge[name] = inst
	}
	m.mu.Unlock()
	if inst != nil {
		inst.Record(context.Background(), v)
	}
}

// Gauge returns the last value set for name.
func (m *Metrics) Gauge(name string) float64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gauges[name]
}

// ObserveFireLag records how late a due trigger of triggerType was claimed.
func (m *Metrics) ObserveFireLag(triggerType string, lag time.Duration) {
	if m == nil {
		return
	}
	seconds := max(lag, 0).Seconds()
	m.mu.Lock()
	m.lags[triggerType] = append(m.lags[triggerType], seconds)
	if m.lag == nil {
		if h, err := telemetry.GetMeter().Float64Histogram(HistogramFireLag, metric.WithUnit("s")); err == nil {
			m.lag = h
		}
	}
	h := m.lag
	m.mu.Unlock()
	if h != nil {
		h.Record(context.Background(), seconds, metric.WithAttributes(attribute.String("type", triggerType)))
	}
}

// FireLags returns the lags (seconds) observed for triggerType, oldest first.
func (m *Metrics) FireLags(triggerType string) []float64 {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]float64(nil), m.lags[triggerType]...)
}
