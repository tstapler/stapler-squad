package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --------------------------------------------------------------------------
// Fake TokenStore (minimal in-memory implementation for tests)
// Implements tokens.TokenStoreReader.
// --------------------------------------------------------------------------

type fakeTokenStore struct {
	results   []*tokens.ParseResult
	isLoading bool
}

func (f *fakeTokenStore) GetAll() []*tokens.ParseResult { return f.results }
func (f *fakeTokenStore) IsLoading() bool               { return f.isLoading }
func (f *fakeTokenStore) Subscribe() <-chan struct{}     { return make(chan struct{}, 1) }
func (f *fakeTokenStore) Unsubscribe(_ <-chan struct{})  {}

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
		SessionUUID: uuid,
		PrimaryModel: model,
		ProjectPath: projectPath,
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
// TC-GO-35: GetInsightsSummary daily rollup bucketing
// --------------------------------------------------------------------------

func TestGetInsightsSummary_DailyRollup_BucketsPerCalendarDay(t *testing.T) {
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
