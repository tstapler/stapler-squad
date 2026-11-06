package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
	require.Equal(t, builtInStageOrder, got)
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
