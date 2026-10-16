package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// TestDeleteOldestRows_QueryPlanAvoidsSort guards the CPU fix: with a session at its
// row cap, every flush runs "oldest N by wall_time for one session". That must be an
// ordered index scan, not a read-and-temp-sort of all the session's rows.
func TestDeleteOldestRows_QueryPlanAvoidsSort(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	client, err := OpenAnalyticsDB(ctx, dir)
	require.NoError(t, err)
	defer client.Close()

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "analytics.db")+"?mode=ro")
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
	client, err := OpenAnalyticsDB(ctx, t.TempDir())
	require.NoError(t, err)
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
