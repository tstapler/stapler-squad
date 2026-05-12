package analytics

import (
	"context"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// SQLiteAnalyticsProvider records analytics events to a dedicated SQLite database
// via the ent ORM. It is safe for concurrent use — the underlying ent client
// serialises all writes through a single connection (MaxOpenConns=1).
type SQLiteAnalyticsProvider struct {
	client *ent.Client
}

// NewSQLiteAnalyticsProvider creates a new SQLiteAnalyticsProvider backed by client.
// The caller retains ownership of client and must close it on shutdown.
func NewSQLiteAnalyticsProvider(client *ent.Client) *SQLiteAnalyticsProvider {
	return &SQLiteAnalyticsProvider{client: client}
}

// Record inserts a single analytics event into the database.
// If event.ID is empty, a new UUID is generated automatically.
// Optional fields (SessionID, DurationMs, Page, Component, Labels) are skipped
// when zero/nil so the database stores NULLs rather than empty strings.
func (p *SQLiteAnalyticsProvider) Record(ctx context.Context, event Event) error {
	id := event.ID
	if id == "" {
		id = uuid.New().String()
	}

	q := p.client.AnalyticsEvent.Create().
		SetID(id).
		SetEventName(event.EventName).
		SetEventCategory(event.EventCategory)

	if event.SessionID != "" {
		q = q.SetSessionID(event.SessionID)
	}

	q = q.SetNillableDurationMs(event.DurationMs)

	if event.Page != "" {
		q = q.SetPage(event.Page)
	}

	if event.Component != "" {
		q = q.SetComponent(event.Component)
	}

	if len(event.Labels) > 0 {
		q = q.SetLabels(event.Labels)
	}

	_, err := q.Save(ctx)
	return err
}
