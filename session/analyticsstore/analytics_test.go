package analyticsstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/dismissedfinding"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newAnalytics(t testing.TB) *Analytics {
	t.Helper()
	return New(entdbtest.NewClient(t))
}

func row(id, program, sub string, at time.Time) domain.AnalyticsData {
	return domain.AnalyticsData{
		ID: id, SessionID: "s", ToolName: "Bash", CommandPreview: id, Decision: "allow",
		RiskLevel: "low", CommandProgram: program, CommandSubcategory: sub, Source: "claude", CreatedAt: at,
	}
}

func TestAnalytics_should_ListNewestFirstAndFilterByProgram_When_RowsRecorded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	require.NoError(t, a.RecordAnalyticsBatch(ctx, []domain.AnalyticsData{
		row("a", "git", "status", t0),
		row("b", "git", "diff", t0.Add(time.Minute)),
		row("c", "go", "test", t0.Add(2*time.Minute)),
	}))

	all, err := a.ListAnalytics(ctx, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "c", all[0].ID)

	byProg, err := a.ListAnalyticsByProgramSince(ctx, "git", t0, 0)
	require.NoError(t, err)
	assert.Len(t, byProg, 2)

	since, err := a.ListAnalyticsSince(ctx, t0.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, since, 1)
	assert.Equal(t, "c", since[0].ID)
}

func TestAnalytics_should_BeIdempotent_When_BatchRepeatsAnID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	batch := []domain.AnalyticsData{row("dup", "git", "status", t0)}
	require.NoError(t, a.RecordAnalyticsBatch(ctx, batch))
	require.NoError(t, a.RecordAnalyticsBatch(ctx, batch))
	require.NoError(t, a.RecordAnalyticsBatch(ctx, nil))
	all, err := a.ListAnalytics(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestAnalytics_should_AggregateSubcommands_When_QueriedByProgram(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	require.NoError(t, a.RecordAnalytics(ctx, row("a", "git", "status", t0)))
	require.NoError(t, a.RecordAnalytics(ctx, row("b", "git", "status", t0.Add(time.Minute))))
	require.NoError(t, a.RecordAnalytics(ctx, row("c", "git", "diff", t0.Add(2*time.Minute))))

	counts, err := a.GetSubcommandBreakdown(ctx, "git", t0)
	require.NoError(t, err)
	got := map[string]int{}
	for _, c := range counts {
		got[c.Subcommand+"/"+c.Decision] = c.Count
	}
	assert.Equal(t, map[string]int{"status/allow": 2, "diff/allow": 1}, got)

	trend, err := a.GetSubcommandTrend(ctx, "git", "status", t0)
	require.NoError(t, err)
	require.Len(t, trend, 2)
	assert.Equal(t, "a", trend[0].ID, "trend is oldest first")

	recent, err := a.ListRecentCommandsByProgram(ctx, "git", "", t0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "b"}, recent)
}

func TestAnalytics_should_KeepFirstDismissedAt_When_FindingDismissedTwice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	require.NoError(t, a.DismissFinding(ctx, domain.DismissedFindingData{FindingID: "f1", SessionID: "s1", DismissedAt: t0}))
	require.NoError(t, a.DismissFinding(ctx, domain.DismissedFindingData{FindingID: "f1", SessionID: "s2", DismissedAt: t0.Add(time.Hour)}))
	ids, err := a.ListDismissedFindingIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"f1": true}, ids)

	stored, err := a.client.DismissedFinding.Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "s2", stored.SessionID)
	assert.True(t, stored.DismissedAt.Equal(t0), "dismissed_at is immutable")
}

func ids(rows []domain.AnalyticsData) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func TestAnalytics_should_IncludeBoundaryRow_When_ListedSince(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	require.NoError(t, a.RecordAnalyticsBatch(ctx, []domain.AnalyticsData{
		row("before", "git", "x", t0.Add(-time.Nanosecond)),
		row("at", "git", "x", t0),
		row("after", "git", "x", t0.Add(time.Second)),
	}))
	got, err := a.ListAnalyticsSince(ctx, t0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"after", "at"}, ids(got))
	byProg, err := a.ListAnalyticsByProgramSince(ctx, "git", t0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"after", "at"}, ids(byProg))
}

func TestAnalytics_should_OrderNewestFirstAndTruncate_When_ListedByProgramWithLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	require.NoError(t, a.RecordAnalyticsBatch(ctx, []domain.AnalyticsData{
		row("old", "git", "x", t0),
		row("new", "git", "x", t0.Add(3*time.Minute)),
		row("mid", "git", "x", t0.Add(time.Minute)),
		row("other", "go", "x", t0.Add(4*time.Minute)),
	}))
	all, err := a.ListAnalyticsByProgramSince(ctx, "git", t0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"new", "mid", "old"}, ids(all))
	two, err := a.ListAnalyticsByProgramSince(ctx, "git", t0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"new", "mid"}, ids(two))
}

func TestAnalytics_should_FilterBySubcommandAndSkipEmptyPreviews_When_ListingRecentCommands(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	empty := row("e", "git", "status", t0.Add(3*time.Minute))
	empty.CommandPreview = ""
	require.NoError(t, a.RecordAnalyticsBatch(ctx, []domain.AnalyticsData{
		row("a", "git", "status", t0),
		row("b", "git", "diff", t0.Add(time.Minute)),
		row("c", "git", "status", t0.Add(2*time.Minute)),
		empty,
	}))
	status, err := a.ListRecentCommandsByProgram(ctx, "git", "status", t0, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "a"}, status, "subcommand filter applies and the empty preview is dropped")
	allSubs, err := a.ListRecentCommandsByProgram(ctx, "git", "", t0, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"c", "b", "a"}, allSubs)
	// The limit is applied in SQL before empty previews are dropped, so an empty newest row uses up a slot.
	limited, err := a.ListRecentCommandsByProgram(ctx, "git", "", t0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"c"}, limited)
}

func TestAnalytics_should_DefaultDismissedAtToNow_When_ZeroAndKeepItWhenGiven(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	before := time.Now().Add(-time.Second)
	require.NoError(t, a.DismissFinding(ctx, domain.DismissedFindingData{FindingID: "zero"}))
	after := time.Now().Add(time.Second)
	require.NoError(t, a.DismissFinding(ctx, domain.DismissedFindingData{FindingID: "given", DismissedAt: t0}))

	zero, err := a.client.DismissedFinding.Query().Where(dismissedfinding.FindingID("zero")).Only(ctx)
	require.NoError(t, err)
	assert.False(t, zero.DismissedAt.IsZero())
	assert.True(t, zero.DismissedAt.After(before) && zero.DismissedAt.Before(after), "got %v", zero.DismissedAt)
	given, err := a.client.DismissedFinding.Query().Where(dismissedfinding.FindingID("given")).Only(ctx)
	require.NoError(t, err)
	assert.True(t, given.DismissedAt.Equal(t0))
}

func TestAnalytics_should_RoundTripEveryField_When_SavedAndListed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newAnalytics(t)
	in := domain.AnalyticsData{
		ID: "rt", SessionID: "s1", ToolName: "Bash", CommandPreview: "git status", Cwd: "/w",
		Decision: "deny", RiskLevel: "high", RuleID: "r1", RuleName: "no-force-push", Reason: "because",
		Alternative: "alt", DurationMs: 42, ApprovalID: "ap", CommandProgram: "git", CommandCategory: "vcs",
		CommandSubcategory: "status", PythonImports: []string{"os", "sys"}, Source: "pi", CreatedAt: t0,
	}
	require.NoError(t, a.RecordAnalytics(ctx, in))
	got, err := a.ListAnalytics(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[0].CreatedAt.Equal(t0))
	got[0].CreatedAt = in.CreatedAt
	assert.Equal(t, in, got[0])
}
