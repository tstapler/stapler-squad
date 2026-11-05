package escapedb

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
)

// The driver persists Time.String(); a monotonic reading would add an " m=+..." suffix per row.
func TestFlushBatch_StoresWallTimeWithoutMonotonicSuffix(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	client := openForTest(ctx, t, dir)

	w := NewEscapeEventBatchWriter(client, 100)
	w.flushBatch(ctx, []pkganalytics.EscapeEventRecord{{
		SessionID: "s1", Stage: "input", SequenceType: "csi", ByteLen: 1,
		WallTime: time.Now(), SessionSeq: 1,
	}})
	n, err := client.EscapeEvent.Query().Where(escapeevent.SessionID("s1")).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	db, err := sql.Open("sqlite", "file:"+Path(dir)+"?mode=ro")
	require.NoError(t, err)
	defer db.Close()
	var stored string
	require.NoError(t, db.QueryRow("SELECT CAST(wall_time AS TEXT) FROM escape_events").Scan(&stored))
	assert.NotContains(t, stored, " m=")
}
