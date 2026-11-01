package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"

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

// countedClient returns an ent client whose INSERT statements are counted.
func countedClient(t *testing.T) (*ent.Client, *atomic.Int64) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "a.db")+"?_pragma=busy_timeout(5000)&_fk=1")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	var n atomic.Int64
	var count atomic.Bool
	drv := dialect.DebugWithContext(entsql.OpenDB(dialect.SQLite, db), func(_ context.Context, a ...any) {
		if count.Load() && strings.Contains(fmt.Sprint(a...), "INSERT INTO") {
			n.Add(1)
		}
	})
	client := ent.NewClient(ent.Driver(drv))
	session.EntSchemaCreateMu.Lock()
	err = client.Schema.Create(context.Background())
	session.EntSchemaCreateMu.Unlock()
	require.NoError(t, err)
	count.Store(true)
	return client, &n
}

// POST /api/analytics batches up to 100 events; they must reach SQLite as one
// INSERT (one commit on the single analytics connection), not one per event.
func TestSQLiteAnalyticsProvider_RecordBatch_should_UseOneStatement(t *testing.T) {
	client, inserts := countedClient(t)
	p := NewSQLiteAnalyticsProvider(client)
	events := make([]Event, 100)
	for i := range events {
		events[i] = Event{ID: fmt.Sprintf("e-%d", i), EventName: "x", EventCategory: "user_action"}
	}
	require.NoError(t, RecordAll(context.Background(), p, events))
	assert.Equal(t, int64(1), inserts.Load())
	n, err := client.AnalyticsEvent.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 100, n)

	// A retried batch (same IDs) is idempotent rather than failing.
	require.NoError(t, RecordAll(context.Background(), p, events))
	n, _ = client.AnalyticsEvent.Query().Count(context.Background())
	assert.Equal(t, 100, n)
}

type plainProvider struct{ got []Event }

func (p *plainProvider) Record(_ context.Context, e Event) error {
	p.got = append(p.got, e)
	return nil
}

func TestRecordAll_should_FallBackToPerEventRecord_When_ProviderIsNotBatch(t *testing.T) {
	p := &plainProvider{}
	require.NoError(t, RecordAll(context.Background(), p, []Event{{EventName: "a"}, {EventName: "b"}}))
	assert.Len(t, p.got, 2)
}

func TestQualityMetricsProvider_RecordBatch_should_DropMetricsOnlyEvents(t *testing.T) {
	inner := &plainProvider{}
	p := WithQualityMetrics(inner)
	require.NoError(t, RecordAll(context.Background(), p, []Event{
		{EventName: "stored", EventCategory: "user_action"},
		{EventName: clientEventPrefix + "web_vital.lcp", EventCategory: "performance"}, // metrics-only, never stored
		{EventName: "also.stored", EventCategory: "user_action"},
	}))
	names := make([]string, 0, len(inner.got))
	for _, e := range inner.got {
		names = append(names, e.EventName)
	}
	assert.Equal(t, []string{"stored", "also.stored"}, names)
}
