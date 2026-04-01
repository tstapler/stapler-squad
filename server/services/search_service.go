package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/telemetry"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SearchService handles all Claude history and full-text search RPC methods.
//
// It owns the history cache and search engine state that were previously
// scattered across SessionService.
//
// Bug note: historyCacheMu protects the history cache fields from concurrent
// access. Without this, concurrent ListClaudeHistory calls would race on cache
// refresh (previously unprotected on SessionService).
type SearchService struct {
	searchEngine     *search.SearchEngine
	snippetGenerator *search.SnippetGenerator

	historyCacheMu   sync.RWMutex
	historyCache     *session.ClaudeSessionHistory
	historyCacheTime time.Time
	historyCacheTTL  time.Duration
}

// NewSearchService creates a SearchService with the given search components.
func NewSearchService(
	searchEngine *search.SearchEngine,
	snippetGenerator *search.SnippetGenerator,
	historyCacheTTL time.Duration,
) *SearchService {
	return &SearchService{
		searchEngine:     searchEngine,
		snippetGenerator: snippetGenerator,
		historyCacheTTL:  historyCacheTTL,
	}
}

// getOrRefreshHistoryCache returns the cached history or refreshes it if stale.
func (ss *SearchService) getOrRefreshHistoryCache(ctx context.Context) (*session.ClaudeSessionHistory, error) {
	ctx, span := telemetry.StartSpan(ctx, "SearchService.getOrRefreshHistoryCache")
	defer span.End()

	now := time.Now()

	// Fast path: check with read lock first.
	ss.historyCacheMu.RLock()
	if ss.historyCache != nil && now.Sub(ss.historyCacheTime) < ss.historyCacheTTL {
		cached := ss.historyCache
		span.SetAttributes(
			attribute.Bool("cache.hit", true),
			attribute.Int("history.entry_count", cached.Count()),
		)
		ss.historyCacheMu.RUnlock()
		return cached, nil
	}
	ss.historyCacheMu.RUnlock()

	// Cache is stale or doesn't exist — refresh with write lock.
	ss.historyCacheMu.Lock()
	defer ss.historyCacheMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed).
	if ss.historyCache != nil && now.Sub(ss.historyCacheTime) < ss.historyCacheTTL {
		span.SetAttributes(attribute.Bool("cache.hit", true))
		return ss.historyCache, nil
	}

	span.SetAttributes(attribute.Bool("cache.hit", false))

	_, loadSpan := telemetry.StartSpan(ctx, "SearchService.loadHistoryFromDisk")
	loadStart := time.Now()

	hist, err := session.NewClaudeSessionHistoryFromClaudeDir()

	loadDuration := time.Since(loadStart)
	loadSpan.SetAttributes(attribute.Int64("load.duration_ms", loadDuration.Milliseconds()))
	if err != nil {
		loadSpan.RecordError(err)
		loadSpan.End()
		return nil, fmt.Errorf("failed to create history manager: %w", err)
	}
	loadSpan.SetAttributes(attribute.Int("history.entry_count", hist.Count()))
	loadSpan.End()

	ss.historyCache = hist
	ss.historyCacheTime = now

	span.SetAttributes(
		attribute.Int("history.entry_count", hist.Count()),
		attribute.Int64("cache.refresh_duration_ms", time.Since(now).Milliseconds()),
	)

	log.InfoLog.Printf("History cache refreshed: %d entries in %v", hist.Count(), time.Since(now))
	return hist, nil
}

// ListClaudeHistory returns Claude session history entries with optional filtering.
func (ss *SearchService) ListClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.ListClaudeHistoryRequest],
) (*connect.Response[sessionv1.ListClaudeHistoryResponse], error) {
	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	var entries []session.ClaudeHistoryEntry

	if req.Msg.Project != nil && *req.Msg.Project != "" {
		entries = hist.GetByProject(*req.Msg.Project)
	} else if req.Msg.SearchQuery != nil && *req.Msg.SearchQuery != "" {
		entries = hist.Search(*req.Msg.SearchQuery)
	} else {
		entries = hist.GetAll()
	}

	totalCount := len(entries)
	if req.Msg.Limit > 0 && int(req.Msg.Limit) < len(entries) {
		entries = entries[:req.Msg.Limit]
	}

	protoEntries := make([]*sessionv1.ClaudeHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		protoEntries = append(protoEntries, &sessionv1.ClaudeHistoryEntry{
			Id:           entry.ID,
			Name:         entry.Name,
			Project:      entry.Project,
			CreatedAt:    timestamppb.New(entry.CreatedAt),
			UpdatedAt:    timestamppb.New(entry.UpdatedAt),
			Model:        entry.Model,
			MessageCount: int32(entry.MessageCount),
		})
	}

	return connect.NewResponse(&sessionv1.ListClaudeHistoryResponse{
		Entries:    protoEntries,
		TotalCount: int32(totalCount),
	}), nil
}

// GetClaudeHistoryDetail retrieves detailed information for a specific history entry.
func (ss *SearchService) GetClaudeHistoryDetail(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryDetailRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryDetailResponse], error) {
	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	entry, err := hist.GetByID(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&sessionv1.GetClaudeHistoryDetailResponse{
		Entry: &sessionv1.ClaudeHistoryEntry{
			Id:           entry.ID,
			Name:         entry.Name,
			Project:      entry.Project,
			CreatedAt:    timestamppb.New(entry.CreatedAt),
			UpdatedAt:    timestamppb.New(entry.UpdatedAt),
			Model:        entry.Model,
			MessageCount: int32(entry.MessageCount),
		},
	}), nil
}

// GetClaudeHistoryMessages retrieves messages from a specific conversation.
func (ss *SearchService) GetClaudeHistoryMessages(
	ctx context.Context,
	req *connect.Request[sessionv1.GetClaudeHistoryMessagesRequest],
) (*connect.Response[sessionv1.GetClaudeHistoryMessagesResponse], error) {
	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	_, err = hist.GetByID(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %w", err))
	}

	messages, err := hist.GetMessagesFromConversationFile(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load messages: %w", err))
	}

	totalCount := len(messages)
	offset := int(req.Msg.Offset)
	limit := int(req.Msg.Limit)

	if offset > 0 && offset < len(messages) {
		messages = messages[offset:]
	}
	if limit > 0 && limit < len(messages) {
		messages = messages[:limit]
	}

	protoMessages := make([]*sessionv1.ClaudeMessage, 0, len(messages))
	for _, msg := range messages {
		protoMessages = append(protoMessages, &sessionv1.ClaudeMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: timestamppb.New(msg.Timestamp),
			Model:     msg.Model,
		})
	}

	return connect.NewResponse(&sessionv1.GetClaudeHistoryMessagesResponse{
		Messages:   protoMessages,
		TotalCount: int32(totalCount),
	}), nil
}

// SearchClaudeHistory performs full-text search across Claude conversation history.
func (ss *SearchService) SearchClaudeHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(telemetry.AttrSearchQuery, req.Msg.Query),
		attribute.Int("search.limit", int(req.Msg.Limit)),
		attribute.Int("search.offset", int(req.Msg.Offset)),
	)

	if req.Msg.Query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	_, syncSpan := telemetry.StartSpan(ctx, "SearchEngine.IncrementalSync")
	syncStart := time.Now()
	syncResult, err := ss.searchEngine.IncrementalSync(hist)
	syncDuration := time.Since(syncStart)
	syncSpan.SetAttributes(
		attribute.Int64("sync.duration_ms", syncDuration.Milliseconds()),
		attribute.Bool("sync.was_full_rebuild", syncResult.WasFullRebuild),
		attribute.Int("sync.sessions_added", syncResult.SessionsAdded),
		attribute.Int("sync.sessions_updated", syncResult.SessionsUpdated),
		attribute.Int("sync.sessions_removed", syncResult.SessionsRemoved),
	)
	if err != nil {
		syncSpan.RecordError(err)
		syncSpan.End()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to sync search index: %w", err))
	}
	syncSpan.End()

	if syncResult.HasChanges() || syncResult.WasFullRebuild {
		log.InfoLog.Printf("Search index sync: %s", syncResult.String())
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset := int(req.Msg.Offset)
	if offset < 0 {
		offset = 0
	}

	searchOpts := search.SearchOptions{
		Limit:  limit,
		Offset: offset,
	}

	_, searchSpan := telemetry.StartSpan(ctx, "SearchEngine.Search")
	searchStart := time.Now()
	searchResults, err := ss.searchEngine.Search(req.Msg.Query, searchOpts)
	searchDuration := time.Since(searchStart)
	searchSpan.SetAttributes(
		attribute.Int64("search.duration_ms", searchDuration.Milliseconds()),
		attribute.String("search.query", req.Msg.Query),
	)
	if err != nil {
		searchSpan.RecordError(err)
		searchSpan.End()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("search failed: %w", err))
	}
	searchSpan.SetAttributes(
		attribute.Int("search.result_count", len(searchResults.Results)),
		attribute.Int("search.total_matches", searchResults.TotalMatches),
	)
	searchSpan.End()

	tokenizer := ss.searchEngine.GetTokenizer()
	queryTokens := tokenizer.Tokenize(req.Msg.Query)

	protoResults := make([]*sessionv1.SearchResult, 0, len(searchResults.Results))
	for _, result := range searchResults.Results {
		entry, _ := hist.GetByID(result.SessionID)

		doc := ss.searchEngine.GetDocument(result.DocID)
		snippets := ss.snippetGenerator.GenerateFromSearchResult(doc, queryTokens)

		protoSnippets := make([]*sessionv1.SearchSnippet, 0, len(snippets))
		for _, snippet := range snippets {
			highlightRanges := make([]*sessionv1.HighlightRange, 0, len(snippet.HighlightRanges))
			for _, hr := range snippet.HighlightRanges {
				highlightRanges = append(highlightRanges, &sessionv1.HighlightRange{
					Start: int32(hr.Start),
					End:   int32(hr.End),
				})
			}
			protoSnippets = append(protoSnippets, &sessionv1.SearchSnippet{
				Text:            snippet.Text,
				HighlightRanges: highlightRanges,
				MessageRole:     snippet.MessageRole,
				MessageTime:     timestamppb.New(snippet.MessageTime),
			})
		}

		sessionName := result.SessionID
		project := ""
		model := ""
		var createdAt time.Time
		if entry != nil {
			sessionName = entry.Name
			project = entry.Project
			model = entry.Model
			createdAt = entry.CreatedAt
		}

		protoResults = append(protoResults, &sessionv1.SearchResult{
			SessionId:    result.SessionID,
			SessionName:  sessionName,
			Project:      project,
			MessageIndex: int32(result.MessageIndex),
			Score:        float32(result.Score),
			Snippets:     protoSnippets,
			Metadata: &sessionv1.SearchResultMetadata{
				IsMetadataMatch: false,
				MatchSource:     "message_content",
				Model:           model,
				CreatedAt:       timestamppb.New(createdAt),
			},
		})
	}

	return connect.NewResponse(&sessionv1.SearchClaudeHistoryResponse{
		Results:      protoResults,
		TotalMatches: int32(searchResults.TotalMatches),
		QueryTimeMs:  searchResults.QueryTime.Milliseconds(),
		HasMore:      searchResults.TotalMatches > offset+len(protoResults),
	}), nil
}
