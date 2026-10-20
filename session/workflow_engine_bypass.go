package session

// workflow_engine_bypass.go implements workflow_engine_bypass_total{caller}
// (ADR-007 D9): a count of status writes made through an exempt entry point
// (rollback, debug) that the workflow engine deliberately never checks.
// Pending migration sites are NOT instrumented; the inventory test in
// server/services tracks those.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	engineBypassMu     sync.Mutex
	engineBypassCounts = map[TransitionCaller]int64{}
)

var engineBypassTotal = mustEngineBypassCounter()

func mustEngineBypassCounter() metric.Int64Counter {
	counter, err := telemetry.GetMeter().Int64Counter("workflow_engine_bypass_total",
		metric.WithDescription("Status writes made through an exempt entry point (rollback, debug) that the workflow engine never checks, labeled by \"caller\""))
	if err != nil {
		// A malformed instrument name is a build-time programmer error.
		panic(err)
	}
	return counter
}

// RecordEngineBypass increments workflow_engine_bypass_total{caller}. Call it
// only from exempt entry points, never from a site pending migration.
func RecordEngineBypass(caller TransitionCaller) {
	engineBypassMu.Lock()
	engineBypassCounts[caller]++
	engineBypassMu.Unlock()
	engineBypassTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.String("caller", string(caller))))
}

// EngineBypassCount returns the in-process count for one caller.
func EngineBypassCount(caller TransitionCaller) int64 {
	engineBypassMu.Lock()
	defer engineBypassMu.Unlock()
	return engineBypassCounts[caller]
}

// EngineBypassSnapshot returns a copy of the per-caller counts for the
// actuator. The map is never nil.
func EngineBypassSnapshot() map[string]int64 {
	engineBypassMu.Lock()
	defer engineBypassMu.Unlock()
	out := make(map[string]int64, len(engineBypassCounts))
	for k, v := range engineBypassCounts {
		out[string(k)] = v
	}
	return out
}
