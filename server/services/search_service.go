package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/telemetry"
	"golang.org/x/sync/singleflight"

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

// historyBranchEntry is a TTL-cached git branch name for a project path.
type historyBranchEntry struct {
	branch    string
	expiresAt time.Time
}

// historySnapshot is an immutable COW record stored in atomic.Value.
type historySnapshot struct {
	cache *session.ClaudeSessionHistory
	at    time.Time
}

// SearchService handles all Claude history and full-text search RPC methods.
//
// It owns the history cache and search engine state that were previously
// scattered across SessionService.
//
// Concurrency model: atomic.Value (COW) + singleflight for the history cache;
// sync.Map for the per-path branch cache. No mutexes held across I/O.
type SearchService struct {
	searchEngine     *search.SearchEngine
	snippetGenerator *search.SnippetGenerator

	historyCacheTTL time.Duration
	historySnap     atomic.Value       // stores *historySnapshot; nil before first load
	historyGroup    singleflight.Group //nolint:exhaustruct

	// getInstances is wired after construction so ListClaudeHistory can
	// cross-reference live sessions for session_status enrichment.
	getInstances func() []*session.Instance

	// resolveConversationUUID converts a tmux session UUID to a Claude conversation UUID.
	// Used by GetClaudeHistoryMessages to look up history for backlog sessions.
	resolveConversationUUID func(ctx context.Context, tmuxUUID string) (string, error)

	branchCache    sync.Map // map[string]*historyBranchEntry  keyed by projectPath
	branchCacheTTL time.Duration
}

// SetResolveConversationUUID wires the tmux-UUID → Claude-UUID resolver.
func (ss *SearchService) SetResolveConversationUUID(fn func(ctx context.Context, tmuxUUID string) (string, error)) {
	ss.resolveConversationUUID = fn
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
		branchCacheTTL:   60 * time.Second,
	}
}

// SetInstanceProvider wires the live-instance provider after SessionService
// is fully constructed. Must be called before the first ListClaudeHistory.
func (ss *SearchService) SetInstanceProvider(fn func() []*session.Instance) {
	ss.getInstances = fn
}

// cachedBranch returns the current git branch for projectPath, caching the
// result for branchCacheTTL (60 s). Returns "" on error or detached HEAD.
// Uses sync.Map for lock-free concurrent reads; git is invoked outside any lock.
func (ss *SearchService) cachedBranch(ctx context.Context, projectPath string) string {
	if projectPath == "" {
		return ""
	}
	now := time.Now()

	if v, ok := ss.branchCache.Load(projectPath); ok {
		if e := v.(*historyBranchEntry); now.Before(e.expiresAt) {
			return e.branch
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, "git", "-C", projectPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	branch := ""
	if err == nil {
		branch = strings.TrimSpace(string(out))
		if branch == "HEAD" {
			branch = "" // detached HEAD — not useful to display
		}
	}

	ss.branchCache.Store(projectPath, &historyBranchEntry{branch: branch, expiresAt: now.Add(ss.branchCacheTTL)})
	return branch
}

// liveSessionStatus returns the proto SessionStatus for the history entry
// with the given conversationID by scanning live instances. Returns UNSPECIFIED
// when no live session matches.
func (ss *SearchService) liveSessionStatus(conversationID string) sessionv1.SessionStatus {
	if conversationID == "" || ss.getInstances == nil {
		return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
	for _, inst := range ss.getInstances() {
		if inst.GetConversationUUID() == conversationID {
			return adapters.StatusToProto(inst.Status)
		}
	}
	return sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
}

// getOrRefreshHistoryCache returns the cached history or refreshes it if stale.
// Fast-path reads are lock-free (atomic.Value Load). Concurrent refreshes are
// coalesced via singleflight so at most one disk scan runs at a time.
func (ss *SearchService) getOrRefreshHistoryCache(ctx context.Context) (*session.ClaudeSessionHistory, error) {
	ctx, span := telemetry.StartSpan(ctx, "SearchService.getOrRefreshHistoryCache")
	defer span.End()

	now := time.Now()

	// Fast path: atomic load — no lock.
	if v := ss.historySnap.Load(); v != nil {
		snap := v.(*historySnapshot)
		if now.Sub(snap.at) < ss.historyCacheTTL {
			span.SetAttributes(
				attribute.Bool("cache.hit", true),
				attribute.Int("history.entry_count", snap.cache.Count()),
			)
			return snap.cache, nil
		}
	}

	// Slow path: coalesce concurrent refreshes with singleflight.
	span.SetAttributes(attribute.Bool("cache.hit", false))
	type result struct {
		hist *session.ClaudeSessionHistory
	}
	v, err, _ := ss.historyGroup.Do("refresh", func() (interface{}, error) {
		// Re-check after winning the coalesce race — another goroutine may have
		// stored a fresh snapshot while we were waiting.
		if sv := ss.historySnap.Load(); sv != nil {
			snap := sv.(*historySnapshot)
			if now.Sub(snap.at) < ss.historyCacheTTL {
				return result{hist: snap.cache}, nil
			}
		}

		_, loadSpan := telemetry.StartSpan(ctx, "SearchService.loadHistoryFromDisk")
		loadStart := time.Now()
		hist, loadErr := session.NewClaudeSessionHistoryFromClaudeDir()
		loadDuration := time.Since(loadStart)
		loadSpan.SetAttributes(attribute.Int64("load.duration_ms", loadDuration.Milliseconds()))
		if loadErr != nil {
			loadSpan.RecordError(loadErr)
			loadSpan.End()
			return nil, fmt.Errorf("failed to create history manager: %w", loadErr)
		}
		loadSpan.SetAttributes(attribute.Int("history.entry_count", hist.Count()))
		loadSpan.End()

		ss.historySnap.Store(&historySnapshot{cache: hist, at: now})
		log.Info("history cache refreshed", "entries", hist.Count(), "duration", time.Since(now))
		return result{hist: hist}, nil
	})
	if err != nil {
		return nil, err
	}
	hist := v.(result).hist
	span.SetAttributes(
		attribute.Int("history.entry_count", hist.Count()),
		attribute.Int64("cache.refresh_duration_ms", time.Since(now).Milliseconds()),
	)
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

	if req.Msg.GetExcludeAutomationSessions() && ss.getInstances != nil {
		entries = filterHistoryEntriesByAutomation(entries, ss.getInstances())
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
			Id:            entry.ID,
			Name:          entry.Name,
			Project:       entry.Project,
			CreatedAt:     timestamppb.New(entry.CreatedAt),
			UpdatedAt:     timestamppb.New(entry.UpdatedAt),
			Model:         entry.Model,
			MessageCount:  int32(entry.MessageCount),
			Branch:        ss.cachedBranch(ctx, entry.Project),
			SessionStatus: ss.liveSessionStatus(entry.ID),
		})
	}

	return connect.NewResponse(&sessionv1.ListClaudeHistoryResponse{
		Entries:       protoEntries,
		TotalCount:    int32(totalCount),
		NextPageToken: nextPageToken,
	}), nil
}

// resolveHistoryEntry looks up a history entry by id, retrying against the
// tmux-session-UUID -> Claude-conversation-UUID resolver when the direct
// lookup fails. Callers (e.g. the backlog UI) sometimes pass a tmux session
// UUID rather than the Claude conversation UUID that history.jsonl entries
// are actually keyed by; resolveConversationUUID bridges that gap.
//
// Returns the entry and the ID it was ultimately found under (equal to id
// unless the fallback fired), so callers needing the resolved ID for a
// follow-up lookup (e.g. GetMessagesFromConversationFile) can reuse it.
func (ss *SearchService) resolveHistoryEntry(
	ctx context.Context,
	hist *session.ClaudeSessionHistory,
	id string,
) (*session.ClaudeHistoryEntry, string, error) {
	entry, err := hist.GetByID(id)
	if err == nil {
		return entry, id, nil
	}
	if ss.resolveConversationUUID == nil {
		return nil, "", err
	}
	resolved, resolveErr := ss.resolveConversationUUID(ctx, id)
	if resolveErr != nil || resolved == "" || resolved == id {
		return nil, "", err
	}
	resolvedEntry, resolvedErr := hist.GetByID(resolved)
	if resolvedErr != nil {
		return nil, "", err
	}
	return resolvedEntry, resolved, nil
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

	entry, _, err := ss.resolveHistoryEntry(ctx, hist, req.Msg.Id)
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
	if req.Msg.AnchorIndex != nil && req.Msg.Tail {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("anchor_index and tail are mutually exclusive"))
	}

	hist, err := ss.getOrRefreshHistoryCache(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load history: %w", err))
	}

	_, resolvedID, err := ss.resolveHistoryEntry(ctx, hist, req.Msg.Id)
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
	messages, err := hist.GetMessagesFromConversationFile(resolvedID, fileLimit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load messages: %w", err))
	}

	totalCount := len(messages)
	offset := int(req.Msg.Offset)
	limit := int(req.Msg.Limit)
	if req.Msg.AnchorIndex != nil {
		anchor := int(*req.Msg.AnchorIndex)
		offset = anchor - limit/2
		if offset < 0 {
			offset = 0
		}
	}
	if offset >= len(messages) {
		messages = messages[:0]
	} else if offset > 0 {
		messages = messages[offset:]
	}
	if limit > 0 && limit < len(messages) {
		messages = messages[:limit]
	}

	protoMessages := toProtoClaudeMessages(messages)

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

	// offset is applied directly to the raw engine fetch, so it addresses a
	// position in the raw (pre-dedup/filter) result stream, not the
	// post-processed one — paginating (offset>0) together with any
	// post-processing flag can skip or duplicate sessions across pages.
	// Rebasing offset against the deduped/filtered set is real work with no
	// caller needing it today (RelatedWorkQuery, the only caller of these
	// flags, never sets offset) — reject the combination explicitly instead
	// of silently returning a wrong page.
	needsPostProcessing := req.Msg.GetGroupBySession() || req.Msg.GetExcludeAutomationSessions() || req.Msg.GetProject() != ""
	if req.Msg.Offset > 0 && needsPostProcessing {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("offset is not supported together with group_by_session, exclude_automation_sessions, or project"))
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

	requestedLimit := int(req.Msg.Limit)
	if requestedLimit <= 0 {
		requestedLimit = 20
	}
	if requestedLimit > 100 {
		requestedLimit = 100
	}

	offset := int(req.Msg.Offset)
	if offset < 0 {
		offset = 0
	}

	// needsPostProcessing is true whenever a post-fetch filter/dedup step will
	// run on protoResults below. In that case, truncating the *raw* engine
	// fetch to requestedLimit first would let a single busy or filtered-out
	// session consume the entire raw window before dedup/filtering ever gets
	// a chance to run — so over-fetch a larger raw candidate set instead, and
	// apply requestedLimit only after post-processing (see the truncation
	// step near the end of this function). This raises but does not remove
	// the starvation threshold: if more than rawLimit/requestedLimit sessions
	// outscore a relevant one, it can still be crowded out of the raw window.
	// (offset>0 combined with this is rejected above, before the history
	// load, so needsPostProcessing here only ever gates the oversampling and
	// post-processing steps, never the offset math.)
	rawLimit := requestedLimit
	if needsPostProcessing {
		rawLimit = requestedLimit * 5
		if rawLimit > 100 {
			rawLimit = 100
		}
	}

	searchOpts := search.SearchOptions{
		Limit:  rawLimit,
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

	protoResults = ss.applyResultPostProcessing(protoResults, req.Msg, hist, requestedLimit, needsPostProcessing)

	return connect.NewResponse(&sessionv1.SearchClaudeHistoryResponse{
		Results:      protoResults,
		TotalMatches: int32(searchResults.TotalMatches),
		QueryTimeMs:  searchResults.QueryTime.Milliseconds(),
		HasMore:      searchResults.TotalMatches > offset+len(protoResults),
	}), nil
}
