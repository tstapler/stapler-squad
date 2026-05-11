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
