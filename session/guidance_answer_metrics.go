package session

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
	"github.com/tstapler/stapler-squad/telemetry"
)

// guidance_question_answer_latency_seconds{source,route} and
// guidance_answer_delivery_total{route,state} (plan Observability). The OTel
// meter is a no-op until telemetry is initialised, so each series is also
// mirrored in-process for tests and the stats views.
const (
	MetricGuidanceAnswerLatency  = "guidance_question_answer_latency_seconds"
	MetricGuidanceAnswerDelivery = "guidance_answer_delivery_total"
)

var guidanceMetrics = newGuidanceMetrics()

type (
	latencyKey  struct{ source, route string }
	deliveryKey struct{ route, state string }
)

type guidanceMetricsMirror struct {
	mu        sync.Mutex
	latencies map[latencyKey]int
	delivery  map[deliveryKey]uint64
	latency   metric.Float64Histogram
	deliverC  metric.Int64Counter
}

func newGuidanceMetrics() *guidanceMetricsMirror {
	m := &guidanceMetricsMirror{latencies: map[latencyKey]int{}, delivery: map[deliveryKey]uint64{}}
	meter := telemetry.GetMeter()
	if h, err := meter.Float64Histogram(MetricGuidanceAnswerLatency, metric.WithUnit("s")); err == nil {
		m.latency = h
	}
	if c, err := meter.Int64Counter(MetricGuidanceAnswerDelivery); err == nil {
		m.deliverC = c
	}
	return m
}

// GuidanceAnswerLatencyCount is how many answers were observed for source and route.
func GuidanceAnswerLatencyCount(source domain.QuestionSource, route domain.DeliveryRoute) int {
	guidanceMetrics.mu.Lock()
	defer guidanceMetrics.mu.Unlock()
	return guidanceMetrics.latencies[latencyKey{string(source), string(route)}]
}

// GuidanceAnswerDeliveryTotal is the in-process guidance_answer_delivery_total{route,state}.
func GuidanceAnswerDeliveryTotal(route domain.DeliveryRoute, state domain.DeliveryState) uint64 {
	guidanceMetrics.mu.Lock()
	defer guidanceMetrics.mu.Unlock()
	return guidanceMetrics.delivery[deliveryKey{string(route), string(state)}]
}

// observeGuidanceAnswer records created_at to answered_at and returns it in
// milliseconds (the answer_latency_ms log field).
func observeGuidanceAnswer(ctx context.Context, row *ent.GuidanceRequest) int64 {
	source, route := domain.QuestionSource(row.Source), domain.DeliveryRoute(row.DeliveryRoute)
	latency := max(row.AnsweredAt.Sub(row.CreatedAt), 0)
	m := guidanceMetrics
	m.mu.Lock()
	m.latencies[latencyKey{string(source), string(route)}]++
	m.mu.Unlock()
	if m.latency != nil {
		m.latency.Record(ctx, latency.Seconds(), metric.WithAttributes(attrSource(source), attrRoute(route)))
	}
	ms := latency.Milliseconds()
	log.Info("[GuidanceRequest] answered", "question_id", row.ID.String(), "source", source, "route", route, "answer_latency_ms", ms)
	return ms
}

func attrSource(s domain.QuestionSource) attribute.KeyValue {
	return attribute.String("source", string(s))
}
func attrRoute(r domain.DeliveryRoute) attribute.KeyValue {
	return attribute.String("route", string(r))
}

func countGuidanceDelivery(ctx context.Context, route domain.DeliveryRoute, state domain.DeliveryState) {
	m := guidanceMetrics
	m.mu.Lock()
	m.delivery[deliveryKey{string(route), string(state)}]++
	m.mu.Unlock()
	if m.deliverC != nil {
		m.deliverC.Add(ctx, 1, metric.WithAttributes(attrRoute(route), attribute.String("state", string(state))))
	}
}

// recordGuidanceAnswered observes the latency of a just-applied answer. It is
// best effort: an unreadable row only skips the observation.
func (r *EntRepository) recordGuidanceAnswered(ctx context.Context, id uuid.UUID) {
	row, err := r.client.GuidanceRequest.Query().Where(guidancerequest.ID(id)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			log.Warn("[GuidanceRequest] answered but latency not observed", "id", id, "err", err)
		}
		return
	}
	if row.AnsweredAt == nil {
		return
	}
	observeGuidanceAnswer(ctx, row)
}
