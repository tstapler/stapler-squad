package services

import (
	"context"
	"sort"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// summaryFilter is GetInsightsSummaryRequest's filter set, parsed once.
type summaryFilter struct {
	from, to             time.Time
	modelFilter          string
	sessionIDFilter      string
	conversationIDFilter string
	includeOrphans       bool
}

func newSummaryFilter(msg *sessionv1.GetInsightsSummaryRequest) summaryFilter {
	f := summaryFilter{includeOrphans: msg.IncludeOrphans}
	if msg.From != nil {
		f.from = msg.From.AsTime()
	}
	if msg.To != nil {
		f.to = msg.To.AsTime()
	}
	if msg.ModelFilter != nil {
		f.modelFilter = *msg.ModelFilter
	}
	if msg.SessionIdFilter != nil {
		f.sessionIDFilter = *msg.SessionIdFilter
	}
	if msg.ConversationIdFilter != nil {
		f.conversationIDFilter = *msg.ConversationIdFilter
	}
	return f
}

// excludesByRange applies the time-range and model filters, which run before
// a transcript is associated with a session.
func (f summaryFilter) excludesByRange(r *tokens.ParseResult, firstTs, lastTs time.Time) bool {
	if !f.from.IsZero() && !lastTs.IsZero() && lastTs.Before(f.from) {
		return true
	}
	if !f.to.IsZero() && !firstTs.IsZero() && firstTs.After(f.to) {
		return true
	}
	return f.modelFilter != "" && tokens.NormalizeModelFamily(r.PrimaryModel) != f.modelFilter
}

// excludesBySession applies the orphan and session/conversation ID filters.
// Either ID filter, if set, can independently match and keep the session — this
// lets an orphan session (sessionID always "") be selected via the conversation
// filter. If both are set and neither matches, the session is excluded.
func (f summaryFilter) excludesBySession(r *tokens.ParseResult, sessionID string, isOrphan bool) bool {
	if isOrphan && !f.includeOrphans {
		return true
	}
	hasSession, hasConversation := f.sessionIDFilter != "", f.conversationIDFilter != ""
	if !hasSession && !hasConversation {
		return false
	}
	matched := (hasSession && sessionID == f.sessionIDFilter) ||
		(hasConversation && r.SessionUUID == f.conversationIDFilter)
	return !matched
}

// createdAtOutOfRange reports whether a transcript-less row's CreatedAt falls
// outside the requested range (a zero CreatedAt is never excluded).
func (f summaryFilter) createdAtOutOfRange(createdAt time.Time) bool {
	if createdAt.IsZero() {
		return false
	}
	return (!f.from.IsZero() && createdAt.Before(f.from)) || (!f.to.IsZero() && createdAt.After(f.to))
}

// summaryAccumulator holds GetInsightsSummary's running aggregates.
type summaryAccumulator struct {
	pricing *tokens.PricingTable

	totalCostUSD         float64
	totalInputTokens     int64
	totalOutputTokens    int64
	totalCacheReadTokens int64
	cacheHitDenom        int64 // input + cache_read

	dailyMap    map[string]*sessionv1.DailyTokenBucket
	modelMap    map[string]*sessionv1.ModelBreakdown
	skillMap    map[string]int32
	toolMap     map[string]int64
	activityMap map[sessionv1.ActivityType]*sessionv1.ActivityCostBreakdown

	allUnpricedFamilies map[string]bool
	dailyUnpriced       map[string]map[string]bool
	roleAccums          map[string]*roleCostAccumulator

	// transcriptCoveredSessionIDs / ...ConversationUUIDs are every ID resolved
	// from a transcript; the transcript-less fold-in passes skip rows already
	// counted here so nothing is double-counted.
	transcriptCoveredSessionIDs        map[string]bool
	transcriptCoveredConversationUUIDs map[string]bool

	sessions []*sessionv1.SessionTokenSummary
	findings []*sessionv1.WasteFinding
}

func newSummaryAccumulator(pricing *tokens.PricingTable, capacity int) *summaryAccumulator {
	return &summaryAccumulator{
		pricing:                            pricing,
		dailyMap:                           make(map[string]*sessionv1.DailyTokenBucket),
		modelMap:                           make(map[string]*sessionv1.ModelBreakdown),
		skillMap:                           make(map[string]int32),
		toolMap:                            make(map[string]int64),
		activityMap:                        make(map[sessionv1.ActivityType]*sessionv1.ActivityCostBreakdown),
		allUnpricedFamilies:                make(map[string]bool),
		dailyUnpriced:                      make(map[string]map[string]bool),
		roleAccums:                         make(map[string]*roleCostAccumulator),
		transcriptCoveredSessionIDs:        make(map[string]bool),
		transcriptCoveredConversationUUIDs: make(map[string]bool),
		sessions:                           make([]*sessionv1.SessionTokenSummary, 0, capacity),
		findings:                           make([]*sessionv1.WasteFinding, 0, capacity),
	}
}

// addFindings appends r's waste findings (Epic 1.1) minus dismissed ones.
// ComputeFindings isolates each detector's panic, so one malformed session
// can never fail the whole request.
func (a *summaryAccumulator) addFindings(r *tokens.ParseResult, sessionID string, dismissed map[string]bool) {
	for _, f := range tokens.ComputeFindings(r, a.pricing) {
		findingID := tokens.ComputeFindingID(sessionID, r.SessionUUID, f.Type, f.Message)
		if dismissed[findingID] {
			continue
		}
		a.findings = append(a.findings, &sessionv1.WasteFinding{
			FindingType:     f.Type,
			Severity:        f.Severity,
			DollarImpactUsd: float64(f.DollarImpact),
			SessionId:       sessionID,
			ConversationId:  r.SessionUUID,
			Message:         f.Message,
			FindingId:       findingID,
		})
	}
}

// addSession folds one in-range session into every aggregate.
func (a *summaryAccumulator) addSession(
	r *tokens.ParseResult,
	summary *sessionv1.SessionTokenSummary,
	groupedMeta SessionMeta,
	lastTs time.Time,
) {
	costUSD, unpriced := summary.EstimatedCostUsd, summary.UnpricedModels
	sessionUnpriced := len(unpriced) > 0
	for _, f := range unpriced {
		a.allUnpricedFamilies[f] = true
	}
	// Computed once and reused for the daily rollup and the model breakdown.
	modelFamilyCosts, unpricedFamilies := a.pricing.ModelFamilyCost(r)

	ab := a.activityMap[summary.ActivityType]
	if ab == nil {
		ab = &sessionv1.ActivityCostBreakdown{ActivityType: summary.ActivityType}
		a.activityMap[summary.ActivityType] = ab
	}
	ab.EstimatedCostUsd += costUSD
	ab.SessionCount++

	a.sessions = append(a.sessions, summary)

	// An unpriced session's cost is excluded from total_cost_usd (not just
	// $0-added) so it stays in sync with role_breakdown's identical exclusion
	// rule — see accumulateRoleCost's doc comment and Story 4.1.3.
	if !sessionUnpriced {
		a.totalCostUSD += costUSD
	}
	accumulateRoleCost(a.roleAccums, groupedMeta, costUSD, sessionUnpriced)
	a.totalInputTokens += r.TotalInput
	a.totalOutputTokens += r.TotalOutput
	a.totalCacheReadTokens += r.CacheRead
	a.cacheHitDenom += r.TotalInput + r.CacheRead

	a.addDaily(r, lastTs, costUSD, modelFamilyCosts, unpricedFamilies)
	a.addModels(r, modelFamilyCosts, unpricedFamilies)

	for _, sa := range r.SkillActivations {
		a.skillMap[sa.Name]++
	}
	for toolName, stat := range r.ToolUsage {
		a.toolMap[toolName] += int64(stat.CallCount) //nolint:gosec
	}
}

// addDaily buckets by calendar day of last message (or FileModTime).
func (a *summaryAccumulator) addDaily(
	r *tokens.ParseResult,
	lastTs time.Time,
	costUSD float64,
	modelFamilyCosts map[string]float64,
	unpricedFamilies map[string]bool,
) {
	bucketDay := dailyBucketKey(lastTs, r.FileModTime)
	if bucketDay == "" {
		return
	}
	b := a.dailyMap[bucketDay]
	if b == nil {
		t, _ := time.Parse("2006-01-02", bucketDay)
		b = &sessionv1.DailyTokenBucket{
			Date:          timestamppb.New(t),
			CostByModel:   make(map[string]float64),
			TokensByModel: make(map[string]int64),
		}
		a.dailyMap[bucketDay] = b
	}
	b.TotalInputTokens += r.TotalInput
	b.TotalOutputTokens += r.TotalOutput
	b.CacheReadTokens += r.CacheRead
	b.EstimatedCostUsd += costUSD
	b.SessionCount++
	for family, cost := range modelFamilyCosts {
		b.CostByModel[family] += cost
	}
	for _, turn := range r.TurnTimeline {
		if family := tokens.NormalizeModelFamily(turn.Model); family != "" {
			b.TokensByModel[family] += turn.Input + turn.Output
		}
	}
	if a.dailyUnpriced[bucketDay] == nil {
		a.dailyUnpriced[bucketDay] = make(map[string]bool)
	}
	for family := range unpricedFamilies {
		a.dailyUnpriced[bucketDay][family] = true
	}
}

func (a *summaryAccumulator) modelRow(family string) *sessionv1.ModelBreakdown {
	mb := a.modelMap[family]
	if mb == nil {
		mb = &sessionv1.ModelBreakdown{ModelFamily: family}
		a.modelMap[family] = mb
	}
	return mb
}

// addModels folds r's per-family token counts, costs and session counts.
func (a *summaryAccumulator) addModels(
	r *tokens.ParseResult,
	modelFamilyCosts map[string]float64,
	unpricedFamilies map[string]bool,
) {
	for _, turn := range r.TurnTimeline {
		family := tokens.NormalizeModelFamily(turn.Model)
		// Skip turns with no usage across every counter (e.g. "<synthetic>"
		// turns) — nothing to break out into a model row.
		if family == "" || (turn.Input == 0 && turn.Output == 0 && turn.CacheRead == 0 && turn.CacheCreation == 0) {
			continue
		}
		mb := a.modelRow(family)
		mb.TotalInputTokens += turn.Input
		mb.TotalOutputTokens += turn.Output
		mb.CacheReadTokens += turn.CacheRead
	}
	for family, cost := range modelFamilyCosts {
		mb := a.modelRow(family)
		mb.EstimatedCostUsd += cost
		mb.SessionCount++
	}
	// Flag families with usage but no pricing entry; SessionCount increments the
	// same way so such a row doesn't show tokens next to SessionCount: 0.
	for family := range unpricedFamilies {
		mb := a.modelRow(family)
		mb.PricingUnavailable = true
		mb.SessionCount++
	}
}

// foldTranscriptless folds a row that has no Claude transcript (an ItemSession
// or deleted-item ledger row) into total_cost_usd and role_breakdown only.
func (a *summaryAccumulator) foldTranscriptless(f summaryFilter, createdAt time.Time, costPriced bool, cost float64, meta SessionMeta) {
	if f.createdAtOutOfRange(createdAt) {
		return
	}
	if costPriced {
		a.totalCostUSD += cost
	}
	accumulateRoleCost(a.roleAccums, meta, cost, !costPriced)
}

// foldBacklogEntries is Story 4.1.4: ItemSession rows with no matching
// transcript (e.g. a Gemini-priced triage call that writes no JSONL file).
func (a *summaryAccumulator) foldBacklogEntries(f summaryFilter, smr sessionMetaResult) {
	for _, e := range smr.entries {
		if e.SessionUUID == "" || a.transcriptCoveredSessionIDs[e.SessionUUID] {
			continue
		}
		a.foldTranscriptless(f, e.CreatedAt, e.CostPriced, e.EstimatedCostUsd,
			SessionMeta{Role: e.SessionRole, ItemID: e.ItemID, ItemTitle: e.ItemTitle})
	}
}

// foldLedgerEntries is Story 3: once an item and its ItemSession row are
// deleted, the ledger is the only surviving cost record.
func (a *summaryAccumulator) foldLedgerEntries(f summaryFilter, smr sessionMetaResult) {
	for _, e := range smr.ledgerEntries {
		if e.ConversationUUID == "" || a.transcriptCoveredConversationUUIDs[e.ConversationUUID] {
			continue
		}
		a.foldTranscriptless(f, e.CreatedAt, e.CostPriced, e.EstimatedCostUsd,
			SessionMeta{Role: e.SessionRole, ItemID: e.ItemID, ItemTitle: e.ItemTitle})
	}
}

func sortedDaily(dailyMap map[string]*sessionv1.DailyTokenBucket, dailyUnpriced map[string]map[string]bool) []*sessionv1.DailyTokenBucket {
	keys := make([]string, 0, len(dailyMap))
	for k := range dailyMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	daily := make([]*sessionv1.DailyTokenBucket, 0, len(keys))
	for _, k := range keys {
		dailyMap[k].UnpricedModels = sortedKeys(dailyUnpriced[k])
		daily = append(daily, dailyMap[k])
	}
	return daily
}

func sortedModels(modelMap map[string]*sessionv1.ModelBreakdown) []*sessionv1.ModelBreakdown {
	families := make([]string, 0, len(modelMap))
	for k := range modelMap {
		families = append(families, k)
	}
	sort.Strings(families)
	models := make([]*sessionv1.ModelBreakdown, 0, len(families))
	for _, k := range families {
		models = append(models, modelMap[k])
	}
	return models
}

func sortedActivity(activityMap map[sessionv1.ActivityType]*sessionv1.ActivityCostBreakdown) []*sessionv1.ActivityCostBreakdown {
	out := make([]*sessionv1.ActivityCostBreakdown, 0, len(activityMap))
	for _, ab := range activityMap {
		out = append(out, ab)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EstimatedCostUsd > out[j].EstimatedCostUsd })
	return out
}

// rankedFindings sorts by dollar impact descending and caps at findingsCap.
func rankedFindings(findings []*sessionv1.WasteFinding) []*sessionv1.WasteFinding {
	sort.Slice(findings, func(i, j int) bool { return findings[i].DollarImpactUsd > findings[j].DollarImpactUsd })
	if len(findings) > findingsCap {
		return findings[:findingsCap]
	}
	return findings
}

// response assembles the final message from the accumulated aggregates.
func (a *summaryAccumulator) response(isLoading bool) *sessionv1.GetInsightsSummaryResponse {
	cacheHitRate := float64(0)
	if a.cacheHitDenom > 0 {
		cacheHitRate = float64(a.totalCacheReadTokens) / float64(a.cacheHitDenom)
	}
	return &sessionv1.GetInsightsSummaryResponse{
		Sessions:             a.sessions,
		TotalCostUsd:         a.totalCostUSD,
		TotalInputTokens:     a.totalInputTokens,
		TotalOutputTokens:    a.totalOutputTokens,
		TotalCacheReadTokens: a.totalCacheReadTokens,
		OverallCacheHitRate:  cacheHitRate,
		Daily:                sortedDaily(a.dailyMap, a.dailyUnpriced),
		Models:               sortedModels(a.modelMap),
		TopSkills:            buildTopEntries(a.skillMap, 20),
		TopTools:             buildTopToolEntries(a.toolMap, 20),
		IsLoading:            isLoading,
		PricingAsOf:          timestamppb.New(a.pricing.LoadedAt),
		UnpricedModels:       sortedKeys(a.allUnpricedFamilies),
		Findings:             rankedFindings(a.findings),
		ActivityBreakdown:    sortedActivity(a.activityMap),
		RoleBreakdown:        buildRoleBreakdown(a.roleAccums),
	}
}

// dismissedFindingIDs is fetched once per request (not per-finding).
func (s *InsightsService) dismissedFindingIDs(ctx context.Context) map[string]bool {
	if s.dismissedFindings == nil {
		return nil
	}
	ids, err := s.dismissedFindings.ListDismissedFindingIDs(ctx)
	if err != nil {
		log.Warn("insights: failed to load dismissed finding IDs, findings will not be filtered", "err", err)
		return nil
	}
	return ids
}

func (s *InsightsService) startBackfillOnce() {
	s.backfillOnce.Do(func() {
		go func() {
			bctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := s.BackfillConversationUUIDs(bctx); err != nil {
				log.Warn("insights backfill failed", "err", err)
			}
		}()
	})
}

// computeInsightsSummary is the uncached aggregate over the token store.
func (s *InsightsService) computeInsightsSummary(
	ctx context.Context,
	msg *sessionv1.GetInsightsSummaryRequest,
) (*sessionv1.GetInsightsSummaryResponse, error) {
	s.startBackfillOnce()
	results := s.store.GetAll()
	filter := newSummaryFilter(msg)
	acc := newSummaryAccumulator(s.pricing, len(results))

	var sessionSnapshot []tokens.SessionRecord
	if s.associator != nil {
		sessionSnapshot = s.associator.Snapshot()
	}
	smr := s.sessionMetaForSessions(ctx)
	dismissed := s.dismissedFindingIDs(ctx)

	for _, r := range results {
		if r == nil {
			continue
		}
		s.accumulateTranscript(acc, filter, r, sessionSnapshot, smr.meta, dismissed)
	}

	// Emit a deduped runtime warning for any newly-observed unpriced family
	// (AC-5 runtime-signal half) — once per family per process, not per request.
	s.warnNewUnpricedFamilies(acc.allUnpricedFamilies)

	acc.foldBacklogEntries(filter, smr)
	acc.foldLedgerEntries(filter, smr)

	return acc.response(s.store.IsLoading()), nil
}

// accumulateTranscript filters r and, if kept, folds it into acc.
func (s *InsightsService) accumulateTranscript(
	acc *summaryAccumulator,
	filter summaryFilter,
	r *tokens.ParseResult,
	sessionSnapshot []tokens.SessionRecord,
	sessionMeta map[string]SessionMeta,
	dismissed map[string]bool,
) {
	firstTs, lastTs := sessionTimestamps(r)
	if filter.excludesByRange(r, firstTs, lastTs) {
		return
	}

	rec, isOrphan := tokens.SessionRecord{}, true
	if s.associator != nil {
		rec, isOrphan = s.associator.AssociateRecordWithSnapshot(r, sessionSnapshot)
	}
	sessionID := rec.SessionID
	attributed, byConversation := metaFor(sessionMeta, sessionID, r)
	if byConversation {
		isOrphan = false // a deleted session's transcript, still attributable by conversation UUID
		if attributed.ItemSessionUUID != "" {
			acc.transcriptCoveredSessionIDs[attributed.ItemSessionUUID] = true
		}
	}
	// Coverage is recorded before the session/orphan filters so the fold-in
	// passes still recognize this transcript even if this response excludes it.
	if sessionID != "" {
		acc.transcriptCoveredSessionIDs[sessionID] = true
	}
	if r.SessionUUID != "" {
		acc.transcriptCoveredConversationUUIDs[r.SessionUUID] = true
	}
	if filter.excludesBySession(r, sessionID, isOrphan) {
		return
	}

	summary, groupedMeta := buildSessionSummaryForRecord(r, s.pricing, rec, isOrphan, sessionMeta)
	// Findings are a separate, non-summable response-level list (ADR-002).
	acc.addFindings(r, sessionID, dismissed)
	acc.addSession(r, summary, groupedMeta, lastTs)
}
