package session

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/flow"
)

func TestEntFlowReader_should_ReturnResolvedAndOpenRows_When_StuckIntervals(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		item := seedFlowItem(t, c, "stuck")
		cr := c.BacklogStuckState.Create().SetItemID(item).SetReason("pr_needs_fix").SetFirstDetectedAt(base)
		if i >= 7 {
			cr.SetResolvedAt(base.Add(time.Hour))
		}
		_, err := cr.Save(ctx)
		require.NoError(t, err)
	}

	got, err := NewEntFlowReader(c).StuckIntervals(ctx)
	require.NoError(t, err)
	require.Len(t, got, 12)
	open := 0
	for _, iv := range got {
		if iv.ResolvedAt == nil {
			open++
		}
		require.Equal(t, "pr_needs_fix", iv.Reason)
		require.Equal(t, base, iv.FirstDetectedAt)
	}
	require.Equal(t, 7, open)
}

func TestEntFlowReader_should_ReturnRawTitle_When_ItemTitleIsLongWithNewline(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	title := strings.Repeat("x", 249) + "\n" + strings.Repeat("y", 250)
	item := seedFlowItem(t, c, title)

	got, err := NewEntFlowReader(c).Items(context.Background(), []flow.ItemID{flow.ItemID(item.String()), flow.ItemID(uuid.NewString()), "not-a-uuid"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, title, got[0].Title)
	require.Equal(t, "idea", string(got[0].Status))
}

func TestEntFlowReader_should_NotReferenceSanitizer_When_ReadingTitles(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("ent_repository_flow*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		require.NotContains(t, string(src), "SanitizeForAgentContext", f)
	}
}

func TestEntFlowReader_should_ReturnBuiltInStagesInPipelineOrder_When_DefaultEngine(t *testing.T) {
	t.Parallel()
	got, err := NewEntFlowReader(nil).Stages(context.Background())
	require.NoError(t, err)
	require.Equal(t, BuiltInStageOrder, got)
	require.Contains(t, got, BacklogStatusRefining)
}

func TestPipelineStages_should_PlaceCustomStageAtConfiguredPosition_When_ConfiguredEngine(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()
	qa, err := client.BacklogStage.Create().SetSlug("qa").SetName("QA").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	edge := func(from, to *ent.BacklogStage) {
		_, err := client.StageTransition.Create().SetFromStageID(from.ID).SetToStageID(to.ID).SetEnabled(true).Save(ctx)
		require.NoError(t, err)
	}
	stage := func(s BacklogStatus) *ent.BacklogStage {
		st, err := client.BacklogStage.Query().Where(backlogstage.Slug(string(s))).Only(ctx)
		require.NoError(t, err)
		return st
	}
	edge(stage(BacklogStatusReview), qa)
	edge(qa, stage(BacklogStatusPRPending))
	require.NoError(t, engine.InvalidateCache(ctx))

	got := PipelineStages(engine)
	want := []BacklogStatus{
		BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued, BacklogStatusInProgress,
		BacklogStatusReview, "qa", BacklogStatusPRPending, BacklogStatusDone, BacklogStatusArchived,
	}
	require.Equal(t, want, got)
}

func seedStuckEpisode(t *testing.T, c *ent.Client, item uuid.UUID, reason string, opened time.Time, resolved *time.Time) {
	t.Helper()
	cr := c.BacklogStuckEpisode.Create().SetItemID(item).SetReason(reason).SetOpenedAt(opened)
	if resolved != nil {
		cr.SetResolvedAt(*resolved)
	}
	_, err := cr.Save(context.Background())
	require.NoError(t, err)
}

func seedStuckRow(t *testing.T, c *ent.Client, item uuid.UUID, reason string, first time.Time, resolved *time.Time) {
	t.Helper()
	cr := c.BacklogStuckState.Create().SetItemID(item).SetReason(reason).SetFirstDetectedAt(first)
	if resolved != nil {
		cr.SetResolvedAt(*resolved)
	}
	_, err := cr.Save(context.Background())
	require.NoError(t, err)
}

func TestStuckReader_should_UnionEpisodesWithLegacyAndNormalizeUtc_When_CaptureSinceSet(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	ctx := context.Background()
	la := time.FixedZone("PDT", -7*3600)
	old := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	oldEnd := old.Add(5 * time.Hour)
	capture := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	// Item A: pre-capture closed legacy row only (kept as-is, lower bound).
	a := seedFlowItem(t, c, "a")
	seedStuckRow(t, c, a, "bouncing", old, &oldEnd)
	// Item B: legacy row opened before capture and still open when the episode started 11-10;
	// legacy first_detected_at is local-time 11-09 20:00 PDT (= 11-10 03:00Z), after the episode start
	// only if the zone is ignored. In UTC it is 11-10 03:00Z >= capture 11-10 00:00Z, so it is dropped.
	b := seedFlowItem(t, c, "b")
	seedStuckRow(t, c, b, "pr_needs_fix", time.Date(2026, 11, 9, 20, 0, 0, 0, la), nil)
	seedStuckEpisode(t, c, b, "pr_needs_fix", capture, nil)
	// Item C: legacy row from before capture still open, episode opened later: legacy clipped at episode start.
	cc := seedFlowItem(t, c, "c")
	seedStuckRow(t, c, cc, "bouncing", old, nil)
	epOpen := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	epEnd := epOpen.Add(2 * time.Hour)
	seedStuckEpisode(t, c, cc, "bouncing", epOpen, &epEnd)

	got, err := NewEntFlowReader(c).StuckIntervals(ctx)
	require.NoError(t, err)

	type key struct {
		item    string
		episode bool
	}
	byKey := map[key]flow.StuckInterval{}
	for _, iv := range got {
		require.Equal(t, time.UTC, iv.FirstDetectedAt.Location())
		byKey[key{string(iv.ItemID), iv.Episode}] = iv
	}
	require.Len(t, got, 4, "A legacy, B episode only, C clipped legacy + C episode")
	assert.Contains(t, byKey, key{a.String(), false})
	assert.NotContains(t, byKey, key{b.String(), false}, "legacy row at/after the episode start is dropped")
	assert.Contains(t, byKey, key{b.String(), true})
	clipped := byKey[key{cc.String(), false}]
	require.NotNil(t, clipped.ResolvedAt)
	assert.Equal(t, epOpen, *clipped.ResolvedAt, "open legacy row clipped at the first episode start")
	assert.True(t, byKey[key{cc.String(), true}].Episode)
}

func TestStuckReader_should_ReportEpisodeDriftInputs_When_StuckCapture(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	ctx := context.Background()
	capture := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedStuckEpisode(t, c, seedFlowItem(t, c, "e"), "bouncing", capture.Add(time.Duration(i)*time.Hour), nil)
	}
	for i := 0; i < 4; i++ {
		seedStuckRow(t, c, seedFlowItem(t, c, "s"), "bouncing", capture.Add(time.Duration(i+1)*time.Hour), nil)
	}
	seedStuckRow(t, c, seedFlowItem(t, c, "pre"), "bouncing", capture.Add(-48*time.Hour), nil) // pre-capture open row is excluded

	got, err := NewEntFlowReader(c).StuckCapture(ctx)
	require.NoError(t, err)
	assert.Equal(t, flow.StuckCapture{CaptureSince: capture, OpenEpisodes: 3, OpenStateRows: 4}, got)
	assert.Equal(t, 1, got.Drift())
}

func TestStuckReader_should_FallBackToLegacyOnly_When_EpisodeTableMissingOrEmpty(t *testing.T) {
	t.Parallel()
	repo := newFlowTestRepo(t)
	c := repo.GetEntClient()
	ctx := context.Background()
	first := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	seedStuckRow(t, c, seedFlowItem(t, c, "x"), "bouncing", first, nil)
	r := NewEntFlowReader(c)

	got, err := r.StuckIntervals(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].Episode)
	cap0, err := r.StuckCapture(ctx)
	require.NoError(t, err)
	assert.True(t, cap0.CaptureSince.IsZero(), "empty table: capture unset")

	require.NoError(t, repo.driver.Exec(ctx, "DROP TABLE backlog_stuck_episodes", []any{}, new(sql.Result)))
	got, err = r.StuckIntervals(ctx)
	require.NoError(t, err, "missing table must not fail the stuck section")
	require.Len(t, got, 1)
	cap1, err := r.StuckCapture(ctx)
	require.NoError(t, err)
	assert.True(t, cap1.CaptureSince.IsZero())
}
