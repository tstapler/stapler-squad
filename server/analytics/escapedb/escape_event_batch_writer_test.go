package escapedb

import (
	"context"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

func rollupTotals(t *testing.T, ctx context.Context, q interface {
	Query(ctx context.Context, query string, args, v any) error
}) (rows, events, bytes int64) {
	t.Helper()
	var r entsql.Rows
	require.NoError(t, q.Query(ctx, "SELECT COUNT(*), COALESCE(SUM(event_count),0), COALESCE(SUM(byte_total),0) FROM "+rollupTable, []any{}, &r))
	defer r.Close()
	require.True(t, r.Next())
	require.NoError(t, r.Scan(&rows, &events, &bytes))
	return rows, events, bytes
}

func TestFlushBatch_WritesRawRowsAndHourlyRollupsTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	events := genEvents(testBase, 30, 20*time.Minute) // spans 10 hours

	NewEscapeEventBatchWriter(drv).flushBatch(ctx, events)

	n, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 30, n)
	_, total, bytes := rollupTotals(t, ctx, drv)
	var wantBytes int64
	for _, e := range events {
		wantBytes += int64(e.ByteLen)
	}
	assert.EqualValues(t, 30, total)
	assert.Equal(t, wantBytes, bytes)
}

func TestFlushBatch_StatementCountIsIndependentOfBatchSize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, drv := openStoreForTest(ctx, t, t.TempDir())
	w := NewEscapeEventBatchWriter(drv)

	countCtx, counter := dbobs.WithQueryCounter(ctx)
	w.flushBatch(countCtx, genEvents(testBase, 400, time.Second)) // one hour, 8 distinct keys
	// BEGIN + INSERT + rollup upsert + COMMIT; the old path issued INSERT + COUNT + DELETE per flush.
	assert.LessOrEqual(t, counter.Count(), int64(5), "statements for one 400-event flush")
}

func TestFlushBatch_FailureRollsBackRawRowsAndCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	require.NoError(t, drv.Exec(ctx, "DROP TABLE "+rollupTable, []any{}, nil))
	w := NewEscapeEventBatchWriter(drv)

	w.flushBatch(ctx, genEvents(testBase, 10, time.Second))

	n, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "raw rows must not outlive a failed rollup upsert")
	assert.EqualValues(t, 10, w.Stats().FailedRows)
	assert.Zero(t, w.Stats().RowsWritten)
}

func TestWriteEscapeEvent_DropsAndCountsWhenQueueFull_WithoutBlocking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, drv := openStoreForTest(ctx, t, t.TempDir())
	w := NewEscapeEventBatchWriter(drv, WithQueueSize(2))

	for _, ev := range genEvents(testBase, 5, time.Second) { // no consumer: a blocking write would hang
		w.WriteEscapeEvent(ctx, ev)
	}

	assert.EqualValues(t, 3, w.DroppedCount())
	s := w.Stats()
	assert.EqualValues(t, 2, s.Enqueued)
	assert.Equal(t, 2, s.QueueDepth)
	assert.Equal(t, 2, s.QueueCapacity)
}

// Events wait for the interval tick rather than being written one flush per event.
func TestStart_WritesOnIntervalTick_NotPerEvent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	tick := make(chan time.Time)
	w := NewEscapeEventBatchWriter(drv, WithTicker(func(time.Duration) (<-chan time.Time, func()) { return tick, func() {} }))
	done := make(chan struct{})
	go func() { defer close(done); w.Start(ctx) }()

	for _, ev := range genEvents(testBase, 3, time.Second) {
		w.WriteEscapeEvent(ctx, ev)
	}
	count := func() int {
		n, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b")).Count(ctx)
		require.NoError(t, err)
		return n
	}
	assert.Zero(t, count(), "nothing is written before the tick")

	require.Eventually(t, func() bool { return w.Stats().QueueDepth == 0 }, 10*time.Second, time.Millisecond)
	tick <- time.Time{} // unbuffered: the loop received it
	tick <- time.Time{} // second receive proves the first flush finished
	assert.Equal(t, 3, count())
	assert.EqualValues(t, 1, w.Stats().Flushes, "one transaction for the whole batch")
	cancel()
	<-done
}

func TestStart_FlushesImmediatelyAtRowThreshold(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, drv := openStoreForTest(ctx, t, t.TempDir())
	w := NewEscapeEventBatchWriter(drv, WithFlushRows(4),
		WithTicker(func(time.Duration) (<-chan time.Time, func()) { return make(chan time.Time), func() {} }))
	done := make(chan struct{})
	go func() { defer close(done); w.Start(ctx) }()

	for _, ev := range genEvents(testBase, 4, time.Second) {
		w.WriteEscapeEvent(ctx, ev)
	}
	require.Eventually(t, func() bool { return w.Stats().RowsWritten == 4 }, 10*time.Second, time.Millisecond)
	cancel()
	<-done
}

// Events queued at shutdown are persisted, not lost to the cancelled context.
func TestStart_ShutdownDrainPersistsQueuedEvents(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client, drv := openStoreForTest(context.Background(), t, t.TempDir())
	w := NewEscapeEventBatchWriter(drv)
	for _, ev := range genEvents(testBase, 7, time.Second) {
		w.WriteEscapeEvent(ctx, ev)
	}
	cancel()

	w.Start(ctx)

	n, err := client.EscapeEvent.Query().Where(escapeevent.SessionIDIn("a", "b")).Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 7, n)
	_, total, _ := rollupTotals(t, context.Background(), drv)
	assert.EqualValues(t, 7, total)
}

// Statements for a bulk insert carry the writer's name, not "unknown".
func TestFlushBatch_BulkInsertIsAttributedToWriter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, drv := openStoreForTest(ctx, t, t.TempDir())

	NewEscapeEventBatchWriter(drv).flushBatch(ctx, genEvents(testBase, 100, time.Second))

	found := false
	for _, pool := range dbobs.Snapshot() {
		for _, st := range pool.TopStatements {
			if strings.Contains(st.Statement, "INSERT INTO `escape_events`") && strings.Contains(st.Caller, "EscapeEventBatchWriter.flushBatch") {
				found = true
			}
		}
	}
	assert.True(t, found, "escape_events INSERT not attributed to flushBatch")
}

func TestFlushBatch_StoresOptionalColumnsAsNullAndReadsBackThroughEnt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, drv := openStoreForTest(ctx, t, t.TempDir())
	ev := genEvents(testBase, 1, time.Second)[0]
	ev.ProjectPath, ev.MangleType = "", ""
	ev.SequenceSignature, ev.RawBytes = "CSI:?25h", []byte{1, 2}

	NewEscapeEventBatchWriter(drv).flushBatch(ctx, []pkganalytics.EscapeEventRecord{ev})

	got, err := client.EscapeEvent.Query().Where(escapeevent.SessionID(ev.SessionID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "CSI:?25h", got.SequenceSignature)
	assert.Equal(t, []byte{1, 2}, got.RawBytes)
	assert.Empty(t, got.ProjectPath)
	assert.True(t, got.WallTime.Equal(ev.WallTime), "wall_time round trip: %v vs %v", got.WallTime, ev.WallTime)
}
