// Package analytics provides the provider interface and implementations for
// recording analytics events from the stapler-squad web UI and backend.
package analytics

import "context"

// Event represents a single analytics event to be recorded.
type Event struct {
	ID            string
	EventName     string
	EventCategory string
	SessionID     string
	DurationMs    *int64
	Page          string
	Component     string
	Labels        map[string]string
}

// AnalyticsProvider is the interface for recording analytics events.
// Implementations include SQLiteAnalyticsProvider (production) and
// LogAnalyticsProvider (testing / fallback).
type AnalyticsProvider interface {
	Record(ctx context.Context, event Event) error
}

// BatchProvider is an optional capability: persist many events in one
// transaction. POST /api/analytics batches up to 100 events per request, and one
// commit per event serialized them on the single analytics connection.
type BatchProvider interface {
	RecordBatch(ctx context.Context, events []Event) error
}

// RecordAll records events through p, using one transaction when p supports
// BatchProvider and falling back to per-event Record otherwise. Per-event
// errors in the fallback are collected, not fatal, matching the old loop.
func RecordAll(ctx context.Context, p AnalyticsProvider, events []Event) error {
	if bp, ok := p.(BatchProvider); ok {
		return bp.RecordBatch(ctx, events)
	}
	var first error
	for _, ev := range events {
		if err := p.Record(ctx, ev); err != nil && first == nil {
			first = err
		}
	}
	return first
}
