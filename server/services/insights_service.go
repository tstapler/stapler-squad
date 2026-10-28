package services

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/contexthistory"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// findingsCap bounds how many WasteFindings GetInsightsSummary returns in one
// response — the panel is a "find the driver fast" view, not an exhaustive log.
const findingsCap = 20

// Compile-time check: InsightsService must implement the generated handler.
var _ sessionv1connect.InsightsServiceHandler = (*InsightsService)(nil)

// insightsBacklogReader is the narrow interface InsightsService uses to source
// persisted session_role at summary-build time (ADR-029). Satisfied by *session.Storage.
type insightsBacklogReader interface {
	GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]session.ItemSessionBacklogEntry, error)
}

// insightsDeletedCostLedgerReader is the narrow interface InsightsService uses
// to source the durable deleted-item cost ledger (session/ent/schema/deleted_item_session_cost.go).
// A separate seam from insightsBacklogReader so a backlogReader test double
// that doesn't implement it is just treated as having no ledger (nil-safe).
type insightsDeletedCostLedgerReader interface {
	GetDeletedItemSessionCostLedger(ctx context.Context) ([]session.DeletedItemSessionCostEntry, error)
}

// DismissedFindingsRepository is the seam for finding-dismissal persistence.
// Satisfied implicitly by *session.Storage. nil (the zero value of
// InsightsService.dismissedFindings) means the feature is unavailable — the
// same "nil means unavailable" idiom as RulesService's configStore/aiClient —
// so tests that construct InsightsService without wiring it still work.
type DismissedFindingsRepository interface {
	DismissFinding(ctx context.Context, data session.DismissedFindingData) error
	ListDismissedFindingIDs(ctx context.Context) (map[string]bool, error)
}

// InsightsService implements the ConnectRPC InsightsServiceHandler.
// It reads from a TokenStoreReader to serve token usage analytics.
type InsightsService struct {
	store         tokens.TokenStoreReader
	pricing       *tokens.PricingTable
	associator    *tokens.Associator
	backlogReader insightsBacklogReader

	// dismissedFindings persists DismissFinding calls and is consulted by
	// GetInsightsSummary to filter dismissed findings out of the response.
	// nil means dismissal is unavailable (see doc comment on the interface).
	dismissedFindings DismissedFindingsRepository

	// contextHistory serves the context-history RPCs. nil means unavailable.
	contextHistory *contexthistory.Store

	// logMu guards loggedUnpricedFamilies.
	logMu sync.Mutex
	// loggedUnpricedFamilies tracks which unpriced model families have already
	// had a warning logged, so a newly-observed family is only logged once
	// (not on every request). Grows unbounded over the process lifetime, but
	// bounded in practice by the number of distinct model families ever seen —
	// safe to leave unbounded (see pre-mortem's minor note).
	loggedUnpricedFamilies map[string]bool
	// backfillOnce runs BackfillConversationUUIDs once per process, on the first
	// summary request. If the token store is still empty then, nothing is stamped
	// and the next process start retries.
	backfillOnce sync.Once

	// summaries caches GetInsightsSummary responses (see summaryCache).
	summaries summaryCache
}

// NewInsightsService creates a new InsightsService.
func NewInsightsService(
	store tokens.TokenStoreReader,
	pricing *tokens.PricingTable,
	associator *tokens.Associator,
	backlogReader insightsBacklogReader,
) *InsightsService {
	return &InsightsService{
		store:                  store,
		pricing:                pricing,
		associator:             associator,
		backlogReader:          backlogReader,
		loggedUnpricedFamilies: make(map[string]bool),
	}
}

// untrackedItemIDPrefix marks a role_breakdown item entry synthesized by
// sessionDisplayTitle rather than sourced from a real backlog item — see
// groupUnattributed's doc comment.
const untrackedItemIDPrefix = "untracked:"

// worktreeTitlePattern extracts a stapler-squad worktree's session title from a
// Claude transcript's decoded project path (".../worktrees/<title>/<16-hex id>").
// Mirrors web-app/src/app/insights/insightsFormatters.ts's WORKTREE_PATH regex —
// keep the two in sync.
var worktreeTitlePattern = regexp.MustCompile(`/worktrees/(.+?)/[0-9a-f]{16}(?:/|$)`)

// sessionDisplayTitle returns a human-readable label for projectPath: the
// worktree's session title if the path is a stapler-squad worktree, else the
// path's final segment. "" in, "" out.
func sessionDisplayTitle(projectPath string) string {
	if m := worktreeTitlePattern.FindStringSubmatch(projectPath); m != nil {
		return strings.ReplaceAll(m[1], "/", "-")
	}
	return path.Base(strings.TrimRight(projectPath, "/"))
}

// groupUnattributed returns meta unchanged unless it carries no backlog
// attribution (Role == ""), in which case it groups it under a synthetic
// per-title item keyed by sessionDisplayTitle(projectPath) — untrackedItemIDPrefix
// keeps that ID unambiguous from a real backlog-item UUID (BacklogItemDetail
// relies on this). Role is further set to "external" when projectPath isn't a
// stapler-squad worktree path; a raw `claude` run inside the main checkout
// itself is indistinguishable from this and accepted as unsolved.
func groupUnattributed(meta SessionMeta, projectPath string) SessionMeta {
	if meta.Role != "" {
		return meta
	}
	title := sessionDisplayTitle(projectPath)
	if title == "" {
		return meta
	}
	if !worktreeTitlePattern.MatchString(projectPath) {
		meta.Role = session.SessionRoleExternal
	}
	meta.ItemID = untrackedItemIDPrefix + title
	meta.ItemTitle = title
	return meta
}

// SessionMeta is the per-session backlog attribution sessionMetaForSessions
// looks up by tmux session ID: its SessionRole plus the backlog item it
// belongs to (Epic 4.1's item-drilldown data).
type SessionMeta struct {
	Role      string
	ItemID    string
	ItemTitle string
	// ItemSessionUUID is the ItemSession's session_uuid, so a conversation-matched
	// transcript can mark that row covered for the fold-in dedupe.
	ItemSessionUUID string
}

// metaFor resolves r's attribution: by live session ID first, else by Claude
// conversation UUID (survives the session row's deletion). ok reports a match.
func metaFor(meta map[string]SessionMeta, sessionID string, r *tokens.ParseResult) (SessionMeta, bool) {
	if m, ok := meta[sessionID]; ok && sessionID != "" {
		return m, true
	}
	if r != nil && r.SessionUUID != "" {
		m, ok := meta[conversationKey(r.SessionUUID)]
		return m, ok
	}
	return SessionMeta{}, false
}

// conversationKey namespaces conversation UUIDs inside the shared SessionMeta map.
func conversationKey(conversationUUID string) string { return "conv:" + conversationUUID }

// sessionMetaResult is sessionMetaForSessions' return value: the built
// sessionID→SessionMeta map, plus the raw rows it was built from. A caller
// that also needs to fold the same backlog entries/ledger rows into a
// separate aggregate (GetInsightsSummary's transcript-less fold-in passes)
// reuses entries/ledgerEntries instead of re-querying
// GetAllItemSessionsWithBacklogInfo/GetDeletedItemSessionCostLedger a second
// time per request.
type sessionMetaResult struct {
	meta          map[string]SessionMeta
	entries       []session.ItemSessionBacklogEntry
	ledgerEntries []session.DeletedItemSessionCostEntry
}

// sessionMetaForSessions builds a sessionUUID→SessionMeta map from one unfiltered
// GetAllItemSessionsWithBacklogInfo scan, keeping the first (most-recently-created,
// per that query's explicit Order(Desc(CreatedAt))) entry seen per UUID (ADR-029).
func (s *InsightsService) sessionMetaForSessions(ctx context.Context) sessionMetaResult {
	if s.backlogReader == nil {
		return sessionMetaResult{}
	}
	entries, err := s.backlogReader.GetAllItemSessionsWithBacklogInfo(ctx)
	if err != nil {
		log.Warn("failed to fetch session roles for insights", "err", err)
		return sessionMetaResult{}
	}
	meta := make(map[string]SessionMeta, len(entries))
	for _, e := range entries {
		m := SessionMeta{Role: e.SessionRole, ItemID: e.ItemID, ItemTitle: e.ItemTitle, ItemSessionUUID: e.SessionUUID}
		if _, exists := meta[e.SessionUUID]; !exists {
			meta[e.SessionUUID] = m
		}
		if e.ConversationUUID != "" {
			if _, exists := meta[conversationKey(e.ConversationUUID)]; !exists {
				meta[conversationKey(e.ConversationUUID)] = m
			}
		}
	}

	// Story 3: fold in the deleted-item cost ledger, keyed by conversation UUID
	// only — a ledger row's snapshotted session_uuid belongs to a session row
	// that was already gone before the item was even deleted, so it can never
	// resolve a live association; only metaFor's conversation-UUID fallback can
	// ever reach these rows.
	var ledgerEntries []session.DeletedItemSessionCostEntry
	if ledgerReader, ok := s.backlogReader.(insightsDeletedCostLedgerReader); ok {
		le, err := ledgerReader.GetDeletedItemSessionCostLedger(ctx)
		if err != nil {
			log.Warn("failed to fetch deleted item session cost ledger for insights", "err", err)
		} else {
			ledgerEntries = le
			for _, e := range ledgerEntries {
				if e.ConversationUUID == "" {
					continue
				}
				key := conversationKey(e.ConversationUUID)
				if _, exists := meta[key]; !exists {
					meta[key] = SessionMeta{Role: e.SessionRole, ItemID: e.ItemID, ItemTitle: e.ItemTitle}
				}
			}
		}
	}
	return sessionMetaResult{meta: meta, entries: entries, ledgerEntries: ledgerEntries}
}

// SetDismissedFindingsStore wires dismissal persistence (nil disables it —
// see DismissedFindingsRepository's doc comment).
func (s *InsightsService) SetDismissedFindingsStore(store DismissedFindingsRepository) {
	s.dismissedFindings = store
}

// SetContextHistoryStore wires the persisted context-history store (nil disables
// the context-history RPCs).
func (s *InsightsService) SetContextHistoryStore(store *contexthistory.Store) {
	s.contextHistory = store
}

func (s *InsightsService) requireContextHistory() error {
	if s.contextHistory == nil {
		return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("context history is not available"))
	}
	return nil
}

func toCeilingSummary(sum contexthistory.SessionSummary) *sessionv1.ContextCeilingSummary {
	return &sessionv1.ContextCeilingSummary{
		ConversationId:       sum.SessionUUID,
		Turns:                int32(sum.Turns),
		PeakTokens:           sum.PeakTokens,
		FractionAtOrAbove_75: sum.FractionWarn,
		FractionAtOrAbove_90: sum.FractionCritical,
	}
}

// GetContextHistory returns a session's persisted context series, ceiling summary and compactions.
// +api: GetContextHistory
func (s *InsightsService) GetContextHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetContextHistoryRequest],
) (*connect.Response[sessionv1.GetContextHistoryResponse], error) {
	if err := s.requireContextHistory(); err != nil {
		return nil, err
	}
	id := req.Msg.ConversationId
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conversation_id is required"))
	}
	series, err := s.contextHistory.Series(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	compactions, err := s.contextHistory.Compactions(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &sessionv1.GetContextHistoryResponse{
		Samples: make([]*sessionv1.ContextSample, 0, len(series)),
		Summary: toCeilingSummary(contexthistory.Summarize(id, series)),
	}
	for _, sm := range series {
		resp.Samples = append(resp.Samples, &sessionv1.ContextSample{
			TurnIndex:     int32(sm.TurnIndex),
			SampledAt:     timestamppb.New(sm.SampledAt),
			Model:         sm.Model,
			ContextTokens: sm.ContextTokens,
			ContextMax:    sm.ContextMax,
		})
	}
	for _, c := range compactions {
		resp.Compactions = append(resp.Compactions, &sessionv1.CompactionEvent{
			ConversationId: c.SessionUUID,
			TurnIndex:      int32(c.TurnIndex),
			OccurredAt:     timestamppb.New(c.OccurredAt),
			Trigger:        c.Trigger,
			TokensBefore:   c.TokensBefore,
			TokensAfter:    c.TokensAfter,
			TokensFreed:    c.TokensFreed,
		})
	}
	return connect.NewResponse(resp), nil
}

// ListSessionsByCeilingTime ranks sessions by time spent near the context ceiling.
// +api: ListSessionsByCeilingTime
func (s *InsightsService) ListSessionsByCeilingTime(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionsByCeilingTimeRequest],
) (*connect.Response[sessionv1.ListSessionsByCeilingTimeResponse], error) {
	if err := s.requireContextHistory(); err != nil {
		return nil, err
	}
	ranked, err := s.contextHistory.RankByCeilingTime(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &sessionv1.ListSessionsByCeilingTimeResponse{}
	for _, sum := range ranked {
		resp.Sessions = append(resp.Sessions, toCeilingSummary(sum))
	}
	return connect.NewResponse(resp), nil
}

// GetCompactionStats aggregates compaction telemetry across sessions.
// +api: GetCompactionStats
func (s *InsightsService) GetCompactionStats(
	ctx context.Context,
	req *connect.Request[sessionv1.GetCompactionStatsRequest],
) (*connect.Response[sessionv1.GetCompactionStatsResponse], error) {
	if err := s.requireContextHistory(); err != nil {
		return nil, err
	}
	var since time.Time
	if req.Msg.Since != nil {
		since = req.Msg.Since.AsTime()
	}
	st, err := s.contextHistory.CompactionStats(ctx, since)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.GetCompactionStatsResponse{
		Count:                  int32(st.Count),
		ManualCount:            int32(st.ManualCount),
		AutoCount:              int32(st.AutoCount),
		TotalTokensFreed:       st.TotalFreed,
		AverageTokensFreed:     st.AverageFreed,
		SessionsWithCompaction: int32(st.SessionsWithAny),
	}), nil
}

// warnNewUnpricedFamilies logs a warning for each family in families that has
// not previously been logged, then marks it logged. Deduped across calls so a
// given unpriced family produces exactly one log line for the life of the
// process, not one per request.
func (s *InsightsService) warnNewUnpricedFamilies(families map[string]bool) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	for family := range families {
		if !s.loggedUnpricedFamilies[family] {
			s.loggedUnpricedFamilies[family] = true
			log.Warn("insights: unpriced model family observed", "family", family)
		}
	}
}

// buildSessionSummary builds the per-session SessionTokenSummary shared by
// GetInsightsSummary, ListSessionTokens, and watchInsights (Story 1.5.2) —
// sessionID/orphan resolution, cost, cache-hit-rate, top tools (with Epic
// 1.2's per-tool cost attribution), skill names, timestamps, activity type
// (Epic 1.2's ClassifyActivity), and waste score (Epic 1.1's
// ComputeWasteScore). Deliberately excludes WasteFinding computation — those
// are a separate, non-summable response-level list (see
// ADR-002-findings-non-summable-dollar-impact.md), not a SessionTokenSummary
// field.
//
// Also returns the grouped SessionMeta (post-groupUnattributed) it computed
// for this session, so a caller that also feeds accumulateRoleCost — the
// proto's SessionRole field alone doesn't carry ItemID/ItemTitle — reuses
// this value instead of recomputing metaFor+groupUnattributed a second time
// and risking the two falling out of sync (see 0107988f9).
func buildSessionSummary(
	r *tokens.ParseResult,
	pt *tokens.PricingTable,
	associator *tokens.Associator,
	snapshot []tokens.SessionRecord,
	sessionMeta map[string]SessionMeta,
) (*sessionv1.SessionTokenSummary, SessionMeta) {
	rec, isOrphan := tokens.SessionRecord{}, true
	if associator != nil {
		rec, isOrphan = associator.AssociateRecordWithSnapshot(r, snapshot)
	}
	return buildSessionSummaryForRecord(r, pt, rec, isOrphan, sessionMeta)
}

// buildSessionSummaryForRecord is buildSessionSummary with the association
// already resolved, so a caller that has associated r (GetInsightsSummary's
// filter pass) does not scan the session snapshot a second time.
func buildSessionSummaryForRecord(
	r *tokens.ParseResult,
	pt *tokens.PricingTable,
	rec tokens.SessionRecord,
	isOrphan bool,
	sessionMeta map[string]SessionMeta,
) (*sessionv1.SessionTokenSummary, SessionMeta) {
	firstTs, lastTs := sessionTimestamps(r)

	sessionID, tags := rec.SessionID, rec.Tags
	attributed, byConversation := metaFor(sessionMeta, sessionID, r)
	if byConversation {
		isOrphan = false
	}
	// Reclassify "" -> "external" the same way accumulateRoleCost's caller does
	// (groupUnattributed), so SessionRole matches the role_breakdown bucket this
	// session is actually counted under — otherwise a click on the "external" bar
	// would filter SessionsTable by a role value no session ever reports.
	attributed = groupUnattributed(attributed, r.ProjectPath)

	categoryCosts, unpriced := pt.EstimateCostByCategory(r)
	costUSD := categoryCosts.Total()
	cacheHitRate := tokens.ComputeCacheHitRate(r.TotalInput, r.CacheRead)
	activityType := tokens.ClassifyActivity(r)
	topTools := sessionTopTools(r, pt)

	skillNames := make([]string, 0, len(r.SkillActivations))
	for _, sa := range r.SkillActivations {
		skillNames = append(skillNames, sa.Name)
	}

	summary := &sessionv1.SessionTokenSummary{
		SessionId:            sessionID,
		ConversationId:       r.SessionUUID,
		ProjectPath:          r.ProjectPath,
		PrimaryModel:         r.PrimaryModel,
		TotalInputTokens:     r.TotalInput,
		TotalOutputTokens:    r.TotalOutput,
		CacheCreationTokens:  r.CacheCreation,
		CacheReadTokens:      r.CacheRead,
		EstimatedCostUsd:     costUSD,
		InputCostUsd:         categoryCosts.Input,
		OutputCostUsd:        categoryCosts.Output,
		CacheCreationCostUsd: categoryCosts.CacheCreation,
		CacheReadCostUsd:     categoryCosts.CacheRead,
		CacheHitRate:         cacheHitRate,
		// #nosec G115 -- r.MessageCount is a per-session Claude message count, far below int32 range.
		MessageCount:     int32(r.MessageCount),
		IsOrphan:         isOrphan,
		SkillActivations: skillNames,
		TopTools:         topTools,
		UnpricedModels:   unpriced,
		ActivityType:     activityType,
		Tags:             tags,
		SessionRole:      attributed.Role,
	}
	if !firstTs.IsZero() {
		summary.FirstMessageAt = timestamppb.New(firstTs)
	}
	if !lastTs.IsZero() {
		summary.LastMessageAt = timestamppb.New(lastTs)
	}
	if score := tokens.ComputeWasteScore(r, pt); score != nil {
		summary.WasteScore = proto.Float64(float64(*score))
	}
	if roi, ok := tokens.ComputeCacheROI(r, pt); ok {
		summary.CacheRoiUsd = roi
	}
	return summary, attributed
}

// GetInsightsSummary returns aggregated token and cost data for a time range.
// Results are served from a short-TTL cache with singleflight coalescing; see
// summaryCache.
func (s *InsightsService) GetInsightsSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetInsightsSummaryRequest],
) (*connect.Response[sessionv1.GetInsightsSummaryResponse], error) {
	resp, err := s.summaries.get(ctx, req.Msg, s.store.IsLoading, func(ctx context.Context) (*sessionv1.GetInsightsSummaryResponse, error) {
		return s.computeInsightsSummary(ctx, req.Msg)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// ListSessionTokens returns per-session token summaries with pagination.
func (s *InsightsService) ListSessionTokens(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionTokensRequest],
) (*connect.Response[sessionv1.ListSessionTokensResponse], error) {
	results := s.store.GetAll()
	msg := req.Msg

	var fromTime, toTime time.Time
	if msg.From != nil {
		fromTime = msg.From.AsTime()
	}
	if msg.To != nil {
		toTime = msg.To.AsTime()
	}

	// Build session summaries.
	summaries := make([]*sessionv1.SessionTokenSummary, 0, len(results))
	allUnpricedFamilies := make(map[string]bool) // union of unpriced families across all sessions in this call
	var sessionSnapshot []tokens.SessionRecord
	if s.associator != nil {
		sessionSnapshot = s.associator.Snapshot()
	}
	sessionMeta := s.sessionMetaForSessions(ctx).meta
	for _, r := range results {
		if r == nil {
			continue
		}
		firstTs, lastTs := sessionTimestamps(r)
		if !fromTime.IsZero() && !lastTs.IsZero() && lastTs.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && !firstTs.IsZero() && firstTs.After(toTime) {
			continue
		}

		summary, _ := buildSessionSummary(r, s.pricing, s.associator, sessionSnapshot, sessionMeta)
		for _, f := range summary.UnpricedModels {
			allUnpricedFamilies[f] = true
		}
		summaries = append(summaries, summary)
	}

	// Emit a deduped runtime warning for any newly-observed unpriced family
	// (AC-5 runtime-signal half) — logs each family at most once for the life
	// of the process, not once per request.
	s.warnNewUnpricedFamilies(allUnpricedFamilies)

	// Sort.
	sortBy := msg.SortBy
	if sortBy == "" {
		sortBy = "date"
	}
	sortDesc := msg.SortDesc
	sort.Slice(summaries, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "cost":
			less = summaries[i].EstimatedCostUsd < summaries[j].EstimatedCostUsd
		case "tokens":
			ti := summaries[i].TotalInputTokens + summaries[i].TotalOutputTokens
			tj := summaries[j].TotalInputTokens + summaries[j].TotalOutputTokens
			less = ti < tj
		default: // "date"
			if summaries[i].LastMessageAt == nil {
				return !sortDesc
			}
			if summaries[j].LastMessageAt == nil {
				return sortDesc
			}
			less = summaries[i].LastMessageAt.AsTime().Before(summaries[j].LastMessageAt.AsTime())
		}
		if sortDesc {
			return !less
		}
		return less
	})

	// Pagination.
	// #nosec G115 -- len(summaries) is the in-memory session summary count, far below int32 range.
	totalCount := int32(len(summaries))
	pageSize := int(msg.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}

	startIdx := 0
	if msg.PageToken != "" {
		// PageToken is the index as a decimal string.
		_, err := fmt.Sscanf(msg.PageToken, "%d", &startIdx)
		if err != nil || startIdx < 0 || startIdx >= len(summaries) {
			startIdx = 0
		}
	}

	endIdx := startIdx + pageSize
	if endIdx > len(summaries) {
		endIdx = len(summaries)
	}

	page := summaries[startIdx:endIdx]
	nextPageToken := ""
	if endIdx < len(summaries) {
		nextPageToken = fmt.Sprintf("%d", endIdx)
	}

	return connect.NewResponse(&sessionv1.ListSessionTokensResponse{
		Sessions:      page,
		NextPageToken: nextPageToken,
		TotalCount:    totalCount,
	}), nil
}

// insightsEventSender is the narrow interface watchInsights sends through.
// Satisfied structurally (no explicit "implements") by
// *connect.ServerStream[sessionv1.InsightsEvent] in production, and by a
// mutex-guarded fake in tests (server/services/insights_service_test.go).
// Mirrors backlogItemEventSender (backlog_service_events.go) — same problem
// (*connect.ServerStream[T] has no exported constructor, so no real fake can
// be built outside the connect package), same fix.
type insightsEventSender interface {
	Send(*sessionv1.InsightsEvent) error
}

// WatchInsights streams summary updates when new JSONL data is parsed.
// Sends an initial "parse_complete" event (or "loading" if still parsing),
// then pushes an "update" event each time the TokenStore processes a new file.
// +api: WatchInsights
func (s *InsightsService) WatchInsights(
	ctx context.Context,
	_ *connect.Request[sessionv1.WatchInsightsRequest],
	stream *connect.ServerStream[sessionv1.InsightsEvent],
) error {
	return s.watchInsights(ctx, stream)
}

// watchInsights is WatchInsights's core logic, extracted behind the
// insightsEventSender interface (see its doc comment) so it is directly
// unit-testable without a real RPC round-trip.
func (s *InsightsService) watchInsights(ctx context.Context, sender insightsEventSender) error {
	done := TrackOpenStream("WatchInsights")
	defer done()

	// 1. Send initial state.
	allParsed := !s.store.IsLoading()
	initialEvent := &sessionv1.InsightsEvent{
		EventType: "parse_complete",
		AllParsed: allParsed,
	}
	if !allParsed {
		initialEvent.EventType = "loading"
	}
	if err := sender.Send(initialEvent); err != nil {
		return fmt.Errorf("send initial event: %w", err)
	}

	// 2. Subscribe to TokenStore changes.
	ch := s.store.Subscribe()
	defer s.store.Unsubscribe(ch)

	// 3. Forward events until context is cancelled.
	for {
		select {
		case <-ctx.Done():
			return nil
		case result, ok := <-ch:
			if !ok {
				return nil
			}
			// The client refetches the summary on this event; drop the cache first
			// so that refetch is not served pre-change data for up to the TTL.
			s.summaries.invalidate()
			var evt *sessionv1.InsightsEvent
			if result != nil {
				var snapshot []tokens.SessionRecord
				if s.associator != nil {
					snapshot = s.associator.Snapshot()
				}
				sessionMeta := s.sessionMetaForSessions(ctx).meta
				summary, _ := buildSessionSummary(result, s.pricing, s.associator, snapshot, sessionMeta)
				evt = &sessionv1.InsightsEvent{
					EventType: "update",
					Session:   summary,
					AllParsed: !s.store.IsLoading(),
				}
			} else {
				// nil means the initial directory walk just completed — a real
				// "parse_complete", not another indistinguishable "update", so
				// the frontend's fetchSummary()-triggering branch (which only
				// listens for "parse_complete") re-fires correctly.
				evt = &sessionv1.InsightsEvent{
					EventType: "parse_complete",
					AllParsed: true,
				}
			}
			if err := sender.Send(evt); err != nil {
				return fmt.Errorf("send update event: %w", err)
			}
		}
	}
}

// GetSessionTurnTimeline returns per-turn token stats for one session, fetched
// on-demand when the session detail drawer opens.
// +api: GetSessionTurnTimeline
func (s *InsightsService) GetSessionTurnTimeline(
	_ context.Context,
	req *connect.Request[sessionv1.GetSessionTurnTimelineRequest],
) (*connect.Response[sessionv1.GetSessionTurnTimelineResponse], error) {
	r := s.store.GetByUUID(req.Msg.ConversationId)
	if r == nil {
		return connect.NewResponse(&sessionv1.GetSessionTurnTimelineResponse{}), nil
	}
	turns := make([]*sessionv1.TurnTokenStat, 0, len(r.TurnTimeline))
	for _, turn := range r.TurnTimeline {
		stat := &sessionv1.TurnTokenStat{
			Model:               turn.Model,
			InputTokens:         turn.Input,
			OutputTokens:        turn.Output,
			CacheCreationTokens: turn.CacheCreation,
			CacheReadTokens:     turn.CacheRead,
			// Cloned defensively: turn.ToolNames aliases the TokenStore's
			// cached ParseResult backing array; the outbound proto message
			// must not share that slice.
			ToolNames: slices.Clone(turn.ToolNames),
		}
		if !turn.Timestamp.IsZero() {
			stat.Timestamp = timestamppb.New(turn.Timestamp)
		}
		turns = append(turns, stat)
	}
	return connect.NewResponse(&sessionv1.GetSessionTurnTimelineResponse{Turns: turns}), nil
}

// DismissFinding persists a WasteFinding dismissal keyed by its finding_id,
// so subsequent GetInsightsSummary calls exclude it. Returns CodeUnimplemented
// when no DismissedFindingsRepository has been wired (see
// SetDismissedFindingsStore).
// +api: DismissFinding
func (s *InsightsService) DismissFinding(
	ctx context.Context,
	req *connect.Request[sessionv1.DismissFindingRequest],
) (*connect.Response[sessionv1.DismissFindingResponse], error) {
	if s.dismissedFindings == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("finding dismissal is not available"))
	}
	if req.Msg.FindingId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("finding_id is required"))
	}

	err := s.dismissedFindings.DismissFinding(ctx, session.DismissedFindingData{
		FindingID:      req.Msg.FindingId,
		SessionID:      req.Msg.SessionId,
		ConversationID: req.Msg.ConversationId,
		FindingType:    int32(req.Msg.FindingType),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("dismiss finding: %w", err))
	}
	s.summaries.invalidate() // dismissed findings must disappear from the next summary

	return connect.NewResponse(&sessionv1.DismissFindingResponse{Success: true}), nil
}

// ---------- helpers ----------

// sessionTimestamps returns the first and last message timestamps from a ParseResult.
func sessionTimestamps(r *tokens.ParseResult) (first, last time.Time) {
	for _, turn := range r.TurnTimeline {
		if turn.Timestamp.IsZero() {
			continue
		}
		if first.IsZero() || turn.Timestamp.Before(first) {
			first = turn.Timestamp
		}
		if last.IsZero() || turn.Timestamp.After(last) {
			last = turn.Timestamp
		}
	}
	return first, last
}

// dailyBucketKey returns the "2006-01-02" string for bucketing by day.
// Prefers lastTs; falls back to fileModTime.
func dailyBucketKey(lastTs time.Time, fileModTime time.Time) string {
	t := lastTs
	if t.IsZero() {
		t = fileModTime
	}
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// sessionTopTools builds a sorted slice of TopToolEntry for a single session,
// including per-tool cost attribution (Epic 1.2's AttributeToolCosts,
// ADR-001). Cost population never changes the call-count-desc sort order —
// that contract predates this project and callers rely on "top tools" still
// meaning "most-called."
func sessionTopTools(r *tokens.ParseResult, pt *tokens.PricingTable) []*sessionv1.TopToolEntry {
	type entry struct {
		name      string
		callCount int32
		mcpServer string
	}
	entries := make([]entry, 0, len(r.ToolUsage))
	for _, stat := range r.ToolUsage {
		entries = append(entries, entry{
			name: stat.ToolName,
			// #nosec G115 -- stat.CallCount is a per-session tool-call count, far below int32 range.
			callCount: int32(stat.CallCount),
			mcpServer: stat.MCPServer,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].callCount > entries[j].callCount
	})
	const maxTopTools = 10
	if len(entries) > maxTopTools {
		entries = entries[:maxTopTools]
	}

	costs, doubleCounted, unpriced := tokens.AttributeToolCosts(r, pt)

	result := make([]*sessionv1.TopToolEntry, 0, len(entries))
	for _, e := range entries {
		cost, hasCost := costs[e.name]
		result = append(result, &sessionv1.TopToolEntry{
			ToolName:  e.name,
			CallCount: e.callCount,
			McpServer: e.mcpServer,
			CostUsd:   cost,
			// A tool that never once had a priced turn stays distinguishable
			// from a genuinely free one — see CostUnpriced's doc comment.
			CostMayDoubleCount: doubleCounted[e.name],
			CostUnpriced:       unpriced[e.name] && !hasCost,
		})
	}
	return result
}

// roleCostAccumulator collects one SessionRole's RoleCostBreakdown plus its
// per-item ItemRoleCost buckets while GetInsightsSummary's main loop runs;
// buildRoleBreakdown flattens it into the response's sorted role_breakdown slice.
type roleCostAccumulator struct {
	breakdown *sessionv1.RoleCostBreakdown
	items     map[string]*sessionv1.ItemRoleCost // keyed by ItemID
}

// accumulateRoleCost folds one session's cost into roleAccums, bucketed by
// meta.Role (empty string for a session with no backlog-item attribution —
// kept, never dropped) and, within that bucket, by meta.ItemID. An unpriced
// session increments UnpricedSessionCount and is excluded from
// EstimatedCostUsd in both the role bucket and its item entry — the same
// exclusion rule GetInsightsSummary applies to total_cost_usd, so
// sum(role_breakdown[].estimated_cost_usd) always equals total_cost_usd.
func accumulateRoleCost(roleAccums map[string]*roleCostAccumulator, meta SessionMeta, costUSD float64, unpriced bool) {
	ra := roleAccums[meta.Role]
	if ra == nil {
		ra = &roleCostAccumulator{
			breakdown: &sessionv1.RoleCostBreakdown{SessionRole: meta.Role},
			items:     make(map[string]*sessionv1.ItemRoleCost),
		}
		roleAccums[meta.Role] = ra
	}
	ra.breakdown.SessionCount++
	if unpriced {
		ra.breakdown.UnpricedSessionCount++
	} else {
		ra.breakdown.EstimatedCostUsd += costUSD
	}

	item := ra.items[meta.ItemID]
	if item == nil {
		item = &sessionv1.ItemRoleCost{ItemId: meta.ItemID, ItemTitle: meta.ItemTitle}
		ra.items[meta.ItemID] = item
	}
	item.SessionCount++
	if unpriced {
		item.UnpricedSessionCount++
	} else {
		item.EstimatedCostUsd += costUSD
	}
}

// buildRoleBreakdown flattens roleAccums into a slice sorted by cost
// descending (matching activityBreakdown's existing sort), with each
// bucket's items sorted the same way.
func buildRoleBreakdown(roleAccums map[string]*roleCostAccumulator) []*sessionv1.RoleCostBreakdown {
	roles := make([]*sessionv1.RoleCostBreakdown, 0, len(roleAccums))
	for _, ra := range roleAccums {
		items := make([]*sessionv1.ItemRoleCost, 0, len(ra.items))
		for _, it := range ra.items {
			items = append(items, it)
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].EstimatedCostUsd != items[j].EstimatedCostUsd {
				return items[i].EstimatedCostUsd > items[j].EstimatedCostUsd
			}
			return items[i].ItemId < items[j].ItemId
		})
		ra.breakdown.Items = items
		roles = append(roles, ra.breakdown)
	}
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].EstimatedCostUsd != roles[j].EstimatedCostUsd {
			return roles[i].EstimatedCostUsd > roles[j].EstimatedCostUsd
		}
		return roles[i].SessionRole < roles[j].SessionRole
	})
	return roles
}

// sortedKeys returns the true keys of a map[string]bool as a sorted slice.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// buildTopEntries builds a TopEntry slice sorted by activation count (desc).
func buildTopEntries(counts map[string]int32, limit int) []*sessionv1.TopEntry {
	type kv struct {
		name  string
		count int32
	}
	sorted := make([]kv, 0, len(counts))
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	result := make([]*sessionv1.TopEntry, 0, len(sorted))
	for _, e := range sorted {
		result = append(result, &sessionv1.TopEntry{
			Name:            e.name,
			ActivationCount: e.count,
		})
	}
	return result
}

// buildTopToolEntries builds a TopEntry slice for tool call counts.
func buildTopToolEntries(toolCounts map[string]int64, limit int) []*sessionv1.TopEntry {
	type kv struct {
		name  string
		count int64
	}
	sorted := make([]kv, 0, len(toolCounts))
	for k, v := range toolCounts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	result := make([]*sessionv1.TopEntry, 0, len(sorted))
	for _, e := range sorted {
		result = append(result, &sessionv1.TopEntry{
			Name: e.name,
			// #nosec G115 -- e.count is an aggregated skill-activation count, far below int32 range.
			ActivationCount: int32(e.count),
		})
	}
	return result
}
