package session

// Task 1.5.9d: the replay runs against a fixture database built in a temp dir.
// It never opens, reads or writes ~/.stapler-squad: the live-directory guard is
// exercised with a temp dir standing in for it.

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
)

// replayFixture builds a database file with the built-in stages and n idea
// items, lets mutate edit the stage configuration, and returns its path.
func replayFixture(t *testing.T, ideaItems int, mutate func(context.Context, StageCRUDRepository)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "copy.db")
	repo, err := NewEntRepository(WithDatabasePath(path))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, repo.GetEntClient()))
	for range ideaItems {
		_, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "replay item", Status: string(BacklogStatusIdea)})
		require.NoError(t, err)
	}
	if mutate != nil {
		mutate(ctx, NewEntStageConfigRepository(repo.GetEntClient()))
	}
	require.NoError(t, repo.Close())
	return path
}

func replayDisableEdge(t *testing.T, edge SystemEdge) func(context.Context, StageCRUDRepository) {
	return func(ctx context.Context, repo StageCRUDRepository) {
		t.Helper()
		ts, err := repo.ListAllTransitions(ctx, string(edge.From))
		require.NoError(t, err)
		off := false
		for _, tr := range ts {
			if tr.ToStageSlug == string(edge.To) {
				_, err := repo.UpdateTransition(ctx, tr.ID, TransitionUpdateInput{Enabled: &off})
				require.NoError(t, err)
				return
			}
		}
		require.FailNowf(t, "edge not seeded", "%s->%s", edge.From, edge.To)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func replayReport(t *testing.T, path string) *ReplayReport {
	t.Helper()
	client, closeFn, err := OpenReplayCopy(path, nil)
	require.NoError(t, err)
	defer closeFn()
	report, err := BuildWorkflowEngineReplay(context.Background(), client)
	require.NoError(t, err)
	return report
}

func (r *ReplayReport) find(caller TransitionCaller, from, to BacklogStatus) (ReplayDecision, bool) {
	for _, d := range r.Decisions {
		if d.Caller == caller && d.Edge.From == from && d.Edge.To == to {
			return d, true
		}
	}
	return ReplayDecision{}, false
}

func TestValidateReplayDBPath_RefusesLiveDirectoryAliasesAndStaleWAL(t *testing.T) {
	live := t.TempDir()
	liveDB := filepath.Join(live, "sessions.db")
	require.NoError(t, os.WriteFile(liveDB, []byte("live"), 0o600))
	outside := t.TempDir()

	_, err := ValidateReplayDBPath(liveDB, []string{live})
	require.ErrorIs(t, err, ErrReplayLiveDatabase, "the live database itself")

	nested := filepath.Join(live, "instances", "x", "copy.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o750))
	require.NoError(t, os.WriteFile(nested, []byte("x"), 0o600))
	_, err = ValidateReplayDBPath(nested, []string{live})
	require.ErrorIs(t, err, ErrReplayLiveDatabase, "any file under a live directory, copy or not")

	link := filepath.Join(outside, "link.db")
	require.NoError(t, os.Symlink(liveDB, link))
	_, err = ValidateReplayDBPath(link, []string{live})
	require.ErrorIs(t, err, ErrReplayLiveDatabase, "a symlink to the live database")

	hard := filepath.Join(outside, "hard.db")
	require.NoError(t, os.Link(liveDB, hard))
	_, err = ValidateReplayDBPath(hard, []string{live})
	require.ErrorIs(t, err, ErrReplayLiveDatabase, "a hard link to the live database")

	copyPath := filepath.Join(outside, "copy.db")
	require.NoError(t, os.WriteFile(copyPath, []byte("copy"), 0o600))
	_, err = ValidateReplayDBPath(copyPath, []string{live})
	require.NoError(t, err, "a real copy elsewhere is accepted")

	require.NoError(t, os.WriteFile(copyPath+"-wal", []byte("pending"), 0o600))
	_, err = ValidateReplayDBPath(copyPath, []string{live})
	require.ErrorContains(t, err, "-wal is not empty")

	_, err = ValidateReplayDBPath("", nil)
	require.Error(t, err)
	_, err = ValidateReplayDBPath(outside, nil)
	require.ErrorContains(t, err, "is a directory")
	_, err = ValidateReplayDBPath(filepath.Join(outside, "missing.db"), nil)
	require.Error(t, err)
}

func TestRunWorkflowEngineReplay_RefusesAPathInsideALiveDirectoryBeforeOpeningIt(t *testing.T) {
	live := t.TempDir()
	path := replayFixture(t, 0, nil)
	inside := filepath.Join(live, "copy.db")
	require.NoError(t, os.Rename(path, inside))

	_, _, err := RunWorkflowEngineReplay(context.Background(), inside, []string{live})

	require.ErrorIs(t, err, ErrReplayLiveDatabase)
}

func TestReplay_OpensTheCopyReadOnlyAndLeavesItByteIdentical(t *testing.T) {
	path := replayFixture(t, 2, nil)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	namesBefore := dirNames(t, filepath.Dir(path))

	out, failed, err := RunWorkflowEngineReplay(context.Background(), path, nil)
	require.NoError(t, err)
	require.False(t, failed)
	require.NotEmpty(t, out)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, sha256.Sum256(before), sha256.Sum256(after), "the copy is byte-identical after a replay")
	assert.Equal(t, namesBefore, dirNames(t, filepath.Dir(path)), "no -wal, -shm or other file appeared next to the copy")

	client, closeFn, err := OpenReplayCopy(path, nil)
	require.NoError(t, err)
	defer closeFn()
	_, err = client.BacklogItem.Create().SetTitle("must not persist").Save(context.Background())
	require.Error(t, err, "the replay connection cannot write")
}

func TestReplay_SeededCopy_AllowsEveryAgentAndSystemEdgeAndGatesEachSystemRequiredEdge(t *testing.T) {
	report := replayReport(t, replayFixture(t, 3, nil))

	assert.Equal(t, "seeded", report.Config)
	assert.False(t, report.Failed(), strings.Join(report.Failures, "\n"))
	assert.Equal(t, 3, report.LiveItems)
	assert.Zero(t, report.StrandedAll)

	want := 0
	for _, spec := range TransitionCallers() {
		want += len(replayEdges(spec))
	}
	assert.Len(t, report.Decisions, want, "one row per registered caller and declared edge")

	merge, ok := report.find(CallerReconcilePRPendingMerge, BacklogStatusPRPending, BacklogStatusDone)
	require.True(t, ok)
	assert.Equal(t, "allowed", merge.Live)
	assert.Equal(t, "denied:"+DeniedOperatorGate, merge.Synthetic, "the edited path: an unsatisfied operator gate denies the merge advance")

	for _, d := range report.Decisions {
		if d.Synthetic != "-" && d.Synthetic != replayNoStoredTransition {
			assert.Equal(t, "denied:"+DeniedOperatorGate, d.Synthetic, "%s %s->%s", d.Caller, d.Edge.From, d.Edge.To)
		}
	}
	assert.Positive(t, report.SyntheticRan)
}

func TestReplayDecision_Failure_FlagsOnlyAgentAndSystemGraphDenials(t *testing.T) {
	denied := func(class CallerClass, reason string) ReplayDecision {
		return ReplayDecision{Caller: "c", Class: class, Edge: SystemEdge{From: BacklogStatusIdea, To: BacklogStatusReady}, Live: replayDeniedPrefix + reason}
	}
	for _, class := range []CallerClass{ClassAgent, ClassSystem} {
		msg, failed := denied(class, DeniedConfigGraph).failure()
		assert.True(t, failed, class)
		assert.Contains(t, msg, "no gate involved")
		_, failed = denied(class, DeniedTargetStageRemoved).failure()
		assert.True(t, failed, class)
		_, failed = denied(class, DeniedOperatorGate).failure()
		assert.False(t, failed, "a denial by a real operator gate is the configuration working")
	}
	_, failed := denied(ClassOperator, DeniedConfigGraph).failure()
	assert.False(t, failed, "operators choose their edge, so a disabled edge denies them by design")
	_, failed = (ReplayDecision{Class: ClassSystem, Live: "allowed"}).failure()
	assert.False(t, failed)
}

func TestReplay_ARealGateOnAFixedEdgeIsReportedDeniedButIsNotAFailure(t *testing.T) {
	addGate := func(ctx context.Context, repo StageCRUDRepository) {
		ts, err := repo.ListAllTransitions(ctx, string(BacklogStatusPRPending))
		require.NoError(t, err)
		for _, tr := range ts {
			if tr.ToStageSlug == string(BacklogStatusDone) {
				_, err := repo.CreateGate(ctx, GateCreateInput{TransitionID: tr.ID, Kind: string(GateKindHumanApproval), Enabled: true})
				require.NoError(t, err)
			}
		}
	}

	report := replayReport(t, replayFixture(t, 0, addGate))

	assert.Equal(t, "edited", report.Config)
	merge, ok := report.find(CallerReconcilePRPendingMerge, BacklogStatusPRPending, BacklogStatusDone)
	require.True(t, ok)
	assert.Equal(t, "denied:"+DeniedOperatorGate, merge.Live)
	assert.False(t, report.Failed(), strings.Join(report.Failures, "\n"))
}

func TestReplay_SystemRequiredEdgeWithNoStoredTransitionCannotCarryAGate(t *testing.T) {
	report := replayReport(t, replayFixture(t, 0, nil))

	d, ok := report.find(CallerRecoverDriftedPR, BacklogStatusReady, BacklogStatusPRPending)
	require.True(t, ok)
	assert.Equal(t, "allowed", d.Live)
	assert.Equal(t, replayNoStoredTransition, d.Synthetic)
}

func TestReplay_PreviewsItemsStrandedByAnEditedGraph(t *testing.T) {
	report := replayReport(t, replayFixture(t, 2, replayDisableEdge(t, SystemEdge{From: BacklogStatusIdea, To: BacklogStatusRefining})))

	assert.Equal(t, 2, report.LiveItems)
	assert.Equal(t, 2, report.Stranded[BacklogStatusIdea], "both idea items lose idea->refining under enforce")
	out := report.Render()
	assert.Contains(t, out, "stranded-item preview: 2 of 2 live item(s)")
	assert.Contains(t, out, "idea")
}

func TestReplay_RenderListsEveryCallerAndTheVerdict(t *testing.T) {
	out := replayReport(t, replayFixture(t, 0, nil)).Render()

	for _, spec := range TransitionCallers() {
		if len(replayEdges(spec)) > 0 {
			assert.Contains(t, out, string(spec.Name))
		}
	}
	assert.Contains(t, out, "OK: no fixed agent or system edge is denied without a gate")
	assert.Contains(t, out, "gates applied in memory only")
}

// TestWorkflowEngineReplayCommand is the body of `make workflow-engine-replay`:
// it replays the copy named by WORKFLOW_ENGINE_REPLAY_DB, prints the report and
// fails (so the make target exits non-zero) when a fixed edge is denied or the
// path is refused. It is skipped when the variable is unset.
func TestWorkflowEngineReplayCommand(t *testing.T) {
	path := os.Getenv("WORKFLOW_ENGINE_REPLAY_DB")
	if path == "" {
		t.Skip("set WORKFLOW_ENGINE_REPLAY_DB to a COPY of the database (make workflow-engine-replay DB=<copy>)")
	}
	out, failed, err := RunWorkflowEngineReplay(context.Background(), path, LiveStateDirs())
	require.NoError(t, err)
	_, _ = os.Stdout.WriteString(out)
	if failed {
		t.Fatal("a fixed agent or system edge is denied with no gate; see the report above")
	}
}

func TestReplayGateOverlay_AddsOneUnsatisfiedGateInMemoryOnly(t *testing.T) {
	path := replayFixture(t, 0, nil)
	client, closeFn, err := OpenReplayCopy(path, nil)
	require.NoError(t, err)
	defer closeFn()
	base := NewEntStageConfigRepository(client)
	edge := SystemEdge{From: BacklogStatusReady, To: BacklogStatusQueued}

	plain, err := base.ListEnabledTransitions(context.Background())
	require.NoError(t, err)
	over, err := replayGateOverlay{base, edge}.ListEnabledTransitions(context.Background())
	require.NoError(t, err)

	gates := func(ts []*ent.StageTransition) (n int) {
		for _, tr := range ts {
			if tr.Edges.FromStage.Slug == string(edge.From) && tr.Edges.ToStage.Slug == string(edge.To) {
				n += len(tr.Edges.Gates)
			}
		}
		return n
	}
	assert.Equal(t, gates(plain)+1, gates(over))
	again, err := base.ListEnabledTransitions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, gates(plain), gates(again), "the stored rows are untouched")
}
