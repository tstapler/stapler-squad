package session

// pi_status_source_metrics.go implements Epic 6.1 Story 6.1.1's
// pi_status_source_events_total counter, following the existing
// telemetry.GetMeter() registration idiom already used by
// executor/safeexec/safeexec_metrics.go and
// server/services/session_creation_metrics.go.

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// piEventTypeUnrecognized is the fixed "type" attribute value recorded for
// any event whose JSON "type" discriminator isn't one of pi_adapter.go's
// known event structs. Every unrecognized type is bucketed under this one
// fixed label rather than the raw, pi-CLI-controlled string, to keep this
// metric's cardinality bounded -- Story 6.1.1's AC only requires that an
// unrecognized type increments *some* labeled bucket, not a per-value one.
const piEventTypeUnrecognized = "unrecognized"

// piStatusSourceEventsTotal is pi_status_source_events_total{type}: a
// counter of every event PiStatusSource.readLoop has decoded (or failed to
// recognize) from a `pi --mode json` subprocess's stdout, incremented once
// per event.
//
// Label scheme: "type" is the event's own JSON `type` field (e.g.
// "agent_start", "tool_execution_end") for a recognized event, or the fixed
// value piEventTypeUnrecognized ("unrecognized") for a line whose `type`
// discriminator pi_adapter.go's PiEventReader doesn't decode.
//
// Feeds plan.md's Observability Plan / PITFALL-3 early-warning use case: a
// sudden rise in the "unrecognized" bucket signals pi's event vocabulary has
// drifted from what pi_adapter.go decodes, worth investigating before it
// silently degrades PiStatusSource's status inference.
var piStatusSourceEventsTotal = mustPiStatusSourceEventsCounter()

func mustPiStatusSourceEventsCounter() metric.Int64Counter {
	counter, err := telemetry.GetMeter().Int64Counter("pi_status_source_events_total",
		metric.WithDescription("Count of pi --mode json subprocess events decoded by PiStatusSource, labeled by \"type\" (the event's own JSON type field, or \"unrecognized\" for an unknown type discriminator)"))
	if err != nil {
		// Only returned for a malformed instrument name/config, a
		// build-time-constant programmer error -- panicking at package init
		// surfaces it immediately in tests rather than silently dropping the
		// metric forever (same reasoning as safeexec_metrics.go's
		// mustInt64Counter).
		panic(err)
	}
	return counter
}

// recordPiStatusSourceEvent increments
// pi_status_source_events_total{type=eventType}.
func recordPiStatusSourceEvent(eventType string) {
	piStatusSourceEventsTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.String("type", eventType)))
}
