package analytics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteAnalyticsProvider_Record_InsertsRow verifies that Record persists a
// single event and that the stored fields match the supplied Event struct.
func TestSQLiteAnalyticsProvider_Record_InsertsRow(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	provider := NewSQLiteAnalyticsProvider(client)

	dur := int64(42)
	evt := Event{
		ID:            "test-id-001",
		EventName:     "button_click",
		EventCategory: "user_action",
		SessionID:     "sess-123",
		DurationMs:    &dur,
		Page:          "/sessions",
		Component:     "SessionList",
		Labels:        map[string]string{"env": "test"},
	}

	err = provider.Record(ctx, evt)
	require.NoError(t, err, "Record must not return an error")

	// Verify the row was inserted by querying it back.
	row, err := client.AnalyticsEvent.Get(ctx, "test-id-001")
	require.NoError(t, err, "Get must find the inserted row")

	assert.Equal(t, "button_click", row.EventName)
	assert.Equal(t, "user_action", row.EventCategory)
	assert.Equal(t, "sess-123", row.SessionID)
	require.NotNil(t, row.DurationMs)
	assert.Equal(t, int64(42), *row.DurationMs)
	assert.Equal(t, "/sessions", row.Page)
	assert.Equal(t, "SessionList", row.Component)
	assert.Equal(t, map[string]string{"env": "test"}, row.Labels)
}

// TestSQLiteAnalyticsProvider_Record_GeneratesUUID verifies that Record assigns a
// UUID when Event.ID is empty.
func TestSQLiteAnalyticsProvider_Record_GeneratesUUID(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	provider := NewSQLiteAnalyticsProvider(client)

	evt := Event{
		EventName:     "page_view",
		EventCategory: "navigation",
	}

	err = provider.Record(ctx, evt)
	require.NoError(t, err)

	// Row count should be 1.
	count, err := client.AnalyticsEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// The single row's ID must be a non-empty string (a UUID).
	ids, err := client.AnalyticsEvent.Query().IDs(ctx)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	assert.NotEmpty(t, ids[0], "auto-generated ID must not be empty")
}

// TestSQLiteAnalyticsProvider_Record_NilOptionals verifies that a minimal event
// (only required fields set) can be recorded without error.
func TestSQLiteAnalyticsProvider_Record_NilOptionals(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	provider := NewSQLiteAnalyticsProvider(client)

	err = provider.Record(ctx, Event{
		EventName:     "rpc_call",
		EventCategory: "rpc",
	})
	require.NoError(t, err)

	row, err := client.AnalyticsEvent.Query().Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, row.DurationMs, "DurationMs should be nil when not set")
	assert.Empty(t, row.SessionID)
	assert.Empty(t, row.Page)
	assert.Empty(t, row.Component)
}
