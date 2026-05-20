package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: InsightsService must implement the generated handler.
var _ sessionv1connect.InsightsServiceHandler = (*InsightsService)(nil)

// InsightsService implements the ConnectRPC InsightsServiceHandler.
// It reads from a TokenStoreReader to serve token usage analytics.
type InsightsService struct {
	store      tokens.TokenStoreReader
	pricing    *tokens.PricingTable
	associator *tokens.Associator
}

// NewInsightsService creates a new InsightsService.
func NewInsightsService(
	store tokens.TokenStoreReader,
	pricing *tokens.PricingTable,
	associator *tokens.Associator,
) *InsightsService {
	return &InsightsService{
		store:      store,
		pricing:    pricing,
		associator: associator,
	}
}

// GetInsightsSummary returns aggregated token and cost data for a time range.
func (s *InsightsService) GetInsightsSummary(
	_ context.Context,
	req *connect.Request[sessionv1.GetInsightsSummaryRequest],
) (*connect.Response[sessionv1.GetInsightsSummaryResponse], error) {
	results := s.store.GetAll()
	msg := req.Msg

	// Extract time filter bounds.
	var fromTime, toTime time.Time
	if msg.From != nil {
		fromTime = msg.From.AsTime()
	}
	if msg.To != nil {
		toTime = msg.To.AsTime()
	}

	// Accumulate aggregates.
	var (
		totalCostUSD         float64
		totalInputTokens     int64
		totalOutputTokens    int64
		totalCacheReadTokens int64
		totalCacheHitNumer   int64 // for cache hit rate: cache_read
		totalCacheHitDenom   int64 // for cache hit rate: input + cache_read

		dailyMap  = make(map[string]*sessionv1.DailyTokenBucket) // key = "2026-05-15"
		modelMap  = make(map[string]*sessionv1.ModelBreakdown)   // key = normalized family
		skillMap  = make(map[string]int32)                        // skill name → activation count
		toolMap   = make(map[string]int64)                        // tool name → call count
	)

	sessions := make([]*sessionv1.SessionTokenSummary, 0, len(results))

	for _, r := range results {
		if r == nil {
			continue
		}

		// Determine timeline bounds for this session.
		firstTs, lastTs := sessionTimestamps(r)

		// Apply time filter: if filter set, skip sessions outside range.
		if !fromTime.IsZero() && !lastTs.IsZero() && lastTs.Before(fromTime) {
			continue
		}
		if !toTime.IsZero() && !firstTs.IsZero() && firstTs.After(toTime) {
			continue
		}

		// Apply model filter.
		if msg.ModelFilter != nil && *msg.ModelFilter != "" {
			if tokens.NormalizeModelFamily(r.PrimaryModel) != *msg.ModelFilter {
				continue
			}
		}

		// Determine session ID and orphan status.
		sessionID, isOrphan := "", true
		if s.associator != nil {
			sessionID, isOrphan = s.associator.Associate(r)
		}

		// Apply orphan filter.
		if isOrphan && !msg.IncludeOrphans {
			continue
		}

		// Apply session ID filter.
		if msg.SessionIdFilter != nil && *msg.SessionIdFilter != "" {
			if sessionID != *msg.SessionIdFilter {
				continue
			}
		}

		costUSD := s.pricing.EstimateCost(r)
		cacheHitRate := computeCacheHitRate(r.TotalInput, r.CacheRead)

		// Build top tools list for this session.
		topTools := sessionTopTools(r)

		// Build skill activation names.
		skillNames := make([]string, 0, len(r.SkillActivations))
		for _, sa := range r.SkillActivations {
			skillNames = append(skillNames, sa.Name)
		}

		summary := &sessionv1.SessionTokenSummary{
			SessionId:           sessionID,
			ConversationId:      r.SessionUUID,
			ProjectPath:         r.ProjectPath,
			PrimaryModel:        r.PrimaryModel,
			TotalInputTokens:    r.TotalInput,
			TotalOutputTokens:   r.TotalOutput,
			CacheCreationTokens: r.CacheCreation,
			CacheReadTokens:     r.CacheRead,
			EstimatedCostUsd:    costUSD,
			CacheHitRate:        cacheHitRate,
			MessageCount:        int32(r.MessageCount), //nolint:gosec
			IsOrphan:            isOrphan,
			SkillActivations:    skillNames,
			TopTools:            topTools,
		}
		if !firstTs.IsZero() {
			summary.FirstMessageAt = timestamppb.New(firstTs)
		}
		if !lastTs.IsZero() {
			summary.LastMessageAt = timestamppb.New(lastTs)
		}

		sessions = append(sessions, summary)

		// Aggregate global totals.
		totalCostUSD += costUSD
		totalInputTokens += r.TotalInput
		totalOutputTokens += r.TotalOutput
		totalCacheReadTokens += r.CacheRead
		totalCacheHitNumer += r.CacheRead
		totalCacheHitDenom += r.TotalInput + r.CacheRead

		// Daily rollup — bucket by calendar day of last message (or FileModTime).
		bucketDay := dailyBucketKey(lastTs, r.FileModTime)
		if bucketDay != "" {
			b := dailyMap[bucketDay]
			if b == nil {
				t, _ := time.Parse("2006-01-02", bucketDay)
				b = &sessionv1.DailyTokenBucket{
					Date:          timestamppb.New(t),
					CostByModel:   make(map[string]float64),
					TokensByModel: make(map[string]int64),
				}
				dailyMap[bucketDay] = b
			}
			b.TotalInputTokens += r.TotalInput
			b.TotalOutputTokens += r.TotalOutput
			b.CacheReadTokens += r.CacheRead
			b.EstimatedCostUsd += costUSD
			b.SessionCount++
			// Per-model breakdown within this day.
			modelFamilyCostsForDay := s.pricing.ModelFamilyCost(r)
			for family, cost := range modelFamilyCostsForDay {
				b.CostByModel[family] += cost
			}
			for _, turn := range r.TurnTimeline {
				family := tokens.NormalizeModelFamily(turn.Model)
				if family != "" {
					b.TokensByModel[family] += turn.Input + turn.Output
				}
			}
		}

		// Model breakdown.
		modelFamilyCosts := s.pricing.ModelFamilyCost(r)
		for _, turn := range r.TurnTimeline {
			family := tokens.NormalizeModelFamily(turn.Model)
			if family == "" {
				continue
			}
			mb := modelMap[family]
			if mb == nil {
				mb = &sessionv1.ModelBreakdown{ModelFamily: family}
				modelMap[family] = mb
			}
			mb.TotalInputTokens += turn.Input
			mb.TotalOutputTokens += turn.Output
			mb.CacheReadTokens += turn.CacheRead
		}
		// Set per-model costs and session count.
		for family, cost := range modelFamilyCosts {
			mb := modelMap[family]
			if mb == nil {
				mb = &sessionv1.ModelBreakdown{ModelFamily: family}
				modelMap[family] = mb
			}
			mb.EstimatedCostUsd += cost
			mb.SessionCount++
		}

		// Skill activations.
		for _, sa := range r.SkillActivations {
			skillMap[sa.Name]++
		}

		// Tool usage.
		for toolName, stat := range r.ToolUsage {
			toolMap[toolName] += int64(stat.CallCount) //nolint:gosec
		}
	}

	// Build sorted daily slice.
	dailyKeys := make([]string, 0, len(dailyMap))
	for k := range dailyMap {
		dailyKeys = append(dailyKeys, k)
	}
	sort.Strings(dailyKeys)
	daily := make([]*sessionv1.DailyTokenBucket, 0, len(dailyKeys))
	for _, k := range dailyKeys {
		daily = append(daily, dailyMap[k])
	}

	// Build sorted model slice.
	modelFamilies := make([]string, 0, len(modelMap))
	for k := range modelMap {
		modelFamilies = append(modelFamilies, k)
	}
	sort.Strings(modelFamilies)
	models := make([]*sessionv1.ModelBreakdown, 0, len(modelFamilies))
	for _, k := range modelFamilies {
		models = append(models, modelMap[k])
	}

	// Build top skills (sorted by activation count desc).
	topSkills := buildTopEntries(skillMap, 20)
	topTools := buildTopToolEntries(toolMap, 20)

	// Overall cache hit rate.
	overallCacheHitRate := float64(0)
	if totalCacheHitDenom > 0 {
		overallCacheHitRate = float64(totalCacheHitNumer) / float64(totalCacheHitDenom)
	}

	resp := &sessionv1.GetInsightsSummaryResponse{
		Sessions:            sessions,
		TotalCostUsd:        totalCostUSD,
		TotalInputTokens:    totalInputTokens,
		TotalOutputTokens:   totalOutputTokens,
		TotalCacheReadTokens: totalCacheReadTokens,
		OverallCacheHitRate: overallCacheHitRate,
		Daily:               daily,
		Models:              models,
		TopSkills:           topSkills,
		TopTools:            topTools,
		IsLoading:           s.store.IsLoading(),
		PricingAsOf:         timestamppb.New(s.pricing.LoadedAt),
	}

	return connect.NewResponse(resp), nil
}

// ListSessionTokens returns per-session token summaries with pagination.
func (s *InsightsService) ListSessionTokens(
	_ context.Context,
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

		sessionID, isOrphan := "", true
		if s.associator != nil {
			sessionID, isOrphan = s.associator.Associate(r)
		}

		costUSD := s.pricing.EstimateCost(r)
		cacheHitRate := computeCacheHitRate(r.TotalInput, r.CacheRead)
		topTools := sessionTopTools(r)

		skillNames := make([]string, 0, len(r.SkillActivations))
		for _, sa := range r.SkillActivations {
			skillNames = append(skillNames, sa.Name)
		}

		summary := &sessionv1.SessionTokenSummary{
			SessionId:           sessionID,
			ConversationId:      r.SessionUUID,
			ProjectPath:         r.ProjectPath,
			PrimaryModel:        r.PrimaryModel,
			TotalInputTokens:    r.TotalInput,
			TotalOutputTokens:   r.TotalOutput,
			CacheCreationTokens: r.CacheCreation,
			CacheReadTokens:     r.CacheRead,
			EstimatedCostUsd:    costUSD,
			CacheHitRate:        cacheHitRate,
			MessageCount:        int32(r.MessageCount), //nolint:gosec
			IsOrphan:            isOrphan,
			SkillActivations:    skillNames,
			TopTools:            topTools,
		}
		if !firstTs.IsZero() {
			summary.FirstMessageAt = timestamppb.New(firstTs)
		}
		if !lastTs.IsZero() {
			summary.LastMessageAt = timestamppb.New(lastTs)
		}
		summaries = append(summaries, summary)
	}

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
	totalCount := int32(len(summaries)) //nolint:gosec
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

// WatchInsights streams summary updates when new JSONL data is parsed.
// Sends an initial "parse_complete" event (or "loading" if still parsing),
// then pushes an "update" event each time the TokenStore processes a new file.
func (s *InsightsService) WatchInsights(
	ctx context.Context,
	_ *connect.Request[sessionv1.WatchInsightsRequest],
	stream *connect.ServerStream[sessionv1.InsightsEvent],
) error {
	// 1. Send initial state.
	allParsed := !s.store.IsLoading()
	initialEvent := &sessionv1.InsightsEvent{
		EventType: "parse_complete",
		AllParsed: allParsed,
	}
	if !allParsed {
		initialEvent.EventType = "loading"
	}
	if err := stream.Send(initialEvent); err != nil {
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
		case _, ok := <-ch:
			if !ok {
				return nil
			}
			evt := &sessionv1.InsightsEvent{
				EventType: "update",
				AllParsed: !s.store.IsLoading(),
			}
			if err := stream.Send(evt); err != nil {
				return fmt.Errorf("send update event: %w", err)
			}
		}
	}
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

// computeCacheHitRate returns cache_read / (input + cache_read), or 0.
func computeCacheHitRate(input, cacheRead int64) float64 {
	denom := input + cacheRead
	if denom == 0 {
		return 0
	}
	return float64(cacheRead) / float64(denom)
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

// sessionTopTools builds a sorted slice of TopToolEntry for a single session.
func sessionTopTools(r *tokens.ParseResult) []*sessionv1.TopToolEntry {
	type entry struct {
		name      string
		callCount int32
		mcpServer string
	}
	entries := make([]entry, 0, len(r.ToolUsage))
	for _, stat := range r.ToolUsage {
		entries = append(entries, entry{
			name:      stat.ToolName,
			callCount: int32(stat.CallCount), //nolint:gosec
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
	result := make([]*sessionv1.TopToolEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, &sessionv1.TopToolEntry{
			ToolName:  e.name,
			CallCount: e.callCount,
			McpServer: e.mcpServer,
		})
	}
	return result
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
			Name:       e.name,
			TokenCount: e.count,
		})
	}
	return result
}
