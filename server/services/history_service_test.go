package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/search"
)

// setupSearchService creates a SearchService with a fresh in-memory search
// engine and a short cache TTL so tests do not block on cache expiry.
func setupSearchService() *SearchService {
	return NewSearchService(
		search.NewSearchEngine(),
		search.NewSnippetGenerator(),
		5*time.Minute,
	)
}

// TestListClaudeHistory_EmptyDir verifies that ListClaudeHistory succeeds and
// returns an empty (or existing) list even when the history file does not
// exist.  The RPC must not return an error — a missing ~/.claude/history.jsonl
// is treated as empty history.
func TestListClaudeHistory_EmptyDir(t *testing.T) {
	svc := setupSearchService()

	resp, err := svc.ListClaudeHistory(
		context.Background(),
		connect.NewRequest(&sessionv1.ListClaudeHistoryRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	// Entries may be non-nil (zero-length slice is valid).
	assert.NotNil(t, resp.Msg.Entries)
}

// TestGetHistoryDetail_EmptyID verifies that GetClaudeHistoryDetail with an
// empty session ID returns CodeNotFound.
func TestGetHistoryDetail_EmptyID(t *testing.T) {
	svc := setupSearchService()

	_, err := svc.GetClaudeHistoryDetail(
		context.Background(),
		connect.NewRequest(&sessionv1.GetClaudeHistoryDetailRequest{
			Id: "",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestGetHistoryMessages_EmptyID verifies that GetClaudeHistoryMessages with
// an empty ID returns CodeNotFound.
func TestGetHistoryMessages_EmptyID(t *testing.T) {
	svc := setupSearchService()

	_, err := svc.GetClaudeHistoryMessages(
		context.Background(),
		connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
			Id: "",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestSearchHistory_EmptyQuery verifies that SearchClaudeHistory with an empty
// query string returns CodeInvalidArgument (query is required).
func TestSearchHistory_EmptyQuery(t *testing.T) {
	svc := setupSearchService()

	_, err := svc.SearchClaudeHistory(
		context.Background(),
		connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
			Query: "",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
