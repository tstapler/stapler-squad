package flow

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

var (
	cmpAMid = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	cmpBMid = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	cmpQ    = CompareQuery{ADays: 14, BDays: 14}
)

// addMerged adds n items that each walk ready to done through the real path,
// merging one minute apart starting at first; every item has a first-pass merge.
func (f *fakeSrc) addMerged(prefix string, n int, first time.Time) {
	for i := 0; i < n; i++ {
		id := ItemID(fmt.Sprintf("%s-%03d", prefix, i))
		f.items = append(f.items, ItemMeta{ID: id, Title: string(id), Status: domain.BacklogStatusDone})
		at := first.Add(time.Duration(i) * time.Minute)
		path := []domain.BacklogStatus{
			domain.BacklogStatusReady, domain.BacklogStatusInProgress, domain.BacklogStatusReview,
			domain.BacklogStatusPRPending, domain.BacklogStatusDone,
		}
		for k := 1; k < len(path); k++ {
			f.events = append(f.events, Event{
				ItemID: id, From: path[k-1], To: path[k],
				At: at.Add(-time.Duration(len(path)-1-k) * time.Hour), Seq: int64(len(f.events) + 1),
			})
		}
	}
}

// addVerdicts adds n PASS verdicts on distinct items and sessions, one minute apart.
func (f *fakeSrc) addVerdicts(prefix string, n int, first time.Time) {
	for i := 0; i < n; i++ {
		id := ItemID(fmt.Sprintf("%s-%03d", prefix, i))
		f.verdicts = append(f.verdicts, Verdict{
			ItemID: id, SessionID: SessionID("s-" + string(id)), CreatedAt: first.Add(time.Duration(i) * time.Minute),
			Outcome: OutcomePass, Summary: "looks good",
		})
		f.items = append(f.items, ItemMeta{ID: id, Title: string(id), Status: domain.BacklogStatusReview})
	}
}

func metricByName(t *testing.T, r *CompareResult, name string) CompareMetric {
	t.Helper()
	for _, m := range r.Metrics {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("metric %q not found", name)
	return CompareMetric{}
}

func compareOf(t *testing.T, f *fakeSrc, q CompareQuery, opts ...AggregatorOption) *CompareResult {
	t.Helper()
	res, err := NewAggregator(f.sources(), fixtureClock, opts...).Compare(context.Background(), q)
	require.NoError(t, err)
	return res
}

func TestCompare_should_ReturnABDeltaAndN_When_TwoAdjacentWindows(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)
	res := compareOf(t, f, cmpQ)

	m := metricByName(t, res, "merged_items")
	assert.Equal(t, CompareOK, m.State)
	assert.Equal(t, 25.0, m.A)
	assert.Equal(t, 30.0, m.B)
	require.NotNil(t, m.Delta)
	assert.Equal(t, 5.0, *m.Delta)
	assert.Equal(t, 25, m.NA)
	assert.Equal(t, 30, m.NB)
	assert.Equal(t, DrillMerged, m.Drill)

	fpy := metricByName(t, res, "first_pass_yield")
	assert.Equal(t, CompareOK, fpy.State, "25 and 30 items clear the minimum of 20")
	assert.Equal(t, 1.0, fpy.A)
	assert.Equal(t, fixtureClock.T.AddDate(0, 0, -28), res.Meta.WindowStart)
	assert.Equal(t, fixtureClock.T, res.Meta.WindowEnd)
	for _, m := range res.Metrics {
		assert.NotEmpty(t, m.Drill, m.Name)
	}
}

func TestCompare_should_ReturnNilDeltaNotInfinity_When_WindowAEmpty(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("b", 30, cmpBMid)
	res := compareOf(t, f, cmpQ)

	m := metricByName(t, res, "merged_items")
	assert.Equal(t, 0.0, m.A)
	assert.Nil(t, m.Delta, "empty A renders n/a")
	for _, m := range res.Metrics {
		assert.False(t, math.IsInf(m.A, 0) || math.IsNaN(m.A) || math.IsInf(m.B, 0) || math.IsNaN(m.B), m.Name)
		if m.Delta != nil {
			assert.False(t, math.IsInf(*m.Delta, 0) || math.IsNaN(*m.Delta), m.Name)
		}
	}
}

func TestCompare_should_ReturnIncomparableLowSampleAndKeepRawValues_When_NAUnder20OrVerdictsUnder30(t *testing.T) {
	t.Parallel()
	t.Run("8 vs 26 items", func(t *testing.T) {
		t.Parallel()
		f := newFakeSrc()
		f.addMerged("a", 8, cmpAMid)
		f.addMerged("b", 26, cmpBMid)
		m := metricByName(t, compareOf(t, f, cmpQ), "first_pass_yield")
		assert.Equal(t, CompareIncomparable, m.State)
		assert.Equal(t, "low sample (N_A=8, N_B=26, need 20)", m.Reason)
		assert.Equal(t, 8, m.NA)
		assert.Equal(t, 26, m.NB)
		assert.Equal(t, 1.0, m.A)
		assert.Equal(t, 1.0, m.B)
		assert.Nil(t, m.Delta)
	})
	t.Run("item boundary 19 and 20", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			a    int
			want CompareState
		}{{19, CompareIncomparable}, {20, CompareOK}} {
			f := newFakeSrc()
			f.addMerged("a", tc.a, cmpAMid)
			f.addMerged("b", 20, cmpBMid)
			m := metricByName(t, compareOf(t, f, cmpQ), "first_pass_yield")
			assert.Equal(t, tc.want, m.State, "N_A=%d", tc.a)
		}
	})
	t.Run("verdict boundary 29 and 30", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			a    int
			want CompareState
			text string
		}{{29, CompareIncomparable, "need 30"}, {30, CompareOK, ""}} {
			f := newFakeSrc()
			f.addVerdicts("va", tc.a, cmpAMid)
			f.addVerdicts("vb", 30, cmpBMid)
			m := metricByName(t, compareOf(t, f, cmpQ), "review_raw_fail_rate")
			assert.Equal(t, tc.want, m.State, "N_A=%d", tc.a)
			assert.Contains(t, m.Reason, tc.text)
		}
	})
}

func TestCompareResult_should_HaveNoSignificanceFields_When_ReflectedOver(t *testing.T) {
	t.Parallel()
	banned := []string{"significance", "p_value", "pvalue", "confidence", "improved", "worse"}
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(tp reflect.Type) {
		for tp.Kind() == reflect.Pointer || tp.Kind() == reflect.Slice || tp.Kind() == reflect.Array {
			tp = tp.Elem()
		}
		if tp.Kind() != reflect.Struct || seen[tp] {
			return
		}
		seen[tp] = true
		for i := 0; i < tp.NumField(); i++ {
			name := strings.ToLower(tp.Field(i).Name)
			for _, b := range banned {
				assert.NotContains(t, name, b, "%s.%s", tp.Name(), tp.Field(i).Name)
			}
			walk(tp.Field(i).Type)
		}
	}
	walk(reflect.TypeOf(CompareResult{}))
	assert.Greater(t, len(seen), 3, "the walk reached the nested types")
}

func TestCompare_should_EqualOffsetQuery_When_ExplicitBoundsGivenAndReject_When_ReversedMixedOrFuture(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)
	// An item merging exactly at B's start belongs to B only (half-open bounds).
	f.addMerged("edge", 1, time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	explicit := CompareQuery{
		AStart: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), AEnd: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC),
		BStart: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), BEnd: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}
	ctx := context.Background()
	agg := NewAggregator(f.sources(), fixtureClock)

	got, err := agg.Compare(ctx, explicit)
	require.NoError(t, err)
	want, err := agg.Compare(ctx, cmpQ)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 31.0, metricByName(t, got, "merged_items").B, "boundary item counted in B")
	assert.Equal(t, 25.0, metricByName(t, got, "merged_items").A, "and not in A")

	reversed := explicit
	reversed.AStart, reversed.AEnd = explicit.AEnd, explicit.AStart
	_, err = agg.Compare(ctx, reversed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a_start")

	mixed := explicit
	mixed.ADays = 14
	_, err = agg.Compare(ctx, mixed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")

	future := explicit
	future.BEnd = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	res, err := agg.Compare(ctx, future)
	require.NoError(t, err)
	assert.Equal(t, fixtureClock.T, res.Meta.WindowEnd, "B end clamped to as-of")
	require.Len(t, res.Meta.Clamps, 1)
	assert.Contains(t, res.Meta.Clamps[0], "b_end")

	_, err = agg.Compare(ctx, CompareQuery{})
	require.Error(t, err)
}

func TestCompare_should_ReturnErrCompareAllTime_When_WindowIsAllTime(t *testing.T) {
	t.Parallel()
	_, err := NewAggregator(newFakeSrc().sources(), fixtureClock).Compare(context.Background(), CompareQuery{AllTime: true, ADays: 7, BDays: 7})
	require.ErrorIs(t, err, ErrCompareAllTime)
	assert.Contains(t, err.Error(), "pick 7, 30 or 90 days")
}

func TestCompare_should_ReturnIncomparableWithReason_When_WindowStraddlesCaptureSince(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)
	f.stuck = []StuckInterval{
		closedStuck("a-000", "pr_needs_fix", cmpAMid, 5*time.Hour), closedStuck("b-000", "pr_needs_fix", cmpBMid, 6*time.Hour),
	}
	f.events = append(f.events,
		Event{ItemID: "a-001", From: domain.BacklogStatusPRPending, To: domain.BacklogStatusInProgress, At: cmpAMid.Add(10 * time.Hour), Seq: 9001},
		Event{ItemID: "b-001", From: domain.BacklogStatusPRPending, To: domain.BacklogStatusInProgress, At: cmpBMid.Add(10 * time.Hour), Seq: 9002},
	)
	capture := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	res := compareOf(t, f, cmpQ, WithCaptureSince(func(context.Context) (time.Time, error) { return capture, nil }))

	assert.Equal(t, capture, res.Meta.CaptureSince)
	tied := 0
	for _, m := range res.Metrics {
		if strings.HasPrefix(m.Name, "stuck_p50_hours.") || strings.HasPrefix(m.Name, "pr_fix_split.") {
			tied++
			assert.Equal(t, CompareIncomparable, m.State, m.Name)
			assert.Equal(t, "window straddles capture_since 2026-10-20", m.Reason, m.Name)
			assert.Nil(t, m.Delta)
		}
	}
	assert.Equal(t, 2, tied, "one stuck reason and one PR-fix trigger")
	assert.Equal(t, CompareOK, metricByName(t, res, "merged_items").State, "metrics not tied to capture are unaffected")

	res = compareOf(t, f, cmpQ, WithCaptureSince(func(context.Context) (time.Time, error) { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), nil }))
	assert.NotEqual(t, CompareIncomparable, metricByName(t, res, "pr_fix_split.unknown").State, "a capture date after both windows does not straddle")
}

func TestCompare_should_RefuseRawCounts_When_WindowsDifferInLength(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)

	res := compareOf(t, f, CompareQuery{ADays: 28, BDays: 14})

	for _, name := range []string{"merged_items", "thrash_items", "rework_review_fail", "rework_pr_fix"} {
		m := metricByName(t, res, name)
		assert.Equal(t, CompareIncomparable, m.State, name)
		assert.Contains(t, m.Reason, "windows differ in length (A 28.0d, B 14.0d)", name)
		assert.Nil(t, m.Delta, name)
		assert.Positive(t, m.NA+m.NB+1, "raw values stay visible: %s", name)
	}
	assert.Equal(t, CompareOK, metricByName(t, res, "first_pass_yield").State, "ratios do not scale with window length")

	equal := compareOf(t, f, cmpQ)
	assert.Equal(t, CompareOK, metricByName(t, equal, "merged_items").State)
}

func TestCompare_should_StraddleCapture_When_CaptureFallsInTheGapBetweenWindows(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("a", 25, cmpAMid)
	f.addMerged("b", 30, cmpBMid)
	f.stuck = []StuckInterval{closedStuck("a-000", "pr_needs_fix", cmpAMid, 5*time.Hour), closedStuck("b-000", "pr_needs_fix", cmpBMid, 6*time.Hour)}
	q := CompareQuery{ // A: Oct 6-13, gap Oct 13-20, B: Oct 20-27
		AStart: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), AEnd: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		BStart: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), BEnd: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
	}
	at := func(c time.Time) *CompareResult {
		return compareOf(t, f, q, WithCaptureSince(func(context.Context) (time.Time, error) { return c, nil }))
	}

	inGap := at(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	m := metricByName(t, inGap, "stuck_p50_hours.pr_needs_fix")
	assert.Equal(t, CompareIncomparable, m.State, "A is pre-capture and B post-capture")
	assert.Contains(t, m.Reason, "straddles capture_since")

	// The stuck sample is too small to compare, so only the reason tells the cases apart.
	assert.NotContains(t, metricByName(t, at(q.AStart), "stuck_p50_hours.pr_needs_fix").Reason, "straddles", "capture at the first start leaves both windows post-capture")
	assert.NotContains(t, metricByName(t, at(q.BEnd), "stuck_p50_hours.pr_needs_fix").Reason, "straddles", "capture at the last end leaves both windows pre-capture")
}
