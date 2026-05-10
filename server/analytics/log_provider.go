package analytics

import (
	"context"
	"encoding/json"

	"github.com/tstapler/stapler-squad/log"
)

// LogAnalyticsProvider is a no-op analytics provider that logs events instead of
// persisting them. It is used as a fallback when the database fails to open and
// as a convenient substitute in unit tests.
type LogAnalyticsProvider struct{}

// NewLogAnalyticsProvider creates a LogAnalyticsProvider.
func NewLogAnalyticsProvider() *LogAnalyticsProvider {
	return &LogAnalyticsProvider{}
}

// Record logs the event using the package-level InfoLog and returns nil.
func (p *LogAnalyticsProvider) Record(_ context.Context, event Event) error {
	labelsJSON, _ := json.Marshal(event.Labels)
	log.InfoLog.Printf("[analytics] event=%q category=%q session=%q page=%q component=%q labels=%s",
		event.EventName, event.EventCategory, event.SessionID, event.Page, event.Component, labelsJSON)
	return nil
}
