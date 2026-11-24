package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

// streamClient serves only the streaming search RPC from a SearchService.
type streamSearchHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	ss *SearchService
}

func (h streamSearchHandler) StreamSearchClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
	stream *connect.ServerStream[sessionv1.SearchClaudeHistoryStreamResponse],
) error {
	return h.ss.StreamSearchClaudeHistory(ctx, req, stream)
}

func newStreamClient(t *testing.T, ss *SearchService) sessionv1connect.SessionServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(streamSearchHandler{ss: ss}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
}

func seedManySessions(t *testing.T, n int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions := map[string]testConvSession{}
	for i := 0; i < n; i++ {
		sessions["sess-"+string(rune('a'+i))] = testConvSession{project: "/repo", messages: []testConvMsg{
			{role: "user", content: "streaming widget search", time: base},
		}}
	}
	seedClaudeHome(t, sessions)
}

func collectStream(t *testing.T, client sessionv1connect.SessionServiceClient, q string) []*sessionv1.SearchClaudeHistoryStreamResponse {
	t.Helper()
	st, err := client.StreamSearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: q, Limit: 50}))
	require.NoError(t, err)
	var msgs []*sessionv1.SearchClaudeHistoryStreamResponse
	for st.Receive() {
		msgs = append(msgs, st.Msg())
	}
	require.NoError(t, st.Err())
	return msgs
}

func TestStreamSearchClaudeHistory_should_EmitBatchesAndFinalFlag_When_IndexWarm(t *testing.T) {
	seedManySessions(t, 12)
	ss := setupSyncedSearchService(t)

	msgs := collectStream(t, newStreamClient(t, ss), "widget")

	require.Len(t, msgs, 2, "12 results at batch size 10 → 2 messages")
	require.Len(t, msgs[0].Results, 10)
	require.False(t, msgs[0].IsFinal)
	require.Len(t, msgs[1].Results, 2)
	require.True(t, msgs[1].IsFinal)
}

func TestStreamSearchClaudeHistory_should_EmitFinalEmptyMessage_When_NoMatches(t *testing.T) {
	seedManySessions(t, 1)
	ss := setupSyncedSearchService(t)

	msgs := collectStream(t, newStreamClient(t, ss), "nonexistentterm")

	require.Len(t, msgs, 1)
	require.True(t, msgs[0].IsFinal)
}

func TestStreamSearchClaudeHistory_should_SendWarmingBatchBeforeSyncCompletes_When_ColdStart(t *testing.T) {
	seedManySessions(t, 3)
	ss := setupSearchService() // never synced
	client := newStreamClient(t, ss)

	st, err := client.StreamSearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: "widget"}))
	require.NoError(t, err)

	require.True(t, st.Receive(), "first message must arrive while the index is still unsynced")
	require.True(t, st.Msg().IndexWarming)
	require.False(t, st.Msg().IsFinal)
	require.False(t, ss.indexSynced())

	require.NoError(t, ss.SyncNow(t.Context()))

	var last *sessionv1.SearchClaudeHistoryStreamResponse
	for st.Receive() {
		last = st.Msg()
	}
	require.NoError(t, st.Err())
	require.NotNil(t, last)
	require.True(t, last.IsFinal)
	require.Len(t, last.Results, 3)
}

func TestSearchClaudeHistory_should_ReturnUnavailable_When_IndexNeverSynced(t *testing.T) {
	seedManySessions(t, 1)
	ss := setupSearchService()

	_, err := ss.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: "widget"}))

	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.Contains(t, err.Error(), "warming up")
}

func TestSearchService_should_IndexNewSessions_When_BackgroundSyncLoopRuns(t *testing.T) {
	seedManySessions(t, 1)
	ss := setupSearchService()
	ss.SetSyncInterval(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ss.StartSyncLoop(ctx)

	require.Eventually(t, ss.indexSynced, 5*time.Second, 10*time.Millisecond)
	resp, err := ss.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: "widget"}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1)
}
