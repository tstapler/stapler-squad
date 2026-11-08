package escapedb

import (
	"context"
	"fmt"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// seedRaw inserts events as raw rows only, the state of a database written before rollups existed.
func seedRaw(ctx context.Context, t *testing.T, client *ent.Client, events []pkganalytics.EscapeEventRecord) {
	t.Helper()
	for _, e := range events {
		_, err := client.EscapeEvent.Create().
			SetID(fmt.Sprintf("%s-%d-%d", e.SessionID, e.SessionSeq, e.WallTime.UnixNano())).
			SetSessionID(e.SessionID).SetProjectPath(e.ProjectPath).SetStage(string(e.Stage)).
			SetSequenceType(e.SequenceType).SetByteLength(e.ByteLen).SetMangled(e.Mangled).
			SetMangleType(e.MangleType).SetWallTime(e.WallTime).SetSessionSeq(e.SessionSeq).Save(ctx)
		require.NoError(t, err)
	}
}

func rawCount(ctx context.Context, t *testing.T, client *ent.Client) int {
	t.Helper()
	n, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b", "s", "big", "mid", "small", "one", "two")).Count(ctx)
	require.NoError(t, err)
	return n
}

func TestRetention_PerSessionCapKeepsNewest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	seedRaw(ctx, t, client, genEvents(testBase, 12, time.Second)) // 6 rows each for a and b

	NewRetention(client, nil, RetentionConfig{MaxRowsPerSession: 3}).Run(ctx)

	for _, sessionID := range []string{"a", "b"} {
		got, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(sessionID)).Order(escapeevent.ByWallTime()).All(ctx)
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, map[string]int64{"a": 6, "b": 7}[sessionID], got[0].SessionSeq, "keeps the newest")
	}
}

// Multi-chunk eviction: over-cap sessions land exactly on the cap, an under-cap session is untouched.
func TestRetention_PerSessionCap_MultiChunkAndUnderCap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	const capRows = 40
	sizes := map[string]int{"big": 2*escapeEvictChunkRows + 100 + capRows, "mid": capRows + 3, "small": capRows - 5}
	for s, n := range sizes {
		evs := genEvents(testBase, n, time.Second)
		for i := range evs {
			evs[i].SessionID = s
		}
		seedRaw(ctx, t, client, evs)
	}

	NewRetention(client, nil, RetentionConfig{MaxRowsPerSession: capRows}).Run(ctx)

	for s, n := range sizes {
		want := min(n, capRows)
		got, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(s)).Order(escapeevent.ByWallTime()).All(ctx)
		require.NoError(t, err)
		require.Len(t, got, want, s)
		assert.Equal(t, int64(n-want), got[0].SessionSeq, "%s keeps the newest rows", s)
	}
}

func TestRetention_AgeBoundary_DeletesStrictlyOlderThanCutoff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	now := testBase.Add(10 * 24 * time.Hour)
	cutoff := now.AddDate(0, 0, -3)
	evs := genEvents(cutoff.Add(-time.Second), 3, time.Second) // cutoff-1s, cutoff, cutoff+1s
	seedRaw(ctx, t, client, evs)

	NewRetention(client, nil, RetentionConfig{RawRetentionDays: 3, Now: func() time.Time { return now }}).Run(ctx)

	got, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b")).Order(escapeevent.BySessionSeq()).All(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, int64(1), got[0].SessionSeq, "the row exactly at the cutoff stays")
}

func TestRetention_AgeEviction_IsBudgetedPerRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	now := testBase.Add(10 * 24 * time.Hour)
	seedRaw(ctx, t, client, genEvents(testBase, 3*escapeEvictChunkRows, time.Second)) // all older than 3 days
	r := NewRetention(client, nil, RetentionConfig{RawRetentionDays: 3, AgeChunksPerRun: 2, Now: func() time.Time { return now }})

	r.Run(ctx)
	assert.Equal(t, escapeEvictChunkRows, rawCount(ctx, t, client), "two chunks deleted in the first cycle")
	r.Run(ctx)
	assert.Zero(t, rawCount(ctx, t, client))
}

// Deleting raw rows must not lose their counts: that is the point of the rollups.
func TestRetention_BackfillThenAgeEviction_KeepsHistoryInRollups(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	now := testBase.Add(5 * 24 * time.Hour)
	events := genEvents(testBase, 120, time.Hour) // five days of hourly events
	seedRaw(ctx, t, client, events)
	_, err := ensureRollupState(ctx, drv, now) // every seeded row predates live counting
	require.NoError(t, err)

	r := NewRetention(client, drv, RetentionConfig{RawRetentionDays: 2, BackfillChunkRows: 7, Now: func() time.Time { return now }})
	r.Run(ctx)

	cutoff := now.AddDate(0, 0, -2)
	wantRaw := 0
	for _, e := range events {
		if !e.WallTime.Before(cutoff) {
			wantRaw++
		}
	}
	assert.Equal(t, wantRaw, rawCount(ctx, t, client), "only the raw window remains")
	got, err := NewReader(client, drv, RollupTiers{}, func() time.Time { return now }).Aggregate(ctx, AggregateQuery{})
	require.NoError(t, err)
	assert.Equal(t, truthAggregate(events, "", nil, nil), aggregateMap(got), "history survives raw eviction")
}

func TestRetention_DoesNotDeleteRawRows_UntilBackfillCompletes(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	now := testBase.Add(5 * 24 * time.Hour)
	seedRaw(ctx, t, client, genEvents(testBase, 40, time.Hour))
	_, err := ensureRollupState(ctx, drv, now)
	require.NoError(t, err)
	r := NewRetention(client, drv, RetentionConfig{RawRetentionDays: 1, BackfillChunkRows: 5, Now: func() time.Time { return now }})
	r.pauseHook = func(context.Context) error { cancel(); return context.Canceled } // interrupt after the first chunk

	r.Run(ctx)

	assert.Equal(t, 40, rawCount(context.Background(), t, client), "an unfinished backfill blocks raw deletion")
	st, err := readRollupState(context.Background(), drv)
	require.NoError(t, err)
	assert.False(t, st.BackfillDone)
	assert.EqualValues(t, 5, st.BackfilledRows)

	r.pauseHook = nil // resume
	r.Run(context.Background())
	st, err = readRollupState(context.Background(), drv)
	require.NoError(t, err)
	assert.True(t, st.BackfillDone)
	assert.EqualValues(t, 40, st.BackfilledRows, "resume neither skips nor double counts")
	_, total, _ := rollupTotals(t, context.Background(), drv)
	assert.EqualValues(t, 40, total)
}

// Rows the writer already counted (wall_time at or after live_since) are not counted again.
func TestBackfill_SkipsRowsCountedAtWriteTime_AndIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	liveSince := testBase.Add(10 * time.Hour)
	_, err := ensureRollupState(ctx, drv, liveSince)
	require.NoError(t, err)
	events := genEvents(testBase, 20, time.Hour) // hours 0-9 predate liveSince, 10-19 do not
	seedRaw(ctx, t, client, events)
	pre := events[:10]

	for range 2 {
		require.NoError(t, backfillRollups(ctx, drv, 3, func(context.Context) error { return nil }))
	}

	var r entsql.Rows
	require.NoError(t, drv.Query(ctx, "SELECT SUM(event_count), MIN(first_seen_ms), MAX(last_seen_ms) FROM "+rollupTable, []any{}, &r))
	defer r.Close()
	require.True(t, r.Next())
	var total, first, last int64
	require.NoError(t, r.Scan(&total, &first, &last))
	assert.EqualValues(t, len(pre), total)
	assert.Equal(t, pre[0].WallTime.UnixMilli(), first)
	assert.Equal(t, pre[len(pre)-1].WallTime.UnixMilli(), last)
}

func TestCompactRollups_MergesOldHoursIntoDays_PreservingTotals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, drv := openStoreForTest(ctx, t, t.TempDir())
	now := testBase.Add(20 * 24 * time.Hour)
	NewEscapeEventBatchWriter(drv).flushBatch(ctx, genEvents(testBase, 480, time.Hour)) // 20 days of hourly events
	rowsBefore, totalBefore, bytesBefore := rollupTotals(t, ctx, drv)

	require.NoError(t, compactRollups(ctx, drv, now, RollupTiers{HourlyDays: 14}))

	rows, total, bytes := rollupTotals(t, ctx, drv)
	assert.Equal(t, totalBefore, total)
	assert.Equal(t, bytesBefore, bytes)
	assert.Less(t, rows, rowsBefore)
	cutoff := bucketStart(now.AddDate(0, 0, -14), RollupDaySecs)
	assert.Zero(t, countRollups(t, ctx, drv, "bucket_secs = 3600 AND bucket_start < ?", cutoff), "no old hourly rollups remain")
	assert.Zero(t, countRollups(t, ctx, drv, "bucket_secs = 86400 AND bucket_start >= ?", cutoff), "no day rollup reaches into the hourly tier")
	assert.Positive(t, countRollups(t, ctx, drv, "bucket_secs = 3600 AND bucket_start >= ?", cutoff))

	require.NoError(t, compactRollups(ctx, drv, now, RollupTiers{HourlyDays: 14}))
	rows2, total2, _ := rollupTotals(t, ctx, drv)
	assert.Equal(t, rows, rows2, "a second pass changes nothing")
	assert.Equal(t, total, total2)
}

func TestCompactRollups_PrunesDailyRollupsPastRetention(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, drv := openStoreForTest(ctx, t, t.TempDir())
	now := testBase.Add(20 * 24 * time.Hour)
	NewEscapeEventBatchWriter(drv).flushBatch(ctx, genEvents(testBase, 480, time.Hour))

	require.NoError(t, compactRollups(ctx, drv, now, RollupTiers{HourlyDays: 14, DailyDays: 18}))

	pruneBefore := bucketStart(now.AddDate(0, 0, -18), RollupDaySecs)
	assert.Zero(t, countRollups(t, ctx, drv, "bucket_secs = 86400 AND bucket_start < ?", pruneBefore))
	assert.Positive(t, countRollups(t, ctx, drv, "bucket_secs = 86400 AND bucket_start >= ?", pruneBefore))
}

func countRollups(t *testing.T, ctx context.Context, drv interface {
	Query(ctx context.Context, query string, args, v any) error
}, where string, arg any) int64 {
	t.Helper()
	var r entsql.Rows
	require.NoError(t, drv.Query(ctx, "SELECT COUNT(*) FROM "+rollupTable+" WHERE "+where, []any{arg}, &r))
	defer r.Close()
	require.True(t, r.Next())
	var n int64
	require.NoError(t, r.Scan(&n))
	return n
}
