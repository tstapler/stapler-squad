package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/server/analytics/escapedb"
	"github.com/tstapler/stapler-squad/server/events"
)

// Summaries keep counting events whose raw rows were evicted, because they read rollups.
func TestGetEscapeAnalyticsGlobalSummary_CountsEventsWhoseRawRowsWereEvicted(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client, db, err := escapedb.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	retention := escapedb.NewRetention(client, db.Driver(), escapedb.RetentionConfig{RawRetentionDays: 2})
	retention.Run(context.Background()) // stamps live counting and completes the (empty) backfill
	writer := escapedb.NewEscapeEventBatchWriter(db.Driver())
	old := time.Now().AddDate(0, 0, -10)
	for i := range 6 {
		writer.WriteEscapeEvent(ctx, pkganalytics.EscapeEventRecord{
			SessionID: "s1", Stage: pkganalytics.StagePTYRead, SequenceType: "SGR", ByteLen: 2,
			WallTime: old.Add(time.Duration(i) * time.Second), SessionSeq: int64(i),
		})
	}
	cancel()
	writer.Start(ctx) // drains the queue on shutdown
	retention.Run(context.Background())
	remaining, err := client.EscapeEvent.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, remaining, "raw rows evicted")

	svc := NewSessionService(createTestStorage(t), events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	svc.SetEscapeEventsClient(client)
	svc.SetEscapeAggregateReader(escapedb.NewReader(client, db.Driver(), escapedb.RollupTiers{}, nil))

	resp, err := svc.GetEscapeAnalyticsGlobalSummary(context.Background(), connect.NewRequest(&sessionv1.GetEscapeAnalyticsGlobalSummaryRequest{}))

	require.NoError(t, err)
	assert.EqualValues(t, 6, resp.Msg.TotalSequences)
}
