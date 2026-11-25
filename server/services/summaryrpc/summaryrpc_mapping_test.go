package summaryrpc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/handoffsummary"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// gatedPool records whether the context it receives was already cancelled once
// its gate opens, i.e. after the request context ended.
type gatedPool struct {
	gate    chan struct{}
	ctxErr  chan error
	payload string
}

func (p *gatedPool) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, sink headless.CostSink) (string, error) {
	<-p.gate
	p.ctxErr <- ctx.Err()
	sink(0, true)
	return p.payload, nil
}

func TestTriggerHandoffSummary_should_RunPipelineOnDetachedContext_When_RequestContextEnds(t *testing.T) {
	sessionID := "sess-detached"
	svctest.WithIsolatedStateDirPath(t, t.TempDir())
	writeHandoffConversationFixture(t, sessionID)

	entClient := newTestHandoffSummaryEntClient(t)
	pool := &gatedPool{gate: make(chan struct{}), ctxErr: make(chan error, 1), payload: "text"}
	svc := NewHandoffSummaryService(summaries.NewHandoffSummaryGenerator(entClient, pool))

	reqCtx, cancel := context.WithCancel(context.Background())
	_, err := svc.TriggerHandoffSummary(reqCtx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	cancel()
	close(pool.gate)

	select {
	case got := <-pool.ctxErr:
		require.NoError(t, got, "the dispatched pipeline must not inherit the request context")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the dispatched pipeline to call the pool")
	}
	wait.RequireEventually(t, func() bool {
		row, qerr := entClient.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(context.Background())
		return qerr == nil && (row.Status == "ready" || row.Status == "error")
	}, 5*time.Second, 10*time.Millisecond, "timed out waiting for %s to reach a terminal status", sessionID)
}

func TestToSummaryProto_should_ReturnNil_When_RowIsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, toSessionSummaryProto(nil))
	require.Nil(t, toHandoffSummaryProto(nil))
}

func TestToSessionSummaryProto_should_LeaveOptionalFieldsZero_When_RowHasNone(t *testing.T) {
	t.Parallel()
	p := toSessionSummaryProto(&ent.SessionSummary{SessionID: "s", Status: "ready"})
	require.Nil(t, p.Timeline.StartedAt)
	require.Nil(t, p.Timeline.StoppedAt)
	require.Nil(t, p.GeneratedAt)
	require.Zero(t, p.Timeline.DurationMs)
	require.Zero(t, p.Cost.TotalTokens)
	require.Zero(t, p.Cost.EstimatedCostUsd)
}

func TestToSessionSummaryProto_should_CopyOptionalFields_When_RowHasThem(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dur, tok, cost := int64(1500), int64(99), 0.5
	p := toSessionSummaryProto(&ent.SessionSummary{
		SessionStartedAt: &ts, SessionStoppedAt: &ts, GeneratedAt: &ts,
		DurationMs: &dur, TotalTokens: &tok, EstimatedCostUsd: &cost,
	})
	require.True(t, p.Timeline.StartedAt.AsTime().Equal(ts))
	require.True(t, p.Timeline.StoppedAt.AsTime().Equal(ts))
	require.True(t, p.GeneratedAt.AsTime().Equal(ts))
	require.Equal(t, int64(1500), p.Timeline.DurationMs)
	require.Equal(t, int64(99), p.Cost.TotalTokens)
	require.InDelta(t, 0.5, p.Cost.EstimatedCostUsd, 0)
}

func TestToHandoffSummaryProto_should_CopyTimestamps_When_RowHasThem(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := toHandoffSummaryProto(&summaries.HandoffSummary{GeneratedAt: &ts, GenerationStartedAt: &ts, MiddleMessagesSummarized: 7})
	require.True(t, p.GeneratedAt.AsTime().Equal(ts))
	require.True(t, p.GenerationStartedAt.AsTime().Equal(ts))
	require.Equal(t, int32(7), p.MiddleMessagesSummarized)
	require.Nil(t, toHandoffSummaryProto(&summaries.HandoffSummary{}).GeneratedAt)
}

func TestSummaryStatusProto_should_MapKnownValuesAndDefaultToUnspecified(t *testing.T) {
	t.Parallel()
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_UNSPECIFIED, toSessionSummaryStatusProto("nope"))
	require.Equal(t, sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_UNSPECIFIED, toHandoffSummaryStatusProto("nope"))
	require.Equal(t, sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_ERROR, toSessionSummaryStatusProto("error"))
	require.Equal(t, sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_PENDING, toHandoffSummaryStatusProto("pending"))
}
