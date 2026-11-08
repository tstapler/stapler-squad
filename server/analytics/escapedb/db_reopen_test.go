package escapedb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reopening keeps the rollups and the original live_since cut.
func TestOpen_Reopen_KeepsRollupsAndLiveSince(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	client, db, err := Open(ctx, dir)
	require.NoError(t, err)
	require.NoError(t, setLiveSince(ctx, db.Driver(), testBase))
	NewEscapeEventBatchWriter(db.Driver()).flushBatch(ctx, genEvents(testBase.Add(time.Hour), 6, time.Minute))
	require.NoError(t, client.Close())

	client, db, err = Open(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	_, events, _ := rollupTotals(t, ctx, db.Driver())
	assert.EqualValues(t, 6, events)
	st, err := readRollupState(ctx, db.Driver())
	require.NoError(t, err)
	assert.Equal(t, testBase.UnixMilli(), st.LiveSince.UnixMilli())
}
