package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/telemetry"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// historyCursor is the opaque token encoded into page_token / next_page_token.
// It records the UpdatedAt timestamp (nanoseconds) and ID of the last entry on
// the current page so the next request can resume from that position.
type historyCursor struct {
	UpdatedAtNs int64  `json:"u"`
	ID          string `json:"i"`
}

// encodeHistoryCursor encodes a cursor to an opaque base64url string.
func encodeHistoryCursor(c historyCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeHistoryCursor decodes an opaque page_token string back to a cursor.
// Returns the zero value and false if the token is empty or malformed.
func decodeHistoryCursor(token string) (historyCursor, bool) {
	if token == "" {
		return historyCursor{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return historyCursor{}, false
	}
	var c historyCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return historyCursor{}, false
	}
	return c, true
}

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

	log.Info("history cache refreshed", "entries", hist.Count(), "duration", time.Since(now))
	return hist, nil
}

// ListClaudeHistory returns Claude session history entries with optional filtering
// and cursor-based pagination.
//
// Pagination rules:
//   - page_size controls how many entries are returned per page (default 100, max 500).
//   - page_token, when set, resumes from the position after the last entry on the
//     previous page.  Leave it empty for the first page.
//   - next_page_token in the response is non-empty when more pages exist; pass it
//     as page_token in the next request.
//   - The legacy limit field is honoured when page_size is zero.
//   - Filters (project, search_query) must be identical across all pages of a
//     paginated sequence.
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

	// --- Cursor pagination ---------------------------------------------------
	// Resolve effective page size: page_size takes precedence over limit.
	const defaultPageSize = 100
	const maxPageSize = 500

	pageSize := int(req.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = int(req.Msg.Limit) //nolint:staticcheck // legacy fallback: callers may still send limit
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	// Apply cursor: skip entries up to and including the cursor position.
	if cursor, ok := decodeHistoryCursor(req.Msg.PageToken); ok {
		startIdx := -1
		for i, e := range entries {
			if e.UpdatedAt.UnixNano() == cursor.UpdatedAtNs && e.ID == cursor.ID {
				startIdx = i + 1 // first entry of the next page
				break
			}
		}
		if startIdx > 0 && startIdx < len(entries) {
			entries = entries[startIdx:]
		} else if startIdx >= len(entries) {
			// Cursor points past the last entry — return empty last page.
			entries = nil
		}
		// If startIdx == -1 the cursor wasn't found (cache refreshed) — return
		// from the beginning so the caller can recover gracefully.
	}

	// Slice to page size and build next_page_token if there are more pages.
	var nextPageToken string
	if pageSize > 0 && len(entries) > pageSize {
		lastOnPage := entries[pageSize-1]
		nextPageToken = encodeHistoryCursor(historyCursor{
			UpdatedAtNs: lastOnPage.UpdatedAt.UnixNano(),
			ID:          lastOnPage.ID,
		})
		entries = entries[:pageSize]
	}
	// -------------------------------------------------------------------------

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
		Entries:       protoEntries,
		TotalCount:    int32(totalCount),
		NextPageToken: nextPageToken,
	}), nil
}

// GetClaudeHistoryDetail retrieves detailed information for a specific history entry,
// including lazily-fetched VCS status for the project directory.
//
// VCS status reuses the same vc.VCSProvider + vcsStatusToProto path that
// GetVCSStatus uses for running sessions, so the logic is not duplicated.
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

	protoEntry := &sessionv1.ClaudeHistoryEntry{
		Id:           entry.ID,
		Name:         entry.Name,
		Project:      entry.Project,
		CreatedAt:    timestamppb.New(entry.CreatedAt),
		UpdatedAt:    timestamppb.New(entry.UpdatedAt),
		Model:        entry.Model,
		MessageCount: int32(entry.MessageCount),
	}

	// Lazily enrich with VCS status. Reuse the existing vc.VCSProvider so the
	// git/jj logic is not duplicated. Errors are non-fatal — the UI should
	// handle a nil VcsStatus gracefully.
	if entry.Project != "" {
		if vcsStatus := fetchHistoryVCSStatus(entry.Project); vcsStatus != nil {
			protoEntry.VcsStatus = vcsStatus
		}
	}

	return connect.NewResponse(&sessionv1.GetClaudeHistoryDetailResponse{
		Entry: protoEntry,
	}), nil
}

// newHistoryVCSProvider returns a VCS provider for the given project path,
// preferring Git and falling back to Jujutsu.
func newHistoryVCSProvider(projectPath string) (vc.VCSProvider, error) {
	provider, err := vc.NewGitProvider(projectPath)
	if err == nil {
		return provider, nil
	}
	return vc.NewJujutsuProvider(projectPath)
}

// fetchHistoryVCSStatus fetches VCS status for an arbitrary project path.
// Returns nil when the directory is not a VCS repo or when the fetch fails.
func fetchHistoryVCSStatus(projectPath string) *sessionv1.VCSStatus {
	provider, err := newHistoryVCSProvider(projectPath)
	if err != nil {
		log.Debug("fetchHistoryVCSStatus: no VCS provider", "path", projectPath, "err", err)
		return nil
	}
	status, err := provider.GetStatus()
	if err != nil {
		log.Debug("fetchHistoryVCSStatus: GetStatus failed", "path", projectPath, "err", err)
		return nil
	}
	return vcsStatusToProto(status)
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

	// Use the reverse tail reader only when explicitly requested via tail=true.
	// Standard limit/offset reads always scan from the start so offset semantics
	// remain correct and total_count reflects the full conversation length.
	fileLimit := 0
	if req.Msg.Tail && req.Msg.Limit > 0 && req.Msg.Offset == 0 {
		fileLimit = int(req.Msg.Limit)
	}
	messages, err := hist.GetMessagesFromConversationFile(req.Msg.Id, fileLimit)
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
		log.Info("search index sync", "result", syncResult.String())
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
