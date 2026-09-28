package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// TestCreateSession_MainCheckoutBranchCollision is Epic 1.1 Story 1.1.1's
// controlled repro for Hypothesis #2 (project_plans/worktree-envvars-hijack/
// research/architecture.md, its Q3 section): a branch already checked out in
// a repo's MAIN checkout (not a linked worktree) is invisible to
// findLiveWorktreeForBranch's self-heal (session/git/worktree_ops.go), which
// only scans .git/worktrees/ admin dirs -- never the main checkout. The
// static trace predicts this makes `git worktree add` fail and propagate a
// real error, not silently succeed against the bare repo path. This test
// exercises the real CreateSession -> Background Resolution Pipeline ->
// Instance.Start() -> setupFirstTimeWorktree chain to confirm or refute that
// prediction empirically instead of by static reading alone.
//
// Given a bare repo at repoDir with a live, non-terminal instance whose
// actual git HEAD is "collide-branch" (checked out in the repo's own working
// tree, not a linked worktree), When CreateSession is called with
// SessionType: SESSION_TYPE_NEW_WORKTREE, Branch: "collide-branch" against
// the same repoDir, Then the resulting session must reach either:
//   - Failed, with a FailureReason and a captured creation-progress phase
//     naming the worktree/branch conflict (Hypothesis #2 REFUTED -- matches
//     the static prediction), or
//   - Active, with inst.Workspace().ActiveDir == repoDir (Hypothesis #2
//     CONFIRMED -- a real, previously-unknown silent-swallow gap).
func TestCreateSession_MainCheckoutBranchCollision(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	wireRegistryForActorSerialization(fix)

	repoDir := t.TempDir()
	initGitRepoWithCommit(t, repoDir)

	// Actually check out a NEW branch "collide-branch" in the repo's own
	// (main) working tree -- not a linked worktree -- so the collision is a
	// real on-disk git-state fact, mirroring what a live SessionTypeDirectory
	// session's tmux pane would have left behind by running `git checkout -b
	// collide-branch` there.
	repo, err := git.PlainOpen(repoDir)
	require.NoError(t, err)
	headRef, err := repo.Head()
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, wt.Checkout(&git.CheckoutOptions{
		Hash:   headRef.Hash(),
		Branch: plumbing.NewBranchReferenceName("collide-branch"),
		Create: true,
	}))

	// Seed a live, non-terminal Instance representing that main-checkout
	// session (Story 1.1.1's Given clause). Nothing in the pre-Epic-3.3
	// CreateSession path consults this record's liveness -- the collision
	// this test proves/refutes is purely a git-level fact on disk -- but the
	// acceptance criteria's scenario names a live occupant explicitly, so it
	// is seeded for fidelity to the reported bug shape.
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "epic11-collision-main-checkout",
		Path:    repoDir,
		Branch:  "collide-branch",
		Program: "sh",
		Status:  session.Active,
	}))

	// Capture creation-progress phase text (same seam
	// TestCreateSession_should_ReachActiveViaPipeline's ModeIsRestart subtest
	// uses): the pipeline's FailureReason is always the fixed label
	// "StartupError" (session_creation_pipeline.go), never the underlying
	// error text, so the phase hook is the only way to observe whether a
	// Failed outcome actually names the worktree/branch conflict versus some
	// unrelated startup failure.
	var mu sync.Mutex
	var phases []string
	fix.svc.creationPhaseHook = func(msg string) {
		mu.Lock()
		phases = append(phases, msg)
		mu.Unlock()
	}

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "epic11-collision-new-worktree",
		Path:        repoDir,
		Branch:      "collide-branch",
		Program:     "sh",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
	}))
	require.NoError(t, err, "CreateSession's synchronous prefix must accept the request (the collision is only detectable once the pipeline attempts the real git worktree add)")
	id := resp.Msg.Session.Id
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, id) })

	outcome, err := fix.svc.awaitCreationTerminal(context.Background(), id, 30*time.Second, 20*time.Millisecond)
	require.NoError(t, err, "AwaitCreationTerminal must not error before the pipeline reaches a terminal status")

	mu.Lock()
	capturedPhases := append([]string(nil), phases...)
	mu.Unlock()

	switch outcome.Status {
	case session.Failed:
		// Hypothesis #2 REFUTED (for its "silent success" half): the
		// collision propagates loudly, matching architecture.md's static
		// prediction.
		t.Logf("Hypothesis #2 REFUTED: session reached Failed, FailureReason=%q, phases=%v", outcome.FailureReason, capturedPhases)
		assert.Equal(t, "StartupError", outcome.FailureReason)

		foundConflictPhase := false
		for _, p := range capturedPhases {
			if strings.Contains(p, "worktree") || strings.Contains(p, "collide-branch") {
				foundConflictPhase = true
				break
			}
		}
		assert.True(t, foundConflictPhase,
			"expected a captured creation-progress phase naming the worktree/branch conflict, got phases=%v", capturedPhases)
	case session.Active:
		// Hypothesis #2 CONFIRMED: a real, previously-unknown silent-swallow
		// gap -- the new session's worktree setup silently landed in the
		// bare repo path instead of failing or creating a genuinely isolated
		// worktree.
		inst := fix.svc.FindLiveInstance(id)
		require.NotNil(t, inst, "an Active session must be resolvable via FindLiveInstance")
		ws := inst.Workspace()
		t.Logf("Hypothesis #2 CONFIRMED: session reached Active with ActiveDir=%q (repoDir=%q)", ws.ActiveDir, repoDir)
		assert.Equal(t, repoDir, ws.ActiveDir,
			"Hypothesis #2 predicts the silently-swallowed session's ActiveDir collapses to the bare repo path")
	default:
		t.Fatalf("session reached unexpected non-terminal-for-this-test status %v (failure_reason=%q, phases=%v)",
			outcome.Status, outcome.FailureReason, capturedPhases)
	}
}
