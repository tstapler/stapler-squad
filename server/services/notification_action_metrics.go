package services

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// ActionMetrics records one finished InvokeNotificationAction call. The label
// values are the lower-cased ActionKind and ActionOutcome names plus the caller
// channel (web_local, web_remote, mcp, unknown).
type ActionMetrics interface {
	Record(ActionSample)
}

// ActionSample is one finished call.
type ActionSample struct {
	Kind, Outcome, Channel string
	Elapsed                time.Duration
}

type otelActionMetrics struct {
	total   metric.Int64Counter
	latency metric.Float64Histogram
}

var (
	otelActionMetricsOnce sync.Once
	otelActionMetricsInst *otelActionMetrics
)

// defaultActionMetrics returns the process-wide OTel-backed recorder:
// notification_inline_action_total{kind,outcome,channel} and
// notification_inline_action_latency_ms{kind}. A registration failure degrades
// to a no-op recorder; metrics never fail an action.
func defaultActionMetrics() ActionMetrics {
	otelActionMetricsOnce.Do(func() {
		meter := telemetry.GetMeter()
		total, err := meter.Int64Counter("notification_inline_action_total",
			metric.WithDescription("InvokeNotificationAction calls by action kind, outcome and caller channel"))
		if err != nil {
			log.Error("services: failed to register notification_inline_action_total", "error", err)
			return
		}
		latency, err := meter.Float64Histogram("notification_inline_action_latency_ms",
			metric.WithUnit("ms"),
			metric.WithDescription("InvokeNotificationAction handler latency by action kind"))
		if err != nil {
			log.Error("services: failed to register notification_inline_action_latency_ms", "error", err)
			return
		}
		otelActionMetricsInst = &otelActionMetrics{total: total, latency: latency}
	})
	if otelActionMetricsInst == nil {
		return noopActionMetrics{}
	}
	return otelActionMetricsInst
}

func (m *otelActionMetrics) Record(a ActionSample) {
	ctx := context.Background()
	m.total.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", a.Kind), attribute.String("outcome", a.Outcome), attribute.String("channel", a.Channel)))
	m.latency.Record(ctx, float64(a.Elapsed.Microseconds())/1000, metric.WithAttributes(attribute.String("kind", a.Kind)))
}

type noopActionMetrics struct{}

func (noopActionMetrics) Record(ActionSample) {}
