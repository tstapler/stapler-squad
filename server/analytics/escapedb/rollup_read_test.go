package escapedb

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
)

type readerFixture struct {
	client *ent.Client
	drv    dialect.Driver
	events []pkganalytics.EscapeEventRecord
	now    time.Time
	reader *Reader
}

// newReaderFixture writes ten days of hourly events through the writer (raw rows plus
// write-time rollups) and completes the backfill, as a running server would.
func newReaderFixture(t *testing.T, nowOffset time.Duration) *readerFixture {
	t.Helper()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	f := &readerFixture{client: client, drv: drv, now: testBase.Add(nowOffset), events: genEvents(testBase, 240, time.Hour)}
	_, err := ensureRollupState(ctx, drv, testBase.Add(-time.Hour)) // live counting began before the first event
	require.NoError(t, err)
	NewEscapeEventBatchWriter(drv).flushBatch(ctx, f.events)
	NewRetention(client, drv, RetentionConfig{Now: func() time.Time { return f.now }}).Run(ctx)
	f.reader = NewReader(client, drv, RollupTiers{}, func() time.Time { return f.now })
	return f
}

func at(h, m int, day int) *time.Time {
	t := time.Date(2026, 6, day, h, m, 0, 0, time.Local)
	return &t
}

func TestReader_MatchesRawTruth_AcrossRangeShapes(t *testing.T) {
	t.Parallel()
	f := newReaderFixture(t, 10*24*time.Hour)
	cases := map[string]AggregateQuery{
		"unbounded":              {},
		"hour aligned":           {Start: at(5, 0, 2), End: at(9, 0, 4)},
		"mid-hour both ends":     {Start: at(5, 17, 2), End: at(9, 41, 4)},
		"within one hour":        {Start: at(5, 31, 3), End: at(5, 45, 3)},
		"same instant as event":  {Start: at(5, 30, 3), End: at(5, 30, 3)},
		"start only":             {Start: at(12, 10, 5)},
		"end only":               {End: at(12, 10, 5)},
		"session filter":         {SessionID: "a", Start: at(5, 17, 2), End: at(9, 41, 6)},
		"empty range":            {Start: at(5, 0, 20), End: at(6, 0, 20)},
		"end on event, mid-hour": {Start: at(1, 10, 2), End: at(7, 30, 8)},
	}
	for name, q := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := f.reader.Aggregate(context.Background(), q)
			require.NoError(t, err)
			assert.Equal(t, truthAggregate(f.events, q.SessionID, q.Start, q.End), aggregateMap(got))
		})
	}
}

// Old ranges keep answering from rollups after their raw rows are gone.
func TestReader_AnswersOldRangesFromRollups_AfterRawEviction(t *testing.T) {
	t.Parallel()
	f := newReaderFixture(t, 10*24*time.Hour)
	ctx := context.Background()
	NewRetention(f.client, f.drv, RetentionConfig{RawRetentionDays: 3, Now: func() time.Time { return f.now }}).Run(ctx)
	require.Less(t, rawCount(ctx, t, f.client), len(f.events), "raw rows were evicted")

	q := AggregateQuery{Start: at(5, 0, 2), End: at(9, 0, 4)} // wholly older than the raw window
	got, err := f.reader.Aggregate(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, truthAggregate(f.events, "", q.Start, q.End), aggregateMap(got))

	got, err = f.reader.Aggregate(ctx, AggregateQuery{})
	require.NoError(t, err)
	assert.Equal(t, truthAggregate(f.events, "", nil, nil), aggregateMap(got))
}

// Once hourly rollups compact into days, a range reaching into the daily tier snaps outward to UTC days.
func TestReader_DailyTier_SnapsRangeEdgesOutwardToDayBoundaries(t *testing.T) {
	t.Parallel()
	f := newReaderFixture(t, 40*24*time.Hour) // events are 30+ days old: all compacted to days
	q := AggregateQuery{Start: at(5, 0, 3), End: at(7, 0, 4)}

	got, err := f.reader.Aggregate(context.Background(), q)
	require.NoError(t, err)

	dayStart := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	dayEnd := time.Date(2026, 6, 4, 23, 59, 59, 0, time.UTC)
	assert.Equal(t, truthAggregate(f.events, "", &dayStart, &dayEnd), aggregateMap(got), "whole UTC days 3 and 4")
}

func TestReader_FallsBackToRawRows_UntilBackfillIsDone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	events := genEvents(testBase, 24, time.Hour)
	seedRaw(ctx, t, client, events)
	_, err := ensureRollupState(ctx, drv, testBase.Add(48*time.Hour)) // rollups empty, backfill pending

	require.NoError(t, err)
	got, err := NewReader(client, drv, RollupTiers{}, func() time.Time { return testBase.Add(48 * time.Hour) }).Aggregate(ctx, AggregateQuery{})

	require.NoError(t, err)
	assert.Equal(t, truthAggregate(events, "", nil, nil), aggregateMap(got), "an empty rollup table must not read as zero history")
}

func TestReader_WithoutRollupDriver_CountsRawRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	events := genEvents(testBase, 24, time.Hour)
	seedRaw(ctx, t, client, events)

	got, err := NewReader(client, nil, RollupTiers{}, nil).Aggregate(ctx, AggregateQuery{Start: at(3, 0, 1), End: at(9, 0, 1)})

	require.NoError(t, err)
	assert.Equal(t, truthAggregate(events, "", at(3, 0, 1), at(9, 0, 1)), aggregateMap(got))
}
