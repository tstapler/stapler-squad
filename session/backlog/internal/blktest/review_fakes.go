package blktest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
)

// FakeAutoReopenSpawner is a test double implementing reviewgate.AutoReopenSpawner,
// recording every call and signaling on a channel so async callers (spawnReviewGate
// invokes it in a goroutine) can be synchronized with in tests.
type FakeAutoReopenSpawner struct {
	Called chan string // item IDs, one per call
}

func NewFakeAutoReopenSpawner() *FakeAutoReopenSpawner {
	return &FakeAutoReopenSpawner{Called: make(chan string, 8)}
}

func (f *FakeAutoReopenSpawner) AutoReopenAfterFailedReview(ctx context.Context, itemID string) error {
	f.Called <- itemID
	return nil
}

// NewNonEmptyDiffGitRepo creates a small real git repo with two commits (an initial
// commit plus a second one adding change.txt), so GetGitDiff's implicit HEAD~1..HEAD
// range produces a real, non-empty diff.
func NewNonEmptyDiffGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	InitConfiguredRepo(t, repoDir, "main")
	CommitFile(t, repoDir, "README.md", "base\n", "initial")
	CommitFile(t, repoDir, "change.txt", "change\n", "add change")
	return repoDir
}

// FakeCustomCheckCaller is a test double for reviewgate.CustomCheckCaller: it returns a
// canned (Output, Err) pair and records the args it was called with, without
// spawning any real subprocess.
type FakeCustomCheckCaller struct {
	Output string
	Err    error

	Called       bool
	LastKey      headless.FeatureKey
	LastSystem   string
	LastUserText string
}

func (f *FakeCustomCheckCaller) CallBlocking(_ context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, _ headless.CallOptions, _ headless.CostSink) (string, error) {
	f.Called = true
	f.LastKey = key
	f.LastSystem = systemPrompt
	f.LastUserText = userPrompt
	return f.Output, f.Err
}

// NewCustomCheckTestItem creates a minimal BacklogItemData for
// InvokeCustomGateCheck tests — no repo path/worktree needed since this gate
// kind never computes a diff.
func NewCustomCheckTestItem(t *testing.T, storage *session.Storage, title string) *backlogdto.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
		Title:              title,
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusIdea),
	})
	require.NoError(t, err)
	return item
}

// NewCustomCheckTestGate creates a real TransitionGate row (idea -> ready,
// kind=custom) and returns its ID — GateSatisfactionRecord.gate_id is a
// required ent edge to TransitionGate, so a bare uuid.New() with no backing
// row fails the FK constraint on Create.
func NewCustomCheckTestGate(t *testing.T, storage *session.Storage) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	client := storage.GetEntClient()

	fromStage, err := client.BacklogStage.Create().SetSlug("gcc-from-" + uuid.NewString()).SetName("From").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("gcc-to-" + uuid.NewString()).SetName("To").Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(domain.GateKindCustom)).
		SetStateful(true).
		Save(ctx)
	require.NoError(t, err)
	return gate.ID
}
