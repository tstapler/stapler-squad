package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --------------------------------------------------------------------------
// Fake TokenStore (minimal in-memory implementation for tests)
// Implements tokens.TokenStoreReader.
// --------------------------------------------------------------------------

type fakeTokenStore struct {
	results   []*tokens.ParseResult
	isLoading bool
	// subscribeCh, when set, is returned as-is by Subscribe() so a test can
	// push values directly onto it (deterministic control over watchInsights's
	// receive loop, vs. racing a real TokenStore's background walk timing).
	// Subscribe() falls back to a fresh unconnected channel when unset,
	// preserving the original behavior for tests that don't care.
	subscribeCh chan *tokens.ParseResult
}

func (f *fakeTokenStore) GetAll() []*tokens.ParseResult { return f.results }
func (f *fakeTokenStore) GetByUUID(uuid string) *tokens.ParseResult {
	for _, r := range f.results {
		if r.SessionUUID == uuid {
			return r
		}
	}
	return nil
}
func (f *fakeTokenStore) IsLoading() bool { return f.isLoading }
func (f *fakeTokenStore) Subscribe() <-chan *tokens.ParseResult {
	if f.subscribeCh != nil {
		return f.subscribeCh
	}
	return make(chan *tokens.ParseResult, 1)
}
func (f *fakeTokenStore) Unsubscribe(_ <-chan *tokens.ParseResult) {}

// Compile-time assertion: fakeTokenStore must implement TokenStoreReader.
var _ tokens.TokenStoreReader = (*fakeTokenStore)(nil)

// --------------------------------------------------------------------------
// Fake SessionStorage (in-memory list)
// --------------------------------------------------------------------------

type fakeSessionStorage struct {
	records []tokens.SessionRecord
}

func (f *fakeSessionStorage) ListSessionRecords() []tokens.SessionRecord { return f.records }

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// newResult builds a minimal ParseResult for tests.
func newResult(uuid, model, projectPath string, input, output, cacheRead int64, modTime time.Time) *tokens.ParseResult {
	return &tokens.ParseResult{
		SessionUUID:  uuid,
		PrimaryModel: model,
		ProjectPath:  projectPath,
		TotalInput:   input,
		TotalOutput:  output,
		CacheRead:    cacheRead,
		FileModTime:  modTime,
		TurnTimeline: []tokens.TurnStats{
			{
				Model:     model,
				Input:     input,
				Output:    output,
				CacheRead: cacheRead,
				Timestamp: modTime,
			},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{},
	}
}

// newInsightsFixture returns an InsightsService wired with a fake store and
// the default pricing table. Pass nil sessionRecords to get nil associator (all orphans).
func newInsightsFixture(results []*tokens.ParseResult, sessionRecords []tokens.SessionRecord) *InsightsService {
	store := &fakeTokenStore{results: results}
	pricing := tokens.DefaultPricingTable()
	var associator *tokens.Associator
	if sessionRecords != nil {
		storageFake := &fakeSessionStorage{records: sessionRecords}
		associator = tokens.NewAssociator(storageFake)
	}
	return NewInsightsService(store, pricing, associator)
}

// Compile-time assertion: fakeSessionStorage must implement tokens.SessionStorage.
var _ tokens.SessionStorage = (*fakeSessionStorage)(nil)

// --------------------------------------------------------------------------
// TC-GO-30: GetInsightsSummary returns empty response for empty store
// --------------------------------------------------------------------------

func TestGetInsightsSummary_EmptyStore_ReturnsEmptyResponse(t *testing.T) {
	t.Parallel()
	svc := newInsightsFixture(nil, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Sessions)
	assert.Equal(t, float64(0), resp.Msg.TotalCostUsd)
	assert.Equal(t, int64(0), resp.Msg.TotalInputTokens)
}

// --------------------------------------------------------------------------
// TC-GO-31: GetInsightsSummary aggregates totals correctly
// --------------------------------------------------------------------------

func TestGetInsightsSummary_AggregatesTokensAndCost(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "claude-sonnet-4", "/home/user/proj", 1000, 500, 200, now),
		newResult("uuid-2", "claude-sonnet-4", "/home/user/proj2", 2000, 1000, 400, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	assert.Equal(t, int64(3000), resp.Msg.TotalInputTokens)
	assert.Equal(t, int64(1500), resp.Msg.TotalOutputTokens)
	assert.Equal(t, int64(600), resp.Msg.TotalCacheReadTokens)
	assert.Greater(t, resp.Msg.TotalCostUsd, float64(0))
	assert.Len(t, resp.Msg.Sessions, 2)
}

// --------------------------------------------------------------------------
// TC-GO-32: GetInsightsSummary time filter (from/to) works
// --------------------------------------------------------------------------

func TestGetInsightsSummary_TimeFilter_ExcludesOutOfRange(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	nextWeek := now.Add(7 * 24 * time.Hour)

	recent := newResult("uuid-recent", "claude-sonnet-4", "/proj", 1000, 500, 0, now)
	old := newResult("uuid-old", "claude-sonnet-4", "/proj", 999, 499, 0, yesterday.Add(-2*time.Hour))
	results := []*tokens.ParseResult{recent, old}
	svc := newInsightsFixture(results, nil)

	// Filter: from=yesterday → old entry (from 2h before yesterday) should be excluded
	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{
			From:           timestamppb.New(yesterday),
			To:             timestamppb.New(nextWeek),
			IncludeOrphans: true,
		}),
	)

	require.NoError(t, err)
	assert.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "uuid-recent", resp.Msg.Sessions[0].ConversationId)
}

// --------------------------------------------------------------------------
// TC-GO-33: GetInsightsSummary model filter applies correctly
// --------------------------------------------------------------------------

func TestGetInsightsSummary_ModelFilter_OnlyMatchingFamily(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-sonnet", "claude-sonnet-4", "/proj", 1000, 500, 0, now),
		newResult("uuid-opus", "claude-opus-4", "/proj2", 2000, 1000, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	filterFamily := "claude-sonnet-4"
	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{
			ModelFilter:    &filterFamily,
			IncludeOrphans: true,
		}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "uuid-sonnet", resp.Msg.Sessions[0].ConversationId)
}

// --------------------------------------------------------------------------
// TC-GO-34: GetInsightsSummary orphan filter excludes orphans when not requested
// --------------------------------------------------------------------------

func TestGetInsightsSummary_OrphanFilter_ExcludesOrphansWhenNotRequested(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-matched", "claude-sonnet-4", "/home/user/matched", 1000, 500, 0, now),
		newResult("uuid-orphan", "claude-sonnet-4", "/home/user/orphan", 999, 499, 0, now),
	}

	// Only one session record matches uuid-matched via path prefix
	sessionRecords := []tokens.SessionRecord{
		{SessionID: "sess-1", ConversationID: "uuid-matched", Path: "/home/user/matched"},
	}
	svc := newInsightsFixture(results, sessionRecords)

	// IncludeOrphans = false → only the matched session is returned
	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: false}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "uuid-matched", resp.Msg.Sessions[0].ConversationId)
	assert.False(t, resp.Msg.Sessions[0].IsOrphan)
}

// --------------------------------------------------------------------------
// Epic 1.4 Story 1.4.1: conversation_id_filter (deep-linkable orphan lookup)
// --------------------------------------------------------------------------

func TestGetInsightsSummary_WhenConversationIdFilterMatchesOrphanSession_ExpectExactlyThatSessionReturned(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("conv-orphan", "claude-sonnet-4", "/home/user/orphan", 1000, 500, 0, now),
		newResult("conv-other", "claude-sonnet-4", "/home/user/other", 999, 499, 0, now),
	}
	// No session records match either conversation — both are orphans, so
	// session_id_filter could never select either of them.
	svc := newInsightsFixture(results, []tokens.SessionRecord{})

	convFilter := "conv-orphan"
	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{
			ConversationIdFilter: &convFilter,
			IncludeOrphans:       true,
		}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "conv-orphan", resp.Msg.Sessions[0].ConversationId)
	assert.True(t, resp.Msg.Sessions[0].IsOrphan)
}

func TestGetInsightsSummary_WhenSessionIdFilterSetAndNoConversationIdFilter_ExpectOnlyMatchingNonOrphanSessionReturned(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-matched", "claude-sonnet-4", "/home/user/matched", 1000, 500, 0, now),
		newResult("uuid-other", "claude-sonnet-4", "/home/user/other", 999, 499, 0, now),
	}
	sessionRecords := []tokens.SessionRecord{
		{SessionID: "sess-1", ConversationID: "uuid-matched", Path: "/home/user/matched"},
	}
	svc := newInsightsFixture(results, sessionRecords)

	sessionIDFilter := "sess-1"
	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{
			SessionIdFilter: &sessionIDFilter,
			IncludeOrphans:  true,
		}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "uuid-matched", resp.Msg.Sessions[0].ConversationId)
	assert.False(t, resp.Msg.Sessions[0].IsOrphan)
}

// --------------------------------------------------------------------------
// TC-GO-35: GetInsightsSummary daily rollup bucketing
// --------------------------------------------------------------------------

func TestGetInsightsSummary_DailyRollup_BucketsPerCalendarDay(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	day1 := time.Date(2026, 5, 10, 12, 0, 0, 0, loc)
	day2 := time.Date(2026, 5, 11, 12, 0, 0, 0, loc)

	results := []*tokens.ParseResult{
		newResult("uuid-1", "claude-sonnet-4", "/proj", 1000, 500, 0, day1),
		newResult("uuid-2", "claude-sonnet-4", "/proj", 800, 400, 0, day1),
		newResult("uuid-3", "claude-sonnet-4", "/proj", 600, 300, 0, day2),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	// Should have exactly 2 daily buckets.
	require.Len(t, resp.Msg.Daily, 2)
	// Day1 bucket should have 2 sessions.
	assert.Equal(t, int32(2), resp.Msg.Daily[0].SessionCount)
	assert.Equal(t, int64(1800), resp.Msg.Daily[0].TotalInputTokens)
	// Day2 bucket should have 1 session.
	assert.Equal(t, int32(1), resp.Msg.Daily[1].SessionCount)
	assert.Equal(t, int64(600), resp.Msg.Daily[1].TotalInputTokens)
}

// --------------------------------------------------------------------------
// TC-GO-36: GetInsightsSummary model breakdown aggregation
// --------------------------------------------------------------------------

func TestGetInsightsSummary_ModelBreakdown_AggregatesPerFamily(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-s1", "claude-sonnet-4", "/proj", 1000, 500, 0, now),
		newResult("uuid-s2", "claude-sonnet-4", "/proj", 2000, 1000, 0, now),
		newResult("uuid-o1", "claude-opus-4", "/proj", 3000, 1500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Models, 2)

	// Find models by family name.
	modelsByFamily := make(map[string]*sessionv1.ModelBreakdown)
	for _, m := range resp.Msg.Models {
		modelsByFamily[m.ModelFamily] = m
	}

	sonnet, ok := modelsByFamily["claude-sonnet-4"]
	require.True(t, ok, "expected claude-sonnet-4 breakdown")
	assert.Equal(t, int64(3000), sonnet.TotalInputTokens)

	opus, ok := modelsByFamily["claude-opus-4"]
	require.True(t, ok, "expected claude-opus-4 breakdown")
	assert.Equal(t, int64(3000), opus.TotalInputTokens)
}

// --------------------------------------------------------------------------
// TC-GO-37: ListSessionTokens sorts by cost descending
// --------------------------------------------------------------------------

func TestListSessionTokens_SortByCostDesc(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	// opus is more expensive than sonnet.
	results := []*tokens.ParseResult{
		newResult("uuid-cheap", "claude-haiku-3", "/proj", 100, 50, 0, now),
		newResult("uuid-expensive", "claude-opus-4", "/proj", 5000, 2500, 0, now),
		newResult("uuid-mid", "claude-sonnet-4", "/proj", 1000, 500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.ListSessionTokens(
		context.Background(),
		connect.NewRequest(&sessionv1.ListSessionTokensRequest{
			SortBy:   "cost",
			SortDesc: true,
		}),
	)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resp.Msg.Sessions), 2)
	// Most expensive first.
	assert.Equal(t, "uuid-expensive", resp.Msg.Sessions[0].ConversationId)
}

// --------------------------------------------------------------------------
// TC-GO-38: ListSessionTokens pagination
// --------------------------------------------------------------------------

func TestListSessionTokens_Pagination_ReturnsCorrectPage(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := make([]*tokens.ParseResult, 5)
	for i := range results {
		results[i] = newResult(
			"uuid-"+string(rune('A'+i)),
			"claude-sonnet-4",
			"/proj",
			int64(1000*(i+1)),
			int64(500*(i+1)),
			0,
			now.Add(-time.Duration(i)*time.Minute),
		)
	}
	svc := newInsightsFixture(results, nil)

	// First page: size 2.
	resp1, err := svc.ListSessionTokens(
		context.Background(),
		connect.NewRequest(&sessionv1.ListSessionTokensRequest{
			PageSize: 2,
		}),
	)
	require.NoError(t, err)
	assert.Len(t, resp1.Msg.Sessions, 2)
	assert.NotEmpty(t, resp1.Msg.NextPageToken)
	assert.Equal(t, int32(5), resp1.Msg.TotalCount)

	// Second page: use the token from the first page.
	resp2, err := svc.ListSessionTokens(
		context.Background(),
		connect.NewRequest(&sessionv1.ListSessionTokensRequest{
			PageSize:  2,
			PageToken: resp1.Msg.NextPageToken,
		}),
	)
	require.NoError(t, err)
	assert.Len(t, resp2.Msg.Sessions, 2)

	// Third page: last item.
	resp3, err := svc.ListSessionTokens(
		context.Background(),
		connect.NewRequest(&sessionv1.ListSessionTokensRequest{
			PageSize:  2,
			PageToken: resp2.Msg.NextPageToken,
		}),
	)
	require.NoError(t, err)
	assert.Len(t, resp3.Msg.Sessions, 1)
	assert.Empty(t, resp3.Msg.NextPageToken)
}

// --------------------------------------------------------------------------
// TC-GO-39: cache hit rate computation
// --------------------------------------------------------------------------

func TestGetInsightsSummary_CacheHitRate_ComputedCorrectly(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	// input=800, cacheRead=200  → rate = 200/(800+200) = 0.2
	results := []*tokens.ParseResult{
		{
			SessionUUID:  "uuid-cache",
			PrimaryModel: "claude-sonnet-4",
			TotalInput:   800,
			TotalOutput:  400,
			CacheRead:    200,
			FileModTime:  now,
			TurnTimeline: []tokens.TurnStats{
				{Model: "claude-sonnet-4", Input: 800, Output: 400, CacheRead: 200, Timestamp: now},
			},
			ToolUsage: map[string]tokens.ToolTokenStats{},
		},
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.InDelta(t, 0.2, resp.Msg.Sessions[0].CacheHitRate, 0.001)
	assert.InDelta(t, 0.2, resp.Msg.OverallCacheHitRate, 0.001)
}

// --------------------------------------------------------------------------
// AC-2/AC-5: unpriced-model signal reaches the RPC response
// --------------------------------------------------------------------------

func TestGetInsightsSummary_WhenUnpricedModelFamily_ExpectPricingUnavailableFlagged(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "gpt-99-turbo", "/proj", 1000, 500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Models, 1)
	assert.Equal(t, "gpt-99-turbo", resp.Msg.Models[0].ModelFamily)
	assert.True(t, resp.Msg.Models[0].PricingUnavailable)
	assert.Contains(t, resp.Msg.UnpricedModels, "gpt-99-turbo")
}

// --------------------------------------------------------------------------
// AC-1: claude-sonnet-5 pricing table entry is reachable end-to-end through
// the GetInsightsSummary RPC path (not just verified in isolation by
// session/tokens/pricing_test.go).
// --------------------------------------------------------------------------

func TestGetInsightsSummary_WhenSonnet5ModelUsed_ExpectNonZeroCost(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "claude-sonnet-5-20250929", "/proj", 1000, 500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Models, 1)
	assert.Equal(t, "claude-sonnet-5", resp.Msg.Models[0].ModelFamily)
	assert.False(t, resp.Msg.Models[0].PricingUnavailable)
	assert.NotContains(t, resp.Msg.UnpricedModels, "claude-sonnet-5")
	assert.Greater(t, resp.Msg.TotalCostUsd, float64(0))
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Greater(t, resp.Msg.Sessions[0].EstimatedCostUsd, float64(0))
}

func TestListSessionTokens_WhenUnpricedModelFamily_ExpectUnpricedModelsPopulated(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "gpt-99-turbo", "/proj", 1000, 500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.ListSessionTokens(
		context.Background(),
		connect.NewRequest(&sessionv1.ListSessionTokensRequest{}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, []string{"gpt-99-turbo"}, resp.Msg.Sessions[0].UnpricedModels)
}

// --------------------------------------------------------------------------
// Pre-mortem Failure #2 (P2): <synthetic> must never leak into the unpriced
// signal end-to-end, even though Epic 1.2's parser-boundary filter already
// keeps it out of TurnTimeline in production. This test constructs the
// ParseResult by hand (bypassing the parser) to prove the service layer
// itself has no separate leak path — see pre-mortem.md row 2.
// --------------------------------------------------------------------------

func TestGetInsightsSummary_WhenSyntheticTurnMixedWithRealTurns_ExpectSyntheticNeverSurfacedAsUnpriced(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		{
			SessionUUID: "uuid-mixed",
			ProjectPath: "/proj",
			FileModTime: now,
			TurnTimeline: []tokens.TurnStats{
				// Synthetic turn: zero usage across every counter, matching the
				// real empirical shape Claude Code's transcript writer emits.
				{Model: "<synthetic>", Timestamp: now},
				{Model: "claude-sonnet-4", Input: 1000, Output: 500, Timestamp: now},
				{Model: "gpt-99-turbo", Input: 700, Output: 300, Timestamp: now},
			},
			TotalInput:  1700,
			TotalOutput: 800,
			ToolUsage:   map[string]tokens.ToolTokenStats{},
		},
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)

	assert.Contains(t, resp.Msg.UnpricedModels, "gpt-99-turbo")
	assert.NotContains(t, resp.Msg.UnpricedModels, "<synthetic>")

	for _, mb := range resp.Msg.Models {
		assert.NotEqual(t, "<synthetic>", mb.ModelFamily)
	}
}

// --------------------------------------------------------------------------
// AC-5: runtime signal — a newly-observed unpriced family is logged once,
// not once per request. Asserting on actual log output is awkward, so this
// asserts on the observable dedup state instead (whitebox test in package
// services): len(loggedUnpricedFamilies) stays at 1 after two calls with the
// same unpriced family.
// --------------------------------------------------------------------------

func TestGetInsightsSummary_WhenCalledTwiceWithSameUnpricedFamily_ExpectLoggedOnce(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "gpt-99-turbo", "/proj", 1000, 500, 0, now),
	}
	svc := newInsightsFixture(results, nil)

	_, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)
	require.NoError(t, err)

	_, err = svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)
	require.NoError(t, err)

	assert.Len(t, svc.loggedUnpricedFamilies, 1)
	assert.True(t, svc.loggedUnpricedFamilies["gpt-99-turbo"])
}

// --------------------------------------------------------------------------
// AC-4: WatchInsights streaming test coverage, via the insightsEventSender
// seam (mirrors backlogItemEventSender/watchBacklogItems).
// --------------------------------------------------------------------------

// fakeInsightsEventSender is a hand-rolled fake implementing
// insightsEventSender, capturing every sent message in a mutex-guarded slice
// — watchInsights runs in a goroutine below while the test triggers a file
// change concurrently, so Send is called concurrently with Sent().
type fakeInsightsEventSender struct {
	mu   sync.Mutex
	sent []*sessionv1.InsightsEvent
}

func (f *fakeInsightsEventSender) Send(e *sessionv1.InsightsEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, e)
	return nil
}

// Sent returns a snapshot copy of the messages sent so far, safe to read
// concurrently with in-flight Send calls.
func (f *fakeInsightsEventSender) Sent() []*sessionv1.InsightsEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*sessionv1.InsightsEvent, len(f.sent))
	copy(out, f.sent)
	return out
}

var _ insightsEventSender = (*fakeInsightsEventSender)(nil)

// runWatchInsights launches watchInsights in a goroutine, mirroring
// runWatchBacklogItems (backlog_service_events_test.go). Cleanup/completion
// is checked via the package's existing requireCleanReturn helper.
func runWatchInsights(ctx context.Context, svc *InsightsService, sender insightsEventSender) <-chan error {
	done := make(chan error, 1)
	go func() { done <- svc.watchInsights(ctx, sender) }()
	return done
}

func TestWatchInsights_should_forwardUpdateEvent_When_TokenStoreNotifies(t *testing.T) {
	t.Parallel()
	store := tokens.NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store.Start(ctx)
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil)

	sender := &fakeInsightsEventSender{}
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	done := runWatchInsights(runCtx, svc, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	// walkAndEnqueue's deferred cleanup fires exactly one notify() even for
	// an empty historyDir (store.go's defer runs before the historyDir==""
	// early return check) — a real, unbounded race against Subscribe() above.
	// Snapshotting the count immediately before triggering the real file
	// change, and gating completion on the file having actually been parsed
	// (not just "some update event exists"), ties the assertion to the
	// causal chain this test claims to exercise rather than either notify.
	before := len(sender.Sent())
	store.OnHistoryFileChanged("../../session/tokens/testdata/valid_session.jsonl")

	require.Eventually(t, func() bool {
		return len(sender.Sent()) > before && store.GetByUUID("valid_session") != nil
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, "update", sender.Sent()[len(sender.Sent())-1].EventType)

	requireCleanReturn(t, runCancel, done)
}

// --------------------------------------------------------------------------
// AC-1: GetSessionTurnTimeline
// --------------------------------------------------------------------------

func TestGetSessionTurnTimeline_should_returnTurns_When_ConversationIdMatches(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []*tokens.ParseResult{
		newResult("uuid-1", "claude-sonnet-4", "/proj", 1000, 500, 200, now),
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetSessionTurnTimeline(
		context.Background(),
		connect.NewRequest(&sessionv1.GetSessionTurnTimelineRequest{ConversationId: "uuid-1"}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Turns, 1)
	assert.Equal(t, "claude-sonnet-4", resp.Msg.Turns[0].Model)
	assert.Equal(t, int64(1000), resp.Msg.Turns[0].InputTokens)
	assert.Equal(t, int64(500), resp.Msg.Turns[0].OutputTokens)
	assert.Equal(t, int64(200), resp.Msg.Turns[0].CacheReadTokens)
}

func TestGetSessionTurnTimeline_should_returnEmptyTurns_When_ConversationIdUnknown(t *testing.T) {
	t.Parallel()
	svc := newInsightsFixture(nil, nil)

	resp, err := svc.GetSessionTurnTimeline(
		context.Background(),
		connect.NewRequest(&sessionv1.GetSessionTurnTimelineRequest{ConversationId: "no-such-uuid"}),
	)
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Turns)
}

func TestGetSessionTurnTimeline_should_omitTimestamp_When_TurnTimestampIsZero(t *testing.T) {
	t.Parallel()
	results := []*tokens.ParseResult{
		{
			SessionUUID: "uuid-zero-ts",
			TurnTimeline: []tokens.TurnStats{
				{Model: "claude-sonnet-4", Input: 100, Output: 50}, // zero-value Timestamp
			},
			ToolUsage: map[string]tokens.ToolTokenStats{},
		},
	}
	svc := newInsightsFixture(results, nil)

	resp, err := svc.GetSessionTurnTimeline(
		context.Background(),
		connect.NewRequest(&sessionv1.GetSessionTurnTimelineRequest{ConversationId: "uuid-zero-ts"}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Turns, 1)
	assert.Nil(t, resp.Msg.Turns[0].Timestamp)
}

func TestGetSessionTurnTimeline_should_returnIndependentToolNamesSlice_When_CalledTwice(t *testing.T) {
	t.Parallel()
	results := []*tokens.ParseResult{
		{
			SessionUUID: "uuid-tools",
			TurnTimeline: []tokens.TurnStats{
				{Model: "claude-sonnet-4", Input: 100, Output: 50, ToolNames: []string{"bash", "read"}},
			},
			ToolUsage: map[string]tokens.ToolTokenStats{},
		},
	}
	svc := newInsightsFixture(results, nil)
	req := connect.NewRequest(&sessionv1.GetSessionTurnTimelineRequest{ConversationId: "uuid-tools"})

	first, err := svc.GetSessionTurnTimeline(context.Background(), req)
	require.NoError(t, err)
	first.Msg.Turns[0].ToolNames[0] = "mutated"

	second, err := svc.GetSessionTurnTimeline(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "bash", second.Msg.Turns[0].ToolNames[0], "mutating one response's ToolNames must not affect a later call's result")
}

func TestGetSessionTurnTimeline_should_returnTurns_When_backedByRealTokenStore(t *testing.T) {
	t.Parallel()
	store := tokens.NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store.Start(ctx)
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil)

	store.OnHistoryFileChanged("../../session/tokens/testdata/valid_session.jsonl")
	require.Eventually(t, func() bool {
		return store.GetByUUID("valid_session") != nil
	}, 2*time.Second, 10*time.Millisecond)

	resp, err := svc.GetSessionTurnTimeline(
		context.Background(),
		connect.NewRequest(&sessionv1.GetSessionTurnTimelineRequest{ConversationId: "valid_session"}),
	)
	require.NoError(t, err)
	require.Len(t, resp.Msg.Turns, 3)
	// Assert the first turn's fields, not just the count — proves the real
	// parse->handler mapping (not just newResult's synthetic fixture) wires
	// model/token fields correctly. Values per testdata/valid_session.jsonl's
	// first assistant turn (parser_test.go's own documented totals).
	assert.Equal(t, int64(1000), resp.Msg.Turns[0].InputTokens)
	assert.Equal(t, int64(500), resp.Msg.Turns[0].OutputTokens)
	assert.NotEmpty(t, resp.Msg.Turns[0].Model)
}

func TestWatchInsights_should_unsubscribeAndReturn_When_ContextIsCanceled(t *testing.T) {
	t.Parallel()
	store := tokens.NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store.Start(ctx)
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil)

	sender := &fakeInsightsEventSender{}
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	done := runWatchInsights(runCtx, svc, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	requireCleanReturn(t, runCancel, done)
}

// --------------------------------------------------------------------------
// Epic 1.5 Story 1.5.2: buildSessionSummary extraction parity
// --------------------------------------------------------------------------

// TestBuildSessionSummary_WhenCalledDirectly_ExpectProtoEqualToHandBuiltExpectedSummary
// exercises buildSessionSummary directly with a fixture engineered so every
// field it sets — including Epics 1.1/1.2's WasteScore, ActivityType, and
// per-tool cost fields — has a hand-derivable expected value, then asserts
// proto.Equal against a hand-built SessionTokenSummary. This is the refactor
// extraction's safety net (Task 1.5.2a/c): GetInsightsSummary and
// ListSessionTokens must produce byte-identical summaries via this one
// function.
func TestBuildSessionSummary_WhenCalledDirectly_ExpectProtoEqualToHandBuiltExpectedSummary(t *testing.T) {
	t.Parallel()

	const modelFamily = "custom-summary-model"
	pt := &tokens.PricingTable{
		Prices: map[string]tokens.ModelPricing{
			modelFamily: {
				ModelFamily:        modelFamily,
				InputPricePerMTok:  3.0,
				OutputPricePerMTok: 15.0,
				CacheWritePerMTok:  3.75,
				CacheReadPerMTok:   0.3,
			},
		},
	}

	base := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	turns := make([]tokens.TurnStats, 5)
	for i := range turns {
		turns[i] = tokens.TurnStats{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Model:     modelFamily,
			Input:     200_000,
			Output:    100_000,
			CacheRead: 50_000,
			ToolNames: []string{"Read"},
		}
	}

	result := &tokens.ParseResult{
		SessionUUID:   "conv-abc",
		ProjectPath:   "/home/user/proj",
		PrimaryModel:  modelFamily,
		TotalInput:    1_000_000,
		TotalOutput:   500_000,
		CacheCreation: 0,
		CacheRead:     250_000,
		MessageCount:  7,
		TurnTimeline:  turns,
		ToolUsage: map[string]tokens.ToolTokenStats{
			"Read": {ToolName: "Read", CallCount: 5},
		},
		SkillActivations: []tokens.SkillActivation{
			{Name: "code-review", TurnIndex: 0, IsCommand: false},
		},
		FileModTime: base.Add(10 * time.Minute),
	}

	sessionRecords := []tokens.SessionRecord{
		{SessionID: "sess-abc", ConversationID: "conv-abc", Path: "/home/user/proj"},
	}
	associator := tokens.NewAssociator(&fakeSessionStorage{records: sessionRecords})
	snapshot := associator.Snapshot()

	got := buildSessionSummary(result, pt, associator, snapshot)

	// Derivation (see Story 1.5.2's fixture comment above for full formulas):
	//  - EstimatedCostUsd: 1.0*3.0 + 0.5*15.0 + 0.25*0.3 = 10.575 (1M input,
	//    500K output, 250K cache-read, in units of 1M tokens).
	//  - CacheHitRate: 250,000 / (1,000,000 + 250,000) = 0.2.
	//  - Per-turn cost: 0.2*3.0 + 0.1*15.0 + 0.05*0.3 = 2.115; every turn's
	//    sole tool is "Read", so its attributed cost is 5*2.115 = 10.575,
	//    with no double-counting (one distinct tool per turn) and no
	//    unpriced turns.
	//  - ActivityType: ToolUsage is 100% "Read" (a read-tool) calls, so
	//    readRatio = 1.0 >= 0.6 -> ACTIVITY_TYPE_EXPLORATORY (the
	//    "code-review" skill name matches neither "debug" nor "refactor",
	//    so the tool-ratio fallback is what fires).
	//  - WasteScore: cacheShortfall = 0.95-0.2 = 0.75; totalTokens =
	//    1,750,000 -> ceilingPenalty = 1,750,000/2,000,000 = 0.875; first
	//    turn's context = 200,000+50,000 = 250,000 -> contextPenalty =
	//    clamp01(250,000/30,000) = 1. score = 0.75*40 + 0.875*30 + 1*30 =
	//    30 + 26.25 + 30 = 86.25.
	want := &sessionv1.SessionTokenSummary{
		SessionId:           "sess-abc",
		ConversationId:      "conv-abc",
		ProjectPath:         "/home/user/proj",
		PrimaryModel:        modelFamily,
		TotalInputTokens:    1_000_000,
		TotalOutputTokens:   500_000,
		CacheCreationTokens: 0,
		CacheReadTokens:     250_000,
		EstimatedCostUsd:    10.575,
		CacheHitRate:        0.2,
		MessageCount:        7,
		FirstMessageAt:      timestamppb.New(base),
		LastMessageAt:       timestamppb.New(base.Add(4 * time.Minute)),
		IsOrphan:            false,
		SkillActivations:    []string{"code-review"},
		TopTools: []*sessionv1.TopToolEntry{
			{
				ToolName:           "Read",
				CallCount:          5,
				CostUsd:            10.575,
				CostMayDoubleCount: false,
				CostUnpriced:       false,
			},
		},
		UnpricedModels: nil,
		WasteScore:     proto.Float64(86.25),
		ActivityType:   sessionv1.ActivityType_ACTIVITY_TYPE_EXPLORATORY,
		// CacheRoiUsd (Story 1.3.1c): cacheRead*(input-cacheRead)/1e6 -
		// cacheCreation*cacheWrite/1e6 = 250,000*(3.0-0.3)/1e6 - 0 = 0.675.
		CacheRoiUsd: 0.675,
	}

	require.Empty(t, got.UnpricedModels)
	got.UnpricedModels = nil // []string{} vs nil is not proto-meaningful; normalize before comparing.

	// EstimatedCostUsd is summed in one pass over model-family totals while
	// TopTools[0].CostUsd is summed turn-by-turn (AttributeToolCosts) — both
	// formulas are mathematically 10.575 but land on adjacent float64 values
	// (10.574999999999999 vs 10.575000000000001) due to summation order, so
	// compare the float fields with a tolerance and everything else via
	// proto.Equal on a copy with those exact fields zeroed out.
	assert.InDelta(t, 10.575, got.EstimatedCostUsd, 1e-9)
	require.Len(t, got.TopTools, 1)
	assert.InDelta(t, 10.575, got.TopTools[0].CostUsd, 1e-9)
	assert.InDelta(t, 0.675, got.CacheRoiUsd, 1e-9)

	gotForEqual := proto.Clone(got).(*sessionv1.SessionTokenSummary)
	gotForEqual.EstimatedCostUsd = 0
	gotForEqual.TopTools[0].CostUsd = 0
	gotForEqual.CacheRoiUsd = 0
	wantForEqual := proto.Clone(want).(*sessionv1.SessionTokenSummary)
	wantForEqual.EstimatedCostUsd = 0
	wantForEqual.TopTools[0].CostUsd = 0
	wantForEqual.CacheRoiUsd = 0
	assert.True(t, proto.Equal(wantForEqual, gotForEqual), "buildSessionSummary output diverged from hand-built expected summary:\n got:  %+v\n want: %+v", got, want)
}

// --------------------------------------------------------------------------
// Epic 1.5 Story 1.5.3: watchInsights populates InsightsEvent.Session
// --------------------------------------------------------------------------

func TestWatchInsights_WhenChannelReceivesNonNilParseResult_ExpectUpdateEventWithPopulatedSession(t *testing.T) {
	t.Parallel()
	// A test-controlled channel (rather than a real TokenStore's background
	// walk) gives deterministic control over exactly what the receive loop
	// sees — no racing a real walk's timing.
	ch := make(chan *tokens.ParseResult, 1)
	store := &fakeTokenStore{subscribeCh: ch}
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil)

	sender := &fakeInsightsEventSender{}
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	done := runWatchInsights(runCtx, svc, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	// notify(result) sends the non-nil *tokens.ParseResult for the reparsed
	// file (Story 1.5.1), so the "update" event built from it must carry a
	// populated Session, not the previous always-nil status quo.
	ch <- newResult("conv-xyz", "claude-sonnet-4", "/proj", 1000, 500, 200, time.Now().UTC())

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)

	last := sender.Sent()[len(sender.Sent())-1]
	assert.Equal(t, "update", last.EventType)
	require.NotNil(t, last.Session)
	assert.Equal(t, "conv-xyz", last.Session.ConversationId)

	requireCleanReturn(t, runCancel, done)
}

func TestWatchInsights_WhenChannelReceivesNil_ExpectParseCompleteEventNotBareUpdate(t *testing.T) {
	t.Parallel()
	ch := make(chan *tokens.ParseResult, 1)
	store := &fakeTokenStore{subscribeCh: ch}
	svc := NewInsightsService(store, tokens.DefaultPricingTable(), nil)

	sender := &fakeInsightsEventSender{}
	runCtx, runCancel := context.WithCancel(context.Background())
	t.Cleanup(runCancel)
	done := runWatchInsights(runCtx, svc, sender)

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 1 }, 2*time.Second, 10*time.Millisecond)

	// notify(nil) is what a completed full-walk sends (Story 1.5.1) — this
	// must produce a real "parse_complete", not another indistinguishable
	// "update" (today's bug: the frontend's fetchSummary()-triggering branch
	// only listens for "parse_complete", so it never re-fires after the
	// first stream message).
	ch <- nil

	require.Eventually(t, func() bool { return len(sender.Sent()) >= 2 }, 2*time.Second, 10*time.Millisecond)

	last := sender.Sent()[len(sender.Sent())-1]
	assert.Equal(t, "parse_complete", last.EventType)
	assert.True(t, last.AllParsed)
	assert.Nil(t, last.Session)

	requireCleanReturn(t, runCancel, done)
}

// --------------------------------------------------------------------------
// Epic 1.1 Story 1.1.5: findings wired into GetInsightsSummary, capped+sorted
// --------------------------------------------------------------------------

// findingsFixturePricingTable builds a PricingTable with one custom model per
// session index (1..n), each priced so that detectSessionTokenCeiling fires
// with an exact DollarImpact of float64(i) — see the test's derivation
// comment below. All other prices are zero so only the output-token term
// contributes.
func findingsFixturePricingTable(n int) *tokens.PricingTable {
	prices := make(map[string]tokens.ModelPricing, n)
	for i := 1; i <= n; i++ {
		model := fmt.Sprintf("custom-model-%d", i)
		prices[model] = tokens.ModelPricing{
			ModelFamily: model,
			// 2,000,000 output tokens (2 MTok) * (i/2.0 $/MTok) = i dollars exactly
			// (division and multiplication by 2 are exact in IEEE-754 double).
			OutputPricePerMTok: float64(i) / 2.0,
		}
	}
	return &tokens.PricingTable{Prices: prices}
}

// findingsFixtureResult builds a ParseResult that trips exactly the
// detectSessionTokenCeiling detector (and no other) with DollarImpact == i:
//   - 5 turns (>= minTurnsForCacheFloor) with a 98% cache-hit rate (>= the
//     95% floor, calibrated per validation.md's Threshold Calibration step)
//     so detectCacheHitFloorBreach abstains.
//   - a single distinct model, so detectModelSwitchCacheBust abstains (needs >= 2).
//   - each turn's Input+CacheRead is 10,000 (<= the 30,000 oversized-context
//     floor), so detectOversizedStartContext abstains.
//   - total tokens (1,000 input + 2,000,000 output + 49,000 cache-read) exceed
//     the 2,000,000 session-token ceiling, so detectSessionTokenCeiling fires.
func findingsFixtureResult(i int, now time.Time) *tokens.ParseResult {
	model := fmt.Sprintf("custom-model-%d", i)
	turns := make([]tokens.TurnStats, 5)
	for t := range turns {
		turns[t] = tokens.TurnStats{
			Model:     model,
			Input:     200,
			Output:    400_000,
			CacheRead: 9_800,
			Timestamp: now,
		}
	}
	return &tokens.ParseResult{
		SessionUUID:  fmt.Sprintf("uuid-%d", i),
		PrimaryModel: model,
		Models:       []string{model},
		ProjectPath:  "/proj",
		TotalInput:   1_000,
		TotalOutput:  2_000_000,
		CacheRead:    49_000,
		FileModTime:  now,
		TurnTimeline: turns,
		ToolUsage:    map[string]tokens.ToolTokenStats{},
	}
}

func TestGetInsightsSummary_When25SessionsEachProduceOneFinding_ExpectTop20SortedByDollarImpactDesc(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	const n = 25
	results := make([]*tokens.ParseResult, 0, n)
	for i := 1; i <= n; i++ {
		results = append(results, findingsFixtureResult(i, now))
	}

	store := &fakeTokenStore{results: results}
	pricing := findingsFixturePricingTable(n)
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Findings, 20)
	assert.Equal(t, float64(25), resp.Msg.Findings[0].DollarImpactUsd)
	assert.Equal(t, float64(6), resp.Msg.Findings[19].DollarImpactUsd)
	for i := 1; i < len(resp.Msg.Findings); i++ {
		assert.GreaterOrEqual(t, resp.Msg.Findings[i-1].DollarImpactUsd, resp.Msg.Findings[i].DollarImpactUsd)
	}
	assert.Equal(t, sessionv1.FindingType_FINDING_TYPE_SESSION_TOKEN_CEILING, resp.Msg.Findings[0].FindingType)
}

// TestGetInsightsSummary_WhenOneSessionHasDegenerateData_ExpectOtherSessionsFindingsStillReturned
// leans on Story 1.1.3c's panic-isolation inside tokens.ComputeFindings (unit-
// tested directly in findings_test.go against a panicking test double): a
// session with no turn timeline at all is the most degenerate ParseResult
// GetInsightsSummary's loop can hand to ComputeFindings, and this asserts the
// whole request still succeeds and still returns the other session's finding.
func TestGetInsightsSummary_WhenOneSessionHasDegenerateData_ExpectOtherSessionsFindingsStillReturned(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	degenerate := &tokens.ParseResult{SessionUUID: "uuid-degenerate"}
	normal := findingsFixtureResult(1, now)

	store := &fakeTokenStore{results: []*tokens.ParseResult{degenerate, normal}}
	pricing := findingsFixturePricingTable(1)
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Findings, 1)
	assert.Equal(t, float64(1), resp.Msg.Findings[0].DollarImpactUsd)
}

// --------------------------------------------------------------------------
// Epic 1.2: Per-tool/activity cost breakdown
// --------------------------------------------------------------------------

// unitCostPricingTableForServiceTests prices "unit-model" at $1,000,000/MTok
// input, so InputTokens count reads directly as USD cost — keeps fixture turn
// costs at clean, readable dollar figures across these tests.
func unitCostPricingTableForServiceTests() *tokens.PricingTable {
	return &tokens.PricingTable{
		Prices: map[string]tokens.ModelPricing{
			"unit-model": {ModelFamily: "unit-model", InputPricePerMTok: 1_000_000},
		},
	}
}

// TestGetInsightsSummary_WhenMostCalledToolUnpricedAndLessCalledToolPriced_ExpectOrderingUnchangedAndCostFieldsPopulated
// is the literal Story 1.2.2 AC example: Read (50 calls, unpriced model) must
// still sort first (call-count-desc contract unchanged), Bash (5 calls, $5.00
// attributed) gets CostUsd == 5.00, and Read gets CostUnpriced == true (never
// CostUsd == 0 masquerading as "free").
func TestGetInsightsSummary_WhenMostCalledToolUnpricedAndLessCalledToolPriced_ExpectOrderingUnchangedAndCostFieldsPopulated(t *testing.T) {
	t.Parallel()
	pricing := unitCostPricingTableForServiceTests()

	result := &tokens.ParseResult{
		SessionUUID: "uuid-tool-cost",
		TurnTimeline: []tokens.TurnStats{
			{Model: "no-such-model", Input: 1, ToolNames: []string{"Read"}},
			{Model: "unit-model", Input: 5, ToolNames: []string{"Bash"}},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{
			"Read": {ToolName: "Read", CallCount: 50},
			"Bash": {ToolName: "Bash", CallCount: 5},
		},
	}

	store := &fakeTokenStore{results: []*tokens.ParseResult{result}}
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	topTools := resp.Msg.Sessions[0].TopTools
	require.Len(t, topTools, 2)

	// Call-count-desc ordering unchanged: Read (50) still sorts before Bash (5).
	assert.Equal(t, "Read", topTools[0].ToolName)
	assert.Equal(t, "Bash", topTools[1].ToolName)

	assert.True(t, topTools[0].CostUnpriced)
	assert.Equal(t, float64(0), topTools[0].CostUsd)
	assert.False(t, topTools[0].CostMayDoubleCount)

	assert.False(t, topTools[1].CostUnpriced)
	assert.Equal(t, float64(5), topTools[1].CostUsd)
	assert.False(t, topTools[1].CostMayDoubleCount)
}

func TestGetInsightsSummary_WhenMultiToolTurn_ExpectCostMayDoubleCountSetOnBothTools(t *testing.T) {
	t.Parallel()
	pricing := unitCostPricingTableForServiceTests()

	result := &tokens.ParseResult{
		SessionUUID: "uuid-double-count",
		TurnTimeline: []tokens.TurnStats{
			{Model: "unit-model", Input: 2, ToolNames: []string{"Read", "Grep"}},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{
			"Read": {ToolName: "Read", CallCount: 1},
			"Grep": {ToolName: "Grep", CallCount: 1},
		},
	}

	store := &fakeTokenStore{results: []*tokens.ParseResult{result}}
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	topTools := resp.Msg.Sessions[0].TopTools
	require.Len(t, topTools, 2)
	for _, tt := range topTools {
		assert.True(t, tt.CostMayDoubleCount, "tool %s should be flagged double-counted", tt.ToolName)
		assert.Equal(t, float64(2), tt.CostUsd)
		assert.False(t, tt.CostUnpriced)
	}
}

// TestGetInsightsSummary_When3SessionsAcross2ActivityTypes_ExpectActivityBreakdownAggregatedCorrectly
// is the literal Story 1.2.4 AC example: 2 sessions classified feature_dev
// ($3.00, $4.00) and 1 classified debugging ($2.00) must aggregate into
// exactly those two ActivityCostBreakdown entries, cost-desc sorted.
func TestGetInsightsSummary_When3SessionsAcross2ActivityTypes_ExpectActivityBreakdownAggregatedCorrectly(t *testing.T) {
	t.Parallel()
	pricing := unitCostPricingTableForServiceTests()

	featureDevToolUsage := map[string]tokens.ToolTokenStats{
		"Edit": {ToolName: "Edit", CallCount: 5},
	}
	featureDevSessionA := &tokens.ParseResult{
		SessionUUID: "uuid-feature-dev-a",
		TurnTimeline: []tokens.TurnStats{
			{Model: "unit-model", Input: 3},
		},
		ToolUsage: featureDevToolUsage,
	}
	featureDevSessionB := &tokens.ParseResult{
		SessionUUID: "uuid-feature-dev-b",
		TurnTimeline: []tokens.TurnStats{
			{Model: "unit-model", Input: 4},
		},
		ToolUsage: featureDevToolUsage,
	}
	debuggingSession := &tokens.ParseResult{
		SessionUUID:      "uuid-debugging",
		SkillActivations: []tokens.SkillActivation{{Name: "code-debugging"}},
		TurnTimeline: []tokens.TurnStats{
			{Model: "unit-model", Input: 2},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{},
	}

	store := &fakeTokenStore{results: []*tokens.ParseResult{featureDevSessionA, featureDevSessionB, debuggingSession}}
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.ActivityBreakdown, 2)

	// Cost-desc sorted: feature_dev ($7.00) before debugging ($2.00).
	assert.Equal(t, sessionv1.ActivityType_ACTIVITY_TYPE_FEATURE_DEV, resp.Msg.ActivityBreakdown[0].ActivityType)
	assert.Equal(t, float64(7), resp.Msg.ActivityBreakdown[0].EstimatedCostUsd)
	assert.Equal(t, int32(2), resp.Msg.ActivityBreakdown[0].SessionCount)

	assert.Equal(t, sessionv1.ActivityType_ACTIVITY_TYPE_DEBUGGING, resp.Msg.ActivityBreakdown[1].ActivityType)
	assert.Equal(t, float64(2), resp.Msg.ActivityBreakdown[1].EstimatedCostUsd)
	assert.Equal(t, int32(1), resp.Msg.ActivityBreakdown[1].SessionCount)
}

// TestGetInsightsSummary_WhenSessionClassified_ExpectActivityTypeSetOnSummary
// covers Task 1.2.3c's direct enum assignment onto SessionTokenSummary.
func TestGetInsightsSummary_WhenSessionClassified_ExpectActivityTypeSetOnSummary(t *testing.T) {
	t.Parallel()
	pricing := unitCostPricingTableForServiceTests()

	result := &tokens.ParseResult{
		SessionUUID:      "uuid-activity-type",
		SkillActivations: []tokens.SkillActivation{{Name: "code-debugging"}},
		TurnTimeline: []tokens.TurnStats{
			{Model: "unit-model", Input: 1},
		},
		ToolUsage: map[string]tokens.ToolTokenStats{},
	}

	store := &fakeTokenStore{results: []*tokens.ParseResult{result}}
	svc := NewInsightsService(store, pricing, nil)

	resp, err := svc.GetInsightsSummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}),
	)

	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, sessionv1.ActivityType_ACTIVITY_TYPE_DEBUGGING, resp.Msg.Sessions[0].ActivityType)
}
