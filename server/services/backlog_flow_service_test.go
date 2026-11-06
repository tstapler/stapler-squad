package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/flow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeFlowAgg struct {
	summary  *flow.FlowSummary
	compare  *flow.CompareResult
	drill    *flow.DrilldownPage
	timeline *flow.ItemTimeline
	err      error

	gotSummary  flow.FlowQuery
	gotCompare  flow.CompareQuery
	gotDrill    flow.DrilldownQuery
	gotTimeline flow.ItemTimelineQuery
}

func (f *fakeFlowAgg) Summary(_ context.Context, q flow.FlowQuery) (*flow.FlowSummary, error) {
	f.gotSummary = q
	return f.summary, f.err
}

func (f *fakeFlowAgg) Compare(_ context.Context, q flow.CompareQuery) (*flow.CompareResult, error) {
	f.gotCompare = q
	return f.compare, f.err
}

func (f *fakeFlowAgg) Drilldown(_ context.Context, q flow.DrilldownQuery) (*flow.DrilldownPage, error) {
	f.gotDrill = q
	return f.drill, f.err
}

func (f *fakeFlowAgg) ItemTimeline(_ context.Context, q flow.ItemTimelineQuery) (*flow.ItemTimeline, error) {
	f.gotTimeline = q
	return f.timeline, f.err
}

var flowT0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func sampleFlowSummary() *flow.FlowSummary {
	return &flow.FlowSummary{
		Meta: flow.FlowMeta{
			AsOf: flowT0, WindowStart: flowT0.AddDate(0, 0, -30), WindowEnd: flowT0, QueryMs: 12,
			RowCounts: flow.RowCounts{Events: 5, Verdicts: 3, StuckIntervals: 2, Items: 4},
			Estimated: []string{flow.EstimatedDeadlockHeuristic, "pr_fix_cause:unavailable"},
		},
		Tiles: flow.TilesSection{
			Merged:         flow.CountMetric{Value: 7, Drill: flow.DrillMerged},
			LeadTime:       flow.DurationMetric{Stats: flow.Percentiles{N: 7, P50: 90 * time.Minute, P90: 3 * time.Hour, Max: 5 * time.Hour}, Drill: flow.DrillLeadTime},
			FirstPassYield: flow.RatioMetric{Num: 3, Den: 7, Ratio: 3.0 / 7.0, LowSample: true, Drill: flow.DrillFPY},
		},
		Causes: flow.CausesSection{
			Rework:        []flow.CauseRow{{Cause: "review_fail", Count: flow.CountMetric{Value: 2, Drill: flow.DrillReworkReviewFail}}},
			PRFixTriggers: []flow.TriggerRow{{Trigger: "mixed", Count: 1}, {Trigger: "unknown", Count: 3}},
		},
		Review: flow.ReviewSection{
			Meta: flow.SectionMeta{Status: flow.SectionPartial, Error: "verdicts: boom"},
			Deadlocks: flow.ListPage[flow.DeadlockRow]{
				Items:      []flow.DeadlockRow{{ItemID: "i1", Title: "bad\ttitle\n" + strings.Repeat("x", 400), RunLength: 3, LastVerdictAt: flowT0}},
				TotalCount: 41, ReturnedCount: 1, HasMore: true, Truncated: true,
			},
		},
		Anomalies: flow.AnomaliesSection{
			ThrashItems: flow.CountMetric{Value: 1, Drill: flow.DrillThrashEpisode},
			Items:       flow.ListPage[flow.ThrashItemRow]{Items: []flow.ThrashItemRow{{ItemID: "i2", Title: "t", Episodes: 2, Events: 9, FirstAt: flowT0, LastAt: flowT0}}, TotalCount: 1, ReturnedCount: 1},
		},
		Trends: flow.TrendsSection{
			Throughput: []flow.ThroughputBucket{{WeekStart: flowT0, N: 4}}, ThroughputTotal: 4,
		},
	}
}

func callSummary(t *testing.T, svc *BacklogFlowService, req *sessionv1.GetFlowSummaryRequest) (*sessionv1.FlowSummary, error) {
	t.Helper()
	resp, err := svc.GetFlowSummary(context.Background(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Summary, nil
}

func TestBacklogFlowService_should_MapSummaryFieldsAndQuery_When_Days30(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	svc := NewBacklogFlowService(fake)

	got, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: 30, Sections: []string{"tiles", "review"}, Compact: true, ListLimit: 5})
	require.NoError(t, err)

	assert.Equal(t, flow.FlowQuery{
		Window: flow.WindowSpec{Days: 30}, Sections: []flow.SectionName{"tiles", "review"}, Compact: true, ListLimit: 5,
	}, fake.gotSummary)
	assert.EqualValues(t, 7, got.Tiles.Merged.Value)
	assert.Equal(t, "tiles.merged", got.Tiles.Merged.DrillKey)
	assert.EqualValues(t, 90*60*1000, got.Tiles.LeadTime.P50Ms)
	assert.EqualValues(t, 5*3600*1000, got.Tiles.LeadTime.MaxMs)
	assert.Equal(t, "tiles.lead_time", got.Tiles.LeadTime.DrillKey)
	assert.True(t, got.Tiles.FirstPassYield.LowSample)
	assert.InDelta(t, 3.0/7.0, got.Tiles.FirstPassYield.Ratio, 1e-9)
	assert.EqualValues(t, 12, got.Meta.QueryMs)
	assert.Equal(t, flowT0, got.Meta.AsOf.AsTime())
	assert.Nil(t, got.Meta.CaptureSince, "zero capture time must be unset")
	assert.EqualValues(t, 4, got.Meta.RowCounts.Items)
	assert.Equal(t, "partial", got.Review.Meta.Status)
	assert.Equal(t, "verdicts: boom", got.Review.Meta.Error)
	assert.EqualValues(t, 41, got.Review.Deadlocks.TotalCount)
	assert.True(t, got.Review.Deadlocks.HasMore)
	assert.True(t, got.Review.Deadlocks.Truncated)
	assert.EqualValues(t, 4, got.Trends.Throughput[0].N)
}

func TestBacklogFlowService_should_EmitLegendForEveryEstimatedCode(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	got, err := callSummary(t, NewBacklogFlowService(fake), &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
	require.NoError(t, err)
	for _, code := range got.Meta.Estimated {
		assert.NotEmpty(t, got.Meta.EstimatedLegend[code], code)
	}
	assert.Len(t, got.Meta.EstimatedLegend, len(got.Meta.Estimated))
}

func TestBacklogFlowService_should_HaveLegendEntry_When_CodeInEstimatedVocabulary(t *testing.T) {
	t.Parallel()
	vocabulary := []string{flow.EstimatedDeadlockHeuristic, "pr_fix_cause:unavailable", "pr_fix_dwell:combined", "stuck_history:lower_bound"}
	for _, code := range vocabulary {
		text, ok := flowEstimatedLegend[code]
		assert.True(t, ok, code)
		assert.NotEqual(t, flowUnknownLegend, text, code)
	}
	assert.Len(t, flowEstimatedLegend, len(vocabulary), "legend has an entry outside the vocabulary")
}

func TestBacklogFlowService_should_SanitizeTitles(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	got, err := callSummary(t, NewBacklogFlowService(fake), &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
	require.NoError(t, err)
	title := got.Review.Deadlocks.Items[0].Title
	assert.True(t, strings.HasPrefix(title, "bad title xxx"), title)
	assert.LessOrEqual(t, len([]rune(title)), flowTitleMaxRunes)
	assert.NotContains(t, title, "\t")
}

func TestBacklogFlowService_should_AcceptMixedPRFixTrigger(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	got, err := callSummary(t, NewBacklogFlowService(fake), &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
	require.NoError(t, err)
	triggers := make([]string, 0, len(got.Causes.PrFixTriggers))
	for _, r := range got.Causes.PrFixTriggers {
		triggers = append(triggers, r.Trigger)
		assert.Equal(t, "rework.pr_fix", r.DrillKey)
	}
	assert.Equal(t, []string{"mixed", "unknown"}, triggers)
}

func TestBacklogFlowService_should_MapAllTimeAndRejectZeroDays(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	svc := NewBacklogFlowService(fake)

	_, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{AllTime: true, WindowDays: 99})
	require.NoError(t, err)
	assert.Equal(t, flow.WindowSpec{AllTime: true}, fake.gotSummary.Window)

	for _, days := range []int32{0, -3} {
		_, err = callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: days})
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Contains(t, err.Error(), "use all_time=true for all-time")
	}
}

func TestBacklogFlowService_should_ReturnFailedPrecondition_When_AggregatorNil(t *testing.T) {
	t.Parallel()
	var typedNil *flow.Aggregator
	for name, svc := range map[string]*BacklogFlowService{
		"nil interface": NewBacklogFlowService(nil),
		"typed nil":     NewBacklogFlowService(typedNil),
		"nil storage":   NewFlowServiceFromStorage(nil),
	} {
		_, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
		require.Error(t, err, name)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), name)
	}
}

func TestBacklogFlowService_should_MapAggregatorErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  error
		want connect.Code
	}{
		"compare all time": {flow.ErrCompareAllTime, connect.CodeInvalidArgument},
		"bad section":      {errors.New(`unknown section "x"; valid sections: tiles`), connect.CodeInvalidArgument},
		"bad compare":      {errors.New("compare: give a_days and b_days"), connect.CodeInvalidArgument},
		"bad window":       {errors.New("flow window: start must be before end"), connect.CodeInvalidArgument},
		"busy":             {flow.ErrBusy, connect.CodeResourceExhausted},
		"canceled":         {context.Canceled, connect.CodeCanceled},
		"other":            {errors.New("db exploded"), connect.CodeInternal},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc := NewBacklogFlowService(&fakeFlowAgg{err: tc.err})
			_, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
			assert.Equal(t, tc.want, connect.CodeOf(err))
		})
	}
}

func TestBacklogFlowService_should_CompareByOffsetsAndBounds(t *testing.T) {
	t.Parallel()
	d := 1.5
	fake := &fakeFlowAgg{compare: &flow.CompareResult{
		Meta: flow.FlowMeta{AsOf: flowT0},
		A:    sampleFlowSummary(),
		Metrics: []flow.CompareMetric{
			{Name: "merged", Unit: "count", State: flow.CompareOK, A: 3, B: 4.5, Delta: &d, NA: 3, NB: 4, Drill: flow.DrillMerged},
			{Name: "fpy", State: flow.CompareIncomparable, Reason: "low sample"},
		},
	}}
	svc := NewBacklogFlowService(fake)

	resp, err := svc.CompareFlowWindows(context.Background(), connect.NewRequest(&sessionv1.CompareFlowWindowsRequest{ADays: 30, BDays: 30, BOffsetDays: 30}))
	require.NoError(t, err)
	assert.Equal(t, flow.CompareQuery{ADays: 30, BDays: 30, BOffsetDays: 30}, fake.gotCompare)
	require.Len(t, resp.Msg.Metrics, 2)
	require.NotNil(t, resp.Msg.Metrics[0].Delta)
	assert.InDelta(t, 1.5, *resp.Msg.Metrics[0].Delta, 1e-9)
	assert.Equal(t, "tiles.merged", resp.Msg.Metrics[0].DrillKey)
	assert.Nil(t, resp.Msg.Metrics[1].Delta)
	assert.Equal(t, "incomparable", resp.Msg.Metrics[1].State)
	assert.NotNil(t, resp.Msg.A)
	assert.Nil(t, resp.Msg.B)

	boundary := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	_, err = svc.CompareFlowWindows(context.Background(), connect.NewRequest(&sessionv1.CompareFlowWindowsRequest{
		AStart: timestamppb.New(boundary.AddDate(0, 0, -14)), AEnd: timestamppb.New(boundary),
		BStart: timestamppb.New(boundary), BEnd: timestamppb.New(boundary.AddDate(0, 0, 14)),
	}))
	require.NoError(t, err)
	assert.Equal(t, boundary, fake.gotCompare.AEnd, "a_end passes through unchanged (exclusive)")
	assert.Equal(t, boundary, fake.gotCompare.BStart)
	assert.Zero(t, fake.gotCompare.ADays)
}

func TestBacklogFlowService_should_PassThroughUnknownDrillKeyMessage(t *testing.T) {
	t.Parallel()
	msg := `unknown drill key "rework.reviewfail"; valid keys: fpy.first_pass, rework.review_fail`
	svc := NewBacklogFlowService(&fakeFlowAgg{err: errors.New(msg)})
	_, err := svc.ListFlowDrilldown(context.Background(), connect.NewRequest(&sessionv1.ListFlowDrilldownRequest{Key: "rework.reviewfail", WindowDays: 30}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, msg, ce.Message())
}

func TestBacklogFlowService_should_MapDrilldownAndTimeline(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{
		drill: &flow.DrilldownPage{
			Key: flow.DrillReworkReviewFail, Meta: flow.FlowMeta{AsOf: flowT0},
			Items: flow.ListPage[flow.DrillItem]{
				Items:      []flow.DrillItem{{ItemID: "i1", Title: "a\nb", Status: "done", Reason: "r", Duration: 2 * time.Second}},
				TotalCount: 120, ReturnedCount: 50, HasMore: true, Truncated: true,
			},
		},
		timeline: &flow.ItemTimeline{
			ItemID: "i1", Title: "t", Status: "done", TotalEntries: 60, Truncated: true, ThrashEpisodes: 2,
			Entries: []flow.TimelineEntry{{Kind: "thrash_episode", From: "a", To: "b", At: flowT0, Last: flowT0.Add(time.Hour), Count: 9, Note: "x\ty"}},
		},
	}
	svc := NewBacklogFlowService(fake)

	dr, err := svc.ListFlowDrilldown(context.Background(), connect.NewRequest(&sessionv1.ListFlowDrilldownRequest{Key: "rework.review_fail", WindowDays: 30, Limit: 200, Offset: 10}))
	require.NoError(t, err)
	assert.Equal(t, flow.DrilldownQuery{Key: flow.DrillReworkReviewFail, Query: flow.FlowQuery{Window: flow.WindowSpec{Days: 30}}, Limit: 200, Offset: 10}, fake.gotDrill)
	assert.Equal(t, "a b", dr.Msg.Items.Items[0].Title)
	assert.EqualValues(t, 2000, dr.Msg.Items.Items[0].DurationMs)
	assert.EqualValues(t, 120, dr.Msg.Items.TotalCount)
	assert.True(t, dr.Msg.Items.Truncated)

	tl, err := svc.GetItemFlowTimeline(context.Background(), connect.NewRequest(&sessionv1.GetItemFlowTimelineRequest{ItemId: "i1", WindowDays: 30}))
	require.NoError(t, err)
	assert.Equal(t, flow.ItemTimelineQuery{ItemID: "i1", Window: flow.WindowSpec{Days: 30}}, fake.gotTimeline)
	assert.EqualValues(t, 9, tl.Msg.Entries[0].Events)
	assert.Equal(t, "x y", tl.Msg.Entries[0].Note)
	assert.EqualValues(t, 2, tl.Msg.ThrashEpisodes)

	_, err = svc.GetItemFlowTimeline(context.Background(), connect.NewRequest(&sessionv1.GetItemFlowTimelineRequest{WindowDays: 30}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestBacklogFlowService_should_SelectSectionLists(t *testing.T) {
	t.Parallel()
	fake := &fakeFlowAgg{summary: sampleFlowSummary()}
	fake.summary.Review.WeakFilter = "heuristic banner"
	svc := NewBacklogFlowService(fake)

	dl, err := svc.ListDeadlockCandidates(context.Background(), connect.NewRequest(&sessionv1.ListDeadlockCandidatesRequest{WindowDays: 30, Limit: 7}))
	require.NoError(t, err)
	assert.Equal(t, []flow.SectionName{flow.SectionReview}, fake.gotSummary.Sections)
	assert.Equal(t, 7, fake.gotSummary.ListLimit)
	assert.EqualValues(t, 41, dl.Msg.Deadlocks.TotalCount)
	assert.Equal(t, "heuristic banner", dl.Msg.WeakFilter, "the list carries the same weak-filter banner as the review section")

	th, err := svc.ListThrashEpisodes(context.Background(), connect.NewRequest(&sessionv1.ListThrashEpisodesRequest{AllTime: true}))
	require.NoError(t, err)
	assert.Equal(t, []flow.SectionName{flow.SectionAnomalies}, fake.gotSummary.Sections)
	assert.EqualValues(t, 9, th.Msg.Items.Items[0].Events)
}

func TestBacklogFlowService_should_MapWindowTooLargeToResourceExhaustedWithCounts(t *testing.T) {
	t.Parallel()
	svc := NewBacklogFlowService(&fakeFlowAgg{err: fmt.Errorf("summary: %w", &flow.WindowTooLargeError{Rows: 123456, Limit: 100000})})
	_, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{AllTime: true})
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "123456")
	assert.Contains(t, err.Error(), "100000")
}

func TestBacklogFlowService_should_EmitSnakeCaseJSONWithDrillKeys(t *testing.T) {
	t.Parallel()
	got, err := callSummary(t, NewBacklogFlowService(&fakeFlowAgg{summary: sampleFlowSummary()}), &sessionv1.GetFlowSummaryRequest{WindowDays: 30})
	require.NoError(t, err)
	b, err := FlowJSONOptions().Marshal(got)
	require.NoError(t, err)
	js := string(b)
	for _, want := range []string{`"as_of"`, `"row_counts"`, `"query_ms"`, `"estimated_legend"`, `"drill_key": "tiles.merged"`} {
		assert.Contains(t, strings.ReplaceAll(js, `":"`, `": "`), want)
	}
	assert.NotContains(t, js, "asOf")
}

func TestBacklogFlowService_should_HaveAPIMarkerOnEveryHandler(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("backlog_flow_service.go")
	require.NoError(t, err)
	src := string(raw)
	for _, action := range []string{"get_summary", "list_deadlock_candidates", "list_thrash_episodes", "get_item_timeline", "compare_windows", "list_drilldown"} {
		assert.Contains(t, src, fmt.Sprintf("// +api: backlog_flow:%s\n", action))
	}
}

func TestBacklogFlowService_should_ServeEmptySummary_When_BuiltFromStorage(t *testing.T) {
	t.Parallel()
	repo, err := session.NewEntRepository(session.WithDatabasePath(":memory:"))
	require.NoError(t, err)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	defer storage.Close()

	svc := NewFlowServiceFromStorage(storage)
	got, err := callSummary(t, svc, &sessionv1.GetFlowSummaryRequest{WindowDays: 7, Sections: []string{"tiles"}})
	require.NoError(t, err)
	assert.Equal(t, "empty", got.Tiles.Meta.Status)
	assert.Equal(t, "skipped", got.Funnel.Meta.Status)
	assert.EqualValues(t, 0, got.Tiles.Merged.Value)
}

func TestBacklogFlowService_should_MapStuckNotesAndLegendDriftCode_When_EpisodesDrift(t *testing.T) {
	t.Parallel()
	sum := sampleFlowSummary()
	sum.Meta.Estimated = append(sum.Meta.Estimated, "stuck_episode_drift:1")
	sum.Stuck.Notes = []string{"open episodes differ from open stuck rows by 1"}
	got, err := callSummary(t, NewBacklogFlowService(&fakeFlowAgg{summary: sum}), &sessionv1.GetFlowSummaryRequest{WindowDays: 7})
	require.NoError(t, err)

	assert.Equal(t, []string{"open episodes differ from open stuck rows by 1"}, got.Stuck.Notes)
	assert.NotEqual(t, flowUnknownLegend, got.Meta.EstimatedLegend["stuck_episode_drift:1"])
	assert.NotEmpty(t, got.Meta.EstimatedLegend["stuck_episode_drift:1"])
}
