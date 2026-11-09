package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// Regression: the bundle read AnalyticsEvent through the sessions.db client, whose
// table is empty; events recorded via the analytics writer live in analytics.db.
func TestBundleStorageReadsAnalyticsDBNotSessionsDB(t *testing.T) {
	ctx := context.Background()
	aClient, err := OpenAnalyticsDB(ctx, t.TempDir())
	require.NoError(t, err)
	defer func() { _ = aClient.Close() }()
	provider := NewSQLiteAnalyticsProvider(aClient)
	dur := int64(7)
	aClient.AnalyticsEvent.Create().SetID("old").SetEventName("a").SetEventCategory("ui").
		SetSessionID("uuid-1").SetCreatedAt(time.Now().Add(-time.Hour)).ExecX(ctx)
	require.NoError(t, provider.Record(ctx, Event{ID: "new", EventName: "b", EventCategory: "rpc", SessionID: "uuid-1", DurationMs: &dur}))
	require.NoError(t, provider.Record(ctx, Event{ID: "other", EventName: "c", EventCategory: "ui", SessionID: "uuid-2"}))

	repo, err := session.NewEntRepository(session.WithDatabasePath(t.TempDir() + "/sessions.db"))
	require.NoError(t, err)
	defer func() { _ = repo.Close() }()
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	rows, err := NewBundleStorage(storage, aClient).ListAnalyticsBySessionID(ctx, "uuid-1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "new", rows[0].ID, "most recent first")
	assert.Equal(t, "b", rows[0].ToolName)
	assert.Equal(t, "rpc", rows[0].CommandCategory)
	assert.Equal(t, int64(7), rows[0].DurationMs)
	assert.Equal(t, "old", rows[1].ID)
}

func TestListBySessionIDWithoutClientReportsError(t *testing.T) {
	_, err := ListBySessionID(context.Background(), nil, "x")
	require.Error(t, err)
}
