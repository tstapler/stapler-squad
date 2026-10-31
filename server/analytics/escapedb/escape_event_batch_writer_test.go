package escapedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// TestDeleteOldestRows_QueryPlanAvoidsSort guards the CPU fix: with a session at its
// row cap, every flush runs "oldest N by wall_time for one session". That must be an
// ordered index scan, not a read-and-temp-sort of all the session's rows.
func TestDeleteOldestRows_QueryPlanAvoidsSort(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	client := openForTest(ctx, t, dir)
	defer client.Close()

	db, err := sql.Open("sqlite", "file:"+Path(dir)+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()

	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT id FROM escape_events WHERE session_id = ? ORDER BY wall_time LIMIT 100", "s")
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	joined := strings.Join(plan, " | ")
	assert.NotContains(t, joined, "TEMP B-TREE", "oldest-rows query sorts all of a session's rows: %s", joined)
	assert.Contains(t, joined, "session_id_wall_time", "oldest-rows query should use the (session_id, wall_time) index: %s", joined)
}

// TestFlushBatch_EnforcesPerSessionCap_DeletingOldestFirst pins the behaviour the index
// must not change: over the cap, the oldest rows by wall_time go and the newest stay.
func TestFlushBatch_EnforcesPerSessionCap_DeletingOldestFirst(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	const cap = 5
	w := NewEscapeEventBatchWriter(client, cap)
	base := time.Now().Add(-time.Hour)
	batch := func(from, n int) []pkganalytics.EscapeEventRecord {
		var out []pkganalytics.EscapeEventRecord
		for i := from; i < from+n; i++ {
			out = append(out, pkganalytics.EscapeEventRecord{
				SessionID: "s1", Stage: "input", SequenceType: "csi", ByteLen: 1,
				WallTime: base.Add(time.Duration(i) * time.Second), SessionSeq: int64(i),
			})
		}
		return out
	}
	w.flushBatch(ctx, batch(0, 4))
	w.flushBatch(ctx, batch(4, 4)) // 8 rows total -> trim to the cap

	got, err := client.EscapeEvent.Query().Where(escapeevent.SessionID("s1")).Order(escapeevent.ByWallTime()).All(ctx)
	require.NoError(t, err)
	require.Len(t, got, cap)
	seqs := make([]string, 0, len(got))
	for _, e := range got {
		seqs = append(seqs, fmt.Sprint(e.SessionSeq))
	}
	assert.Equal(t, "3 4 5 6 7", strings.Join(seqs, " "), "the %d newest rows should remain", cap)
}

func escapeBatch(session string, base time.Time, from, n int) []pkganalytics.EscapeEventRecord {
	out := make([]pkganalytics.EscapeEventRecord, 0, n)
	for i := from; i < from+n; i++ {
		out = append(out, pkganalytics.EscapeEventRecord{
			SessionID: session, Stage: "input", SequenceType: "csi", ByteLen: 1,
			WallTime: base.Add(time.Duration(i) * time.Second), SessionSeq: int64(i),
		})
	}
	return out
}

func dbCount(t *testing.T, ctx context.Context, w *EscapeEventBatchWriter, session string) int {
	t.Helper()
	n, err := w.client.EscapeEvent.Query().Where(escapeevent.SessionID(session)).Count(ctx)
	require.NoError(t, err)
	return n
}

// The slack lets a session overshoot the cap between trims, but never the
// threshold; a trim lands exactly on the cap and keeps the newest rows. The
// writer's cached count must equal a brute-force COUNT after every flush.
func TestFlushBatch_SlackBoundsOvershootAndTrimsToCap_KeepingNewest(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	const cap = 100
	threshold := escapeEvictionThreshold(cap)
	w := NewEscapeEventBatchWriter(client, cap)
	base := time.Now().Add(-time.Hour)
	next := map[string]int{}
	trims := 0
	for step := 0; step < 60; step++ {
		for _, s := range []string{"a", "b"} {
			n := 3 + step%5
			before := dbCount(t, ctx, w, s)
			w.flushBatch(ctx, escapeBatch(s, base, next[s], n))
			next[s] += n
			got := dbCount(t, ctx, w, s)
			require.LessOrEqual(t, got, threshold, "session %s overshoot", s)
			require.Equal(t, got, w.sessionRowCounts[s], "cached count drifted from COUNT(*) for %s", s)
			if got == cap && before+n > threshold {
				trims++
			}
		}
	}
	assert.Positive(t, trims, "scenario should have triggered trims")

	// Force a final trim, then check the cap and the newest-survive invariant.
	for prev := -1; prev < dbCount(t, ctx, w, "a") || prev == -1; {
		prev = dbCount(t, ctx, w, "a")
		w.flushBatch(ctx, escapeBatch("a", base, next["a"], 1))
		next["a"]++
	}
	require.Equal(t, cap, dbCount(t, ctx, w, "a"))
	rows, err := client.EscapeEvent.Query().Where(escapeevent.SessionID("a")).Order(escapeevent.ByWallTime()).All(ctx)
	require.NoError(t, err)
	for i, r := range rows {
		assert.Equal(t, int64(next["a"]-cap+i), r.SessionSeq, "row %d should be among the newest", i)
	}
}

// Steady state at the cap must not cost a delete round trip per flush.
func TestFlushBatch_AtCap_IssuesFewStatementsPerFlush(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	const cap, flushes = 100, 300
	w := NewEscapeEventBatchWriter(client, cap)
	base := time.Now().Add(-time.Hour)
	w.flushBatch(ctx, escapeBatch("s", base, 0, cap))
	countCtx, counter := dbobs.WithQueryCounter(ctx)
	for i := 0; i < flushes; i++ {
		w.flushBatch(countCtx, escapeBatch("s", base, cap+i, 1))
	}
	// Per-flush trimming issued INSERT + select + delete (3) per flush: 900.
	assert.Less(t, counter.Count(), int64(flushes*3/2), "statements over %d flushes", flushes)
	assert.LessOrEqual(t, dbCount(t, ctx, w, "s"), escapeEvictionThreshold(cap))
}

// Eviction yields while a full batch is queued, and finishes on a later flush.
func TestFlushBatch_EvictionYieldsToQueuedBatch_ThenResumes(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	const cap = 300
	w := NewEscapeEventBatchWriter(client, cap)
	base := time.Now().Add(-time.Hour)
	for i := 0; i < escapeFlushBatchSize; i++ { // a full batch waiting behind this flush
		w.ch <- escapeBatch("s", base, 0, 1)[0]
	}
	w.flushBatch(ctx, escapeBatch("s", base, 0, 1000)) // excess 700 = three chunks
	assert.Equal(t, 1000-escapeEvictChunkRows, dbCount(t, ctx, w, "s"), "one chunk, then yield")
	assert.Equal(t, 1000-escapeEvictChunkRows, w.sessionRowCounts["s"])

	for len(w.ch) > 0 {
		<-w.ch
	}
	w.flushBatch(ctx, escapeBatch("s", base, 1000, 1))
	assert.Equal(t, cap, dbCount(t, ctx, w, "s"))
}

// Statements issued for a large bulk insert carry the writer's name, not "unknown".
func TestFlushBatch_BulkInsertIsAttributedToWriter(t *testing.T) {
	ctx := context.Background()
	client := openForTest(ctx, t, t.TempDir())
	defer client.Close()

	w := NewEscapeEventBatchWriter(client, 0)
	w.flushBatch(ctx, escapeBatch("s", time.Now(), 0, 100))

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

// Events queued at shutdown are persisted, not lost to the cancelled context.
func TestStart_ShutdownDrainPersistsQueuedEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := openForTest(context.Background(), t, t.TempDir())
	defer client.Close()

	w := NewEscapeEventBatchWriter(client, 100)
	for _, ev := range escapeBatch("s", time.Now(), 0, 7) {
		w.WriteEscapeEvent(ctx, ev)
	}
	cancel()
	w.Start(ctx) // drains and returns

	assert.Equal(t, 7, dbCount(t, context.Background(), w, "s"))
}
