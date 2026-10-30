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

func (q qualityMetricsProvider) Record(ctx context.Context, ev Event) error {
	if defaultClientMetrics().observe(ctx, ev) {
		return nil // metrics-only browser event; not stored
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
	return q.AnalyticsProvider.Record(ctx, ev)
}
