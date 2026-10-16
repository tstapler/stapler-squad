package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/contexthistory"
	"github.com/tstapler/stapler-squad/session/tokens"
)

func newContextHistorySvc(t *testing.T) (*InsightsService, *contexthistory.Recorder) {
	t.Helper()
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(t.TempDir(), "s.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	store := contexthistory.NewStore(repo.GetEntClient())

	svc := NewInsightsService(&fakeTokenStore{}, tokens.DefaultPricingTable(), nil, nil)
	svc.SetContextHistoryStore(store)
	return svc, contexthistory.NewRecorder(store, func(string) int { return 100 }, 0)
}

func recentTurns(inputs ...int64) []tokens.TurnStats {
	base := time.Now().Add(-time.Hour)
	out := make([]tokens.TurnStats, len(inputs))
	for i, in := range inputs {
		out[i] = tokens.TurnStats{Timestamp: base.Add(time.Duration(i) * time.Minute), Model: "m", Input: in}
	}
	return out
}

func TestContextHistoryRPCs_ReturnSeriesRankingAndCompactionStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, rec := newContextHistorySvc(t)

	hot := &tokens.ParseResult{SessionUUID: "hot", TurnTimeline: recentTurns(10, 80, 95, 99),
		CompactEvents: []tokens.CompactEvent{{Timestamp: time.Now().Add(-time.Minute), Trigger: "manual", TurnIndex: 2, TokensBefore: 95, TokensAfter: 20}}}
	cool := &tokens.ParseResult{SessionUUID: "cool", TurnTimeline: recentTurns(5, 6)}
	require.NoError(t, rec.Record(ctx, hot))
	require.NoError(t, rec.Record(ctx, cool))

	hist, err := svc.GetContextHistory(ctx, connect.NewRequest(&sessionv1.GetContextHistoryRequest{ConversationId: "hot"}))
	require.NoError(t, err)
	assert.Len(t, hist.Msg.Samples, 4)
	assert.Equal(t, int32(4), hist.Msg.Summary.Turns)
	assert.InDelta(t, 0.75, hist.Msg.Summary.FractionAtOrAbove_75, 1e-9)
	assert.InDelta(t, 0.5, hist.Msg.Summary.FractionAtOrAbove_90, 1e-9)
	require.Len(t, hist.Msg.Compactions, 1)
	assert.Equal(t, int64(75), hist.Msg.Compactions[0].TokensFreed)

	ranked, err := svc.ListSessionsByCeilingTime(ctx, connect.NewRequest(&sessionv1.ListSessionsByCeilingTimeRequest{}))
	require.NoError(t, err)
	require.Len(t, ranked.Msg.Sessions, 2)
	assert.Equal(t, "hot", ranked.Msg.Sessions[0].ConversationId)

	stats, err := svc.GetCompactionStats(ctx, connect.NewRequest(&sessionv1.GetCompactionStatsRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), stats.Msg.Count)
	assert.Equal(t, int32(1), stats.Msg.ManualCount)
	assert.Equal(t, int64(75), stats.Msg.TotalTokensFreed)

	future, err := svc.GetCompactionStats(ctx, connect.NewRequest(&sessionv1.GetCompactionStatsRequest{Since: timestamppb.New(time.Now().Add(time.Hour))}))
	require.NoError(t, err)
	assert.Zero(t, future.Msg.Count)
}

func TestContextHistoryRPCs_UnavailableAndInvalid(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bare := NewInsightsService(&fakeTokenStore{}, tokens.DefaultPricingTable(), nil, nil)
	_, err := bare.GetContextHistory(ctx, connect.NewRequest(&sessionv1.GetContextHistoryRequest{ConversationId: "x"}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

	svc, _ := newContextHistorySvc(t)
	_, err = svc.GetContextHistory(ctx, connect.NewRequest(&sessionv1.GetContextHistoryRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
