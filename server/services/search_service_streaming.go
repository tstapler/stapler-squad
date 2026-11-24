package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const (
	// streamSearchBatchSize is how many results go in each streamed message.
	streamSearchBatchSize = 10
	// streamSearchWarmupTimeout bounds how long a cold-start stream waits for
	// the first index sync before giving up with CodeUnavailable.
	streamSearchWarmupTimeout = 60 * time.Second
)

// StreamSearchClaudeHistory is the server-streaming variant of
// SearchClaudeHistory. Results are flushed in batches of streamSearchBatchSize
// so the client can render the top hits immediately; the last message has
// is_final = true. When the index has not completed its first sync, an empty
// index_warming message is sent right away and the search runs once the
// background sync finishes.
func (ss *SearchService) StreamSearchClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
	stream *connect.ServerStream[sessionv1.SearchClaudeHistoryStreamResponse],
) error {
	if req.Msg.Query == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	if !ss.indexSynced() {
		if err := stream.Send(&sessionv1.SearchClaudeHistoryStreamResponse{IndexWarming: true}); err != nil {
			return err
		}
		if err := ss.waitForFirstSync(ctx); err != nil {
			return err
		}
	}

	resp, err := ss.searchClaudeHistory(ctx, req.Msg)
	if err != nil {
		return err
	}

	results := resp.Results
	for {
		n := min(len(results), streamSearchBatchSize)
		batch := results[:n]
		results = results[n:]
		final := len(results) == 0
		if err := stream.Send(&sessionv1.SearchClaudeHistoryStreamResponse{
			Results:      batch,
			TotalMatches: resp.TotalMatches,
			QueryTimeMs:  resp.QueryTimeMs,
			HasMore:      resp.HasMore,
			IsFinal:      final,
		}); err != nil {
			return err
		}
		if final {
			return nil
		}
	}
}

func (ss *SearchService) waitForFirstSync(ctx context.Context) error {
	timer := time.NewTimer(streamSearchWarmupTimeout)
	defer timer.Stop()
	select {
	case <-ss.syncedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("search index is still warming up; retry shortly"))
	}
}
