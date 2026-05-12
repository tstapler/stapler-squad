package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/analyticsevent"
)

// insertOldEvents inserts n events with the given createdAt timestamp using
// direct ent Create calls so we can control the timestamp precisely.
func insertOldEvents(t *testing.T, ctx context.Context, client *ent.Client, prefix string, n int, createdAt time.Time) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := prefix + string(rune('a'+i))
		_, err := client.AnalyticsEvent.Create().
			SetID(id).
			SetEventName("old_event").
			SetEventCategory("navigation").
			SetCreatedAt(createdAt).
			Save(ctx)
		require.NoError(t, err, "insertOldEvents: Save must not fail for id=%s", id)
	}
}

// TestRetention_AgeEviction verifies that rows older than maxAgeDays are deleted.
func TestRetention_AgeEviction(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	provider := NewSQLiteAnalyticsProvider(client)

	// Insert 5 old events (91 days ago) and 3 recent events.
	old := time.Now().AddDate(0, 0, -91)
	insertOldEvents(t, ctx, client, "old-", 5, old)

	for i := 0; i < 3; i++ {
		err := provider.Record(ctx, Event{
			EventName:     "recent_event",
			EventCategory: "user_action",
		})
		require.NoError(t, err)
	}

	// Verify 8 total rows before enforcement.
	total, err := client.AnalyticsEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 8, total)

	// Run retention with 90-day age limit and no row cap.
	runRetention(ctx, client, 0, 90)

	// Only the 3 recent rows should remain.
	remaining, err := client.AnalyticsEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, remaining, "age eviction should have removed all 5 old rows")

	// Confirm no old rows survived.
	oldCount, err := client.AnalyticsEvent.Query().
		Where(analyticsevent.EventNameEQ("old_event")).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, oldCount)
}

// TestRetention_CountEviction verifies that oldest rows are deleted when count exceeds maxRows.
func TestRetention_CountEviction(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	// Insert 10 events with staggered timestamps so order is deterministic.
	base := time.Now()
	for i := 0; i < 10; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		id := "count-" + string(rune('a'+i))
		_, err := client.AnalyticsEvent.Create().
			SetID(id).
			SetEventName("event").
			SetEventCategory("rpc").
			SetCreatedAt(ts).
			Save(ctx)
		require.NoError(t, err)
	}

	// Run retention with maxRows=6 and no age limit.
	runRetention(ctx, client, 6, 0)

	remaining, err := client.AnalyticsEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 6, remaining, "count eviction should trim to maxRows=6")
}

// TestRetention_Noop verifies that enforcement is a no-op when within limits.
func TestRetention_Noop(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	provider := NewSQLiteAnalyticsProvider(client)
	for i := 0; i < 5; i++ {
		err := provider.Record(ctx, Event{EventName: "e", EventCategory: "rpc"})
		require.NoError(t, err)
	}

	// Limits well above current count — nothing should be deleted.
	runRetention(ctx, client, 100_000, 90)

	count, err := client.AnalyticsEvent.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}
