package analytics

import (
	"context"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// EventInlineActionUndo is the client event sent when the operator presses
// Undo during an inline action's commit delay; the server never sees that Undo.
const EventInlineActionUndo = "inline_action_undo"

// undoKinds are the action kinds that hold behind Undo; anything else is
// reported as "unknown" so a client cannot mint label values.
var undoKinds = map[string]bool{"approve": true, "deny": true, "answer_question": true, "backlog_decision": true}

var undoCounter = sync.OnceValue(func() metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter("inline_action_undo_total",
		metric.WithDescription("Inline actions cancelled with Undo during the commit delay, by action kind"))
	if err != nil {
		log.Error("analytics: failed to register inline_action_undo_total", "error", err)
		return nil
	}
	return c
})

// qualityMetricsProvider counts allow-listed client events into OTel counters
// before handing every event to the wrapped provider.
type qualityMetricsProvider struct{ AnalyticsProvider }

// WithQualityMetrics wraps p so client analytics events that back a quality
// metric also increment it. A nil p is returned unchanged.
func WithQualityMetrics(p AnalyticsProvider) AnalyticsProvider {
	if p == nil {
		return nil
	}
	return qualityMetricsProvider{p}
}

// RecordBatch applies the per-event metric side effects, then stores the events
// that are not metrics-only in one batch.
func (q qualityMetricsProvider) RecordBatch(ctx context.Context, events []Event) error {
	stored := make([]Event, 0, len(events))
	for _, ev := range events {
		if q.observeOnly(ctx, ev) {
			continue
		}
		stored = append(stored, ev)
	}
	return RecordAll(ctx, q.AnalyticsProvider, stored)
}

func (q qualityMetricsProvider) Record(ctx context.Context, ev Event) error {
	if q.observeOnly(ctx, ev) {
		return nil
	}
	return q.AnalyticsProvider.Record(ctx, ev)
}

// observeOnly counts ev into its metric and reports whether ev is metrics-only
// (a browser event that is not stored).
func (q qualityMetricsProvider) observeOnly(ctx context.Context, ev Event) bool {
	if defaultClientMetrics().observe(ctx, ev) {
		return true
	}
	if ev.EventName == EventInlineActionUndo {
		if c := undoCounter(); c != nil {
			kind := strings.ToLower(ev.Labels["kind"])
			if !undoKinds[kind] {
				kind = "unknown"
			}
			c.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind)))
		}
	}
	return false
}
