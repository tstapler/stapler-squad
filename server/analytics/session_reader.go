package analytics

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/analyticsevent"
)

// sessionEventLimit caps one session's read so a long-lived session cannot make
// an unbounded query.
const sessionEventLimit = 2000

// ListBySessionID returns up to 2000 analytics events recorded for sessionID,
// most recent first. client must be the analytics.db client (OpenAnalyticsDB):
// sessions.db has an empty analytics_events table (ADR-036).
func ListBySessionID(ctx context.Context, client *ent.Client, sessionID string) ([]session.AnalyticsData, error) {
	if client == nil {
		return nil, errors.New("analytics db unavailable")
	}
	rows, err := client.AnalyticsEvent.Query().
		Where(analyticsevent.SessionIDEQ(sessionID)).
		Order(ent.Desc(analyticsevent.FieldCreatedAt)).
		Limit(sessionEventLimit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list analytics by session %q: %w", sessionID, err)
	}
	out := make([]session.AnalyticsData, len(rows))
	for i, e := range rows {
		out[i] = toSessionAnalyticsData(e)
	}
	return out, nil
}

// toSessionAnalyticsData leaves fields AnalyticsEvent has no column for at zero.
func toSessionAnalyticsData(e *ent.AnalyticsEvent) session.AnalyticsData {
	var durationMs int64
	if e.DurationMs != nil {
		durationMs = *e.DurationMs
	}
	return session.AnalyticsData{
		ID:              e.ID,
		SessionID:       e.SessionID,
		ToolName:        e.EventName,
		CommandCategory: e.EventCategory,
		DurationMs:      durationMs,
		CreatedAt:       e.CreatedAt,
	}
}

// BundleStorage serves the session diagnostic bundle: everything from sessions.db
// through the embedded Storage, analytics events from analytics.db.
type BundleStorage struct {
	*session.Storage
	analyticsClient *ent.Client
}

// NewBundleStorage wraps storage; a nil analyticsClient makes the analytics
// section omitted with an error rather than silently empty.
func NewBundleStorage(storage *session.Storage, analyticsClient *ent.Client) *BundleStorage {
	return &BundleStorage{Storage: storage, analyticsClient: analyticsClient}
}

// ListAnalyticsBySessionID reads the session's events from analytics.db.
func (b *BundleStorage) ListAnalyticsBySessionID(ctx context.Context, sessionID string) ([]session.AnalyticsData, error) {
	return ListBySessionID(ctx, b.analyticsClient, sessionID)
}
