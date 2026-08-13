package analytics

import (
	"context"
	"encoding/json"
	"log/slog"
)

// LogAnalyticsProvider is a no-op analytics provider that logs events instead of
// persisting them. It is used as a fallback when the database fails to open and
// as a convenient substitute in unit tests.
type LogAnalyticsProvider struct{}

// NewLogAnalyticsProvider creates a LogAnalyticsProvider.
func NewLogAnalyticsProvider() *LogAnalyticsProvider {
	return &LogAnalyticsProvider{}
}

// Record logs the event using slog.Default() and returns nil.
func (p *LogAnalyticsProvider) Record(_ context.Context, event Event) error {
	labelsJSON, _ := json.Marshal(event.Labels)
	slog.Info("analytics event", "event", event.EventName, "category", event.EventCategory, "session", event.SessionID, "page", event.Page, "component", event.Component, "labels", string(labelsJSON))
	return nil
}
