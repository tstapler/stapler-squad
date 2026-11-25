package backlogsvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

const e2eACs = `[{"index":0,"text":"works","status":"pending"},{"index":1,"text":"e2e: make e2e-epic","status":"pending"}]`

type epicFixture struct {
	svc  *BacklogService
	epic *backlogdto.BacklogItemData
	sibs []string
}

func newEpicFixture(t *testing.T, flagOn bool, epicACs string, rework *int) epicFixture {
	t.Helper()
	svc := NewBacklogService(svctest.CreateTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	svc.cfg = &config.Config{FeatureFlags: map[string]bool{featureflags.EpicIntegrationChildFlagName: flagOn}}
	svc.flags = featureflags.NewRegistry([]featureflags.Spec{{Name: featureflags.EpicIntegrationChildFlagName}})
	ctx := context.Background()
	epic, err := svc.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title: "epic", AcceptanceCriteria: domain.AcCriteriaJSON(epicACs), ReworkCapOverride: rework,
	})
	require.NoError(t, err)
	f := epicFixture{svc: svc, epic: epic}
	for _, title := range []string{"slice-a", "slice-b"} {
		s, err := svc.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: title})
		require.NoError(t, err)
		f.sibs = append(f.sibs, s.ID)
	}
	return f
}

func (f epicFixture) setShipped(t *testing.T, id string, paths string) {
	t.Helper()
	_, err := f.svc.storage.UpdateBacklogItem(context.Background(), id, backlogdto.BacklogItemUpdate{ShippedFileStats: &paths}, nil)
	require.NoError(t, err)
}

func TestOnEpicPlanningComplete_should_CreateOneChildWithCommand_When_FlagOnAndE2EAC(t *testing.T) {
	f := newEpicFixture(t, true, e2eACs, nil)
	ctx := context.Background()

	child, err := f.svc.OnEpicPlanningComplete(ctx, f.epic.ID, f.sibs)
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.True(t, child.IsIntegrationChild)
	assert.Contains(t, child.Description, "make e2e-epic")

	again, err := f.svc.OnEpicPlanningComplete(ctx, f.epic.ID, f.sibs)
	require.NoError(t, err)
	assert.Equal(t, child.ID, again.ID, "re-plan must not create a second child")
}

func TestOnEpicPlanningComplete_should_NotCreateChild_When_FlagOff(t *testing.T) {
	f := newEpicFixture(t, false, e2eACs, nil)
	child, err := f.svc.OnEpicPlanningComplete(context.Background(), f.epic.ID, f.sibs)
	require.NoError(t, err)
	assert.Nil(t, child)
}

func TestOnEpicPlanningComplete_should_NotCreateChild_When_NoE2ECommand(t *testing.T) {
	f := newEpicFixture(t, true, `[{"index":0,"text":"works","status":"pending"}]`, nil)
	child, err := f.svc.OnEpicPlanningComplete(context.Background(), f.epic.ID, f.sibs)
	require.NoError(t, err)
	assert.Nil(t, child)
	got, err := f.svc.storage.GetBacklogItem(context.Background(), f.epic.ID)
	require.NoError(t, err)
	assert.Empty(t, got.IntegrationChildID)
}

func (f epicFixture) integrationChild(t *testing.T) *backlogdto.BacklogItemData {
	t.Helper()
	child, err := f.svc.OnEpicPlanningComplete(context.Background(), f.epic.ID, f.sibs)
	require.NoError(t, err)
	require.NotNil(t, child)
	return child
}

func TestOnIntegrationChildFailed_should_CreateFixChildForSibling_When_ConfidentAttribution(t *testing.T) {
	f := newEpicFixture(t, true, e2eACs, nil)
	f.setShipped(t, f.sibs[0], `[{"path":"a/x.go"},{"path":"a/y.go"}]`)
	f.setShipped(t, f.sibs[1], `[{"path":"b/z.go"}]`)
	child := f.integrationChild(t)
	ctx := context.Background()

	out, err := f.svc.OnIntegrationChildFailed(ctx, child.ID, []string{"a/x.go", "a/y.go"})
	require.NoError(t, err)

	assert.Equal(t, f.sibs[0], out.AttributedTo)
	require.NotEmpty(t, out.FixChildID)
	assert.Empty(t, out.GuidanceRequestID)
	blockedBy, _, err := f.svc.storage.ListBacklogItemDependencies(ctx, out.FixChildID)
	require.NoError(t, err)
	require.Len(t, blockedBy, 1)
	assert.Equal(t, f.sibs[0], blockedBy[0].ItemID, "fix child depends on the attributed sibling")
	icBlockedBy, _, err := f.svc.storage.ListBacklogItemDependencies(ctx, child.ID)
	require.NoError(t, err)
	var ids []string
	for _, r := range icBlockedBy {
		ids = append(ids, r.ItemID)
	}
	assert.Contains(t, ids, out.FixChildID, "integration child waits on the fix child")
	epic, err := f.svc.storage.GetBacklogItem(ctx, f.epic.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, epic.IntegrationFixCount)
}

func TestOnIntegrationChildFailed_should_EscalateToEpic_When_ConfidenceBelowThreshold(t *testing.T) {
	f := newEpicFixture(t, true, e2eACs, nil)
	f.setShipped(t, f.sibs[0], `[{"path":"a/x.go"}]`)
	f.setShipped(t, f.sibs[1], `[{"path":"b/z.go"}]`)
	child := f.integrationChild(t)

	// a/x.go is 1 of 3 failing files: 0.33 < 0.6.
	out, err := f.svc.OnIntegrationChildFailed(context.Background(), child.ID, []string{"a/x.go", "q.go", "r.go"})
	require.NoError(t, err)

	assert.Empty(t, out.FixChildID)
	assert.NotEmpty(t, out.GuidanceRequestID)
	epic, err := f.svc.storage.GetBacklogItem(context.Background(), f.epic.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, epic.IntegrationFixCount)
}

func TestOnIntegrationChildFailed_should_EscalateRegardlessOfConfidence_When_FixCapReached(t *testing.T) {
	one := 1
	f := newEpicFixture(t, true, e2eACs, &one)
	f.setShipped(t, f.sibs[0], `[{"path":"a/x.go"}]`)
	child := f.integrationChild(t)
	ctx := context.Background()

	first, err := f.svc.OnIntegrationChildFailed(ctx, child.ID, []string{"a/x.go"})
	require.NoError(t, err)
	require.NotEmpty(t, first.FixChildID)

	second, err := f.svc.OnIntegrationChildFailed(ctx, child.ID, []string{"a/x.go"})
	require.NoError(t, err)
	assert.Empty(t, second.FixChildID, "cap of 1 reached")
	assert.NotEmpty(t, second.GuidanceRequestID)
}

func TestOnIntegrationChildFailed_should_RespectConfiguredThreshold(t *testing.T) {
	f := newEpicFixture(t, true, e2eACs, nil)
	f.svc.cfg.IntegrationAttributionThreshold = 0.2
	f.setShipped(t, f.sibs[0], `[{"path":"a/x.go"}]`)
	child := f.integrationChild(t)

	out, err := f.svc.OnIntegrationChildFailed(context.Background(), child.ID, []string{"a/x.go", "q.go", "r.go"})
	require.NoError(t, err)
	assert.NotEmpty(t, out.FixChildID, "0.33 >= configured 0.2")
}

// Full flow at service level: epic -> siblings done -> integration child auto-created ->
// epic blocked -> integration child passes -> epic closes. (The Playwright e2e variant needs an
// RPC for the Epic items planning hook, which does not exist yet.)
func TestEpicIntegrationFlow_should_CloseEpic_When_IntegrationChildPasses(t *testing.T) {
	f := newEpicFixture(t, true, e2eACs, nil)
	ctx := context.Background()
	repo := f.svc.storage.Repo
	for _, id := range f.sibs {
		_, err := repo.RawTransitionBacklogItemStatus(ctx, id, domain.BacklogStatusDone, nil, "test")
		require.NoError(t, err)
	}

	child, err := f.svc.OnEpicPlanningComplete(ctx, f.epic.ID, f.sibs)
	require.NoError(t, err)
	require.NotNil(t, child)
	unresolved, err := f.svc.storage.UnresolvedBlockerIDs(ctx, child.ID)
	require.NoError(t, err)
	assert.Empty(t, unresolved, "all siblings are done, so the integration child is startable")

	_, err = repo.RawTransitionBacklogItemStatus(ctx, f.epic.ID, domain.BacklogStatusDone, nil, "test")
	require.ErrorIs(t, err, backlogdto.ErrIntegrationCheckPending)

	_, err = repo.RawTransitionBacklogItemStatus(ctx, child.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	closed, err := repo.RawTransitionBacklogItemStatus(ctx, f.epic.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusDone), string(closed.Status))
}
