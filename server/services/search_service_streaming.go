package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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
		if err := stream.Send(&sessionv1.SearchClaudeHistoryStreamResponse{
			IndexWarming: true,
			Results:      ss.metadataPreview(ctx, req.Msg),
		}); err != nil {
			return err
		}
		if err := ss.waitForFirstSync(ctx); err != nil {
			return err
		}
	}

	raw, err := ss.runRawSearch(ctx, req.Msg)
	if err != nil {
		return err
	}

	if raw.needsPostProcessing || req.Msg.GetIncludeContext() {
		// Dedup/filter/context need the full candidate set, so enrich it all
		// up front and only chunk the delivery.
		return ss.streamBatches(stream, raw, ss.finishPostProcessed(raw, req.Msg), nil)
	}
	return ss.streamBatches(stream, raw, nil, req.Msg)
}

// metadataPreview returns up to one batch of session-name/project matches from
// the in-memory history cache, so a cold-start stream shows real hits before the
// index finishes its first sync. The first ranked batch replaces them. A load
// failure yields no preview; the stream still waits for the sync. Requests with
// filters or an offset get none, since the preview cannot honor them.
func (ss *SearchService) metadataPreview(ctx context.Context, msg *sessionv1.SearchClaudeHistoryRequest) []*sessionv1.SearchResult {
	if msg.GetOffset() > 0 || msg.GetProject() != "" || msg.GetModel() != "" ||
		msg.GetStartTime() != nil || msg.GetEndTime() != nil || msg.GetExcludeAutomationSessions() {
		return nil
	}
	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil
	}
	entries := hist.Search(msg.Query)
	if len(entries) > streamSearchBatchSize {
		entries = entries[:streamSearchBatchSize]
	}
	out := make([]*sessionv1.SearchResult, 0, len(entries))
	for _, e := range entries {
		out = append(out, &sessionv1.SearchResult{
			SessionId:   e.ID,
			SessionName: e.Name,
			Project:     e.Project,
			Metadata: &sessionv1.SearchResultMetadata{
				IsMetadataMatch: true,
				MatchSource:     "metadata_preview",
				Model:           e.Model,
				CreatedAt:       timestamppb.New(e.CreatedAt),
			},
		})
	}
	return out
}

// finishPostProcessed enriches every raw hit and applies the post-processing
// pipeline, as the unary RPC does.
func (ss *SearchService) finishPostProcessed(raw *rawSearch, msg *sessionv1.SearchClaudeHistoryRequest) []*sessionv1.SearchResult {
	protos := ss.buildProtoResults(raw.results.Results, msg.Query, raw.hist)
	out := ss.applyResultPostProcessing(protos, msg, raw.hist, raw.requestedLimit, raw.needsPostProcessing)
	if out == nil {
		out = []*sessionv1.SearchResult{} // nil would select the lazy path in streamBatches
	}
	return out
}

// streamBatches sends results in batches of streamSearchBatchSize. When
// prebuilt is nil the raw hits are enriched one batch at a time, so the first
// message does not wait on snippet generation for the rest.
func (ss *SearchService) streamBatches(
	stream *connect.ServerStream[sessionv1.SearchClaudeHistoryStreamResponse],
	raw *rawSearch,
	prebuilt []*sessionv1.SearchResult,
	lazyMsg *sessionv1.SearchClaudeHistoryRequest,
) error {
	hits := raw.results.Results
	total := len(prebuilt)
	if prebuilt == nil {
		total = len(hits)
	}
	sent := 0
	for {
		n := min(total-sent, streamSearchBatchSize)
		var batch []*sessionv1.SearchResult
		if prebuilt != nil {
			batch = prebuilt[sent : sent+n]
		} else {
			batch = ss.buildProtoResults(hits[sent:sent+n], lazyMsg.Query, raw.hist)
		}
		sent += n
		final := sent >= total
		if err := stream.Send(&sessionv1.SearchClaudeHistoryStreamResponse{
			Results: batch,
			// #nosec G115 -- TotalMatches is a hit count far below int32 range.
			TotalMatches: int32(raw.results.TotalMatches),
			QueryTimeMs:  raw.results.QueryTime.Milliseconds(),
			HasMore:      raw.results.TotalMatches > raw.offset+total,
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
