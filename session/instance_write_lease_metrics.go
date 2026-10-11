package session

// Lease metrics are mirrored in-process (the OTel meter is a no-op when
// telemetry is not initialized), following server/deliverygate/metrics.go.

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

var leaseWriters = [...]string{
	LeaseWriterReply, LeaseWriterDriver, LeaseWriterAutonomous, LeaseWriterNudge,
	LeaseWriterSteer, LeaseWriterMCP, LeaseWriterRateLimit, LeaseWriterOther,
}

var leaseBusyMirror [len(leaseWriters)]atomic.Uint64

var leaseBusyCounter = registerLeaseInstruments()

// registerLeaseInstruments registers the busy counter (returned) and the
// held-seconds observable gauge (kept alive by its callback registration).
func registerLeaseInstruments() metric.Int64Counter {
	meter := telemetry.GetMeter()
	busy, err := meter.Int64Counter("hidden_session_write_lease_busy_total",
		metric.WithDescription("Writes that found the per-instance terminal write lease held, by writer"))
	if err != nil {
		panic(err)
	}
	_, err = meter.Float64ObservableGauge("hidden_session_write_lease_held_seconds",
		metric.WithDescription("Longest current hold of the terminal write lease, by writer"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			now := time.Now()
			for _, w := range leaseWriters {
				o.Observe(LeaseHeldSeconds(w, now), metric.WithAttributes(attribute.String("writer", w)))
			}
			return nil
		}))
	if err != nil {
		panic(err)
	}
	return busy
}

func leaseWriterIndex(writer string) int {
	for i, w := range leaseWriters {
		if w == writer {
			return i
		}
	}
	return len(leaseWriters) - 1 // "other"
}

func normalizeLeaseWriter(writer string) string { return leaseWriters[leaseWriterIndex(writer)] }

func recordLeaseBusy(writer string) {
	idx := leaseWriterIndex(writer)
	leaseBusyMirror[idx].Add(1)
	leaseBusyCounter.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("writer", leaseWriters[idx])))
}

// LeaseBusyTotal is the in-process hidden_session_write_lease_busy_total{writer}.
func LeaseBusyTotal(writer string) uint64 {
	return leaseBusyMirror[leaseWriterIndex(writer)].Load()
}

// LeaseHeldSeconds is the in-process hidden_session_write_lease_held_seconds{writer}:
// the longest current hold by that writer at now, 0 when none is held.
func LeaseHeldSeconds(writer string, now time.Time) float64 {
	var longest time.Duration
	leaseRegistry.mu.Lock()
	for _, l := range leaseRegistry.held {
		if l.writer != writer {
			continue
		}
		if d := now.Sub(l.acquiredAt); d > longest {
			longest = d
		}
	}
	leaseRegistry.mu.Unlock()
	return longest.Seconds()
}
