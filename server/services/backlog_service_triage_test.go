package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
)

// --- Story 2.1.2: rework_cap durable write (notifyReworkCapHit) ---

// TestNotifyReworkCapHit_should_markStuckReworkCapImmediately_When_CapHit
// verifies that hitting the rework cap writes a durable rework_cap stuck row
// (threshold 0 — the cap hit is a discrete, definitive event) with a
// cap-describing context, in addition to the existing notification.
func TestNotifyReworkCapHit_should_markStuckReworkCapImmediately_When_CapHit(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Rework cap test item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.notifyReworkCapHit(ctx, item.ID, item.Title, session.BacklogStatusReview, "after a failed review verdict")

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "rework cap")
	assert.NotNil(t, open[0].NotifiedAt, "dedup must be pre-set since the notification already fired")
}

// TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError
// verifies the durable write is additive, not a gate: when MarkStuck errors
// (forced here via an invalid item ID so the ent UUID parse fails), the
// operator notification must still publish — a storage hiccup must never
// silently suppress the cap-hit signal.
func TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	svc.notifyReworkCapHit(ctx, "not-a-valid-item-uuid", "Broken Item", session.BacklogStatusReview, "after a failed review verdict")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event even though MarkStuck errored")
	}
}

// TestNotifyReworkCapHit_should_persistRowSurvivingRestart_When_CapHit verifies
// the rework_cap row survives a simulated server restart (DB close/reopen
// from the same file) — the whole point of moving off the in-memory
// notify-once map (root cause #3).
func TestNotifyReworkCapHit_should_persistRowSurvivingRestart_When_CapHit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rework-cap-restart.db")

	var itemID string
	func() {
		repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()
		storage, err := session.NewStorageWithRepository(repo)
		require.NoError(t, err)

		item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
			Title:  "Restart-surviving rework cap item",
			Status: string(session.BacklogStatusPRPending),
		})
		require.NoError(t, err)
		itemID = item.ID

		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		svc.notifyReworkCapHit(context.Background(), itemID, item.Title, session.BacklogStatusPRPending, "while fixing PR #7")
	}()

	repo2, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	open, err := repo2.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, itemID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
}

// --- AutoReopenForPRFix: live-bug regression (ReconcilePRPending churn) ---

// TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn is the regression
// test for a live production incident: ReconcilePRPending calls AutoReopenForPRFix on
// every ~60s tick for any pr_pending item with failing CI, with no check for whether a
// fix is already in flight. When a work session was still genuinely active (a real
// multi-hour autonomous session, not dead), the old code transitioned pr_pending->
// in_progress, discovered SpawnSessionFromItem was blocked by that same active session,
// and rolled back to pr_pending — writing two BacklogStatusEvent rows every tick,
// forever, with zero progress. AutoReopenForPRFix must now check for an active work
// session FIRST and return early with no status transition at all.
func TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"active-work-uuid": true}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with an active fix session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 42,
		PrURL:    "https://github.com/example/repo/pull/42",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke")
	require.NoError(t, reopenErr, "must not error — this is the expected 'already in flight' outcome, not a failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status, "status must never churn while a fix session is already active")
	assert.Empty(t, creator.calls, "no new session should be spawned while one is already active")
}

// TestAutoReopenForPRFix_DeadWorkSession_TombstonesThenReopens verifies the other half:
// a work session that IS confirmed dead (not live) must be tombstoned automatically so
// the reopen can proceed normally, rather than blocking forever like the bug above.
func TestAutoReopenForPRFix_DeadWorkSession_TombstonesThenReopens(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}} // nothing is live
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with a dead prior session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 43,
		PrURL:    "https://github.com/example/repo/pull/43",
	})
	require.NoError(t, err)
	deadIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "dead-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke")
	require.NoError(t, reopenErr)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status, "reopen must proceed once the dead session is cleared")
	assert.Len(t, creator.calls, 1, "a new fix session must be spawned")

	deadFetched, err := storage.GetItemSession(context.Background(), deadIS.ID)
	require.NoError(t, err)
	assert.NotNil(t, deadFetched.EndedAt, "the dead session must be tombstoned")
}

// --- AutoRespawnReview: closes the "detected/notified but never respawned" gap ---
//
// Before this, markAbandonedReview (session/backlog_lifecycle.go) only wrote a
// stuck row and notified an operator — nothing ever re-triggered the review gate,
// so real backlog items sat stuck in review for days
// (docs/tasks/backlog-feature-improvement.md, 2026-07-17 update). AutoRespawnReview
// implements session.ReviewRespawner and is the mechanism markAbandonedReview now
// dispatches into.

// TestAutoRespawnReview_ReworkCapHit_LeavesInReviewAndNotifies is the regression
// test for the runaway-loop risk this fix introduces if left unbounded: unlike
// AutoReopenAfterFailedReview/AutoReopenForPRFix, AutoRespawnReview never adds a
// work session, so their work-session-counting cap would never trip here. Without
// its own cap on *review* sessions, an item whose underlying work is genuinely
// incomplete (verdict never PASSes) would re-review forever, once per
// abandoned_review occurrence. This asserts the cap — reusing the same
// maxAutoReworkIterations threshold and notifyReworkCapHit pattern as the other
// two rework loops — actually stops it.
func TestAutoRespawnReview_ReworkCapHit_LeavesInReviewAndNotifies(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Repeatedly-failing review item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// Seed maxAutoReworkIterations (3) prior, already-ended review sessions —
	// the shape left behind by 3 prior abandoned_review respawns that each ran
	// to completion without a PASS verdict.
	for i := 0; i < 3; i++ {
		is, isErr := storage.CreateItemSession(context.Background(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "prior-re-review-" + string(rune('a'+i)),
			SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, isErr)
		require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr, "hitting the cap is an expected outcome, not a failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "item must stay in review, not spin")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason, "cap hit must write the same durable rework_cap row the other two rework loops use")
}

// TestAutoRespawnReview_ActiveReviewSession_SkipsWithoutDoubleSpawn verifies
// AutoRespawnReview does not spawn a second, concurrent re-review pass when one
// is already running — the headless re-review path only records its ItemSession
// row after the LLM call completes, so a naive implementation could otherwise
// double-dispatch across two reconcile ticks landing close together.
func TestAutoRespawnReview_ActiveReviewSession_SkipsWithoutDoubleSpawn(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	// A headless pool being wired but never called proves no second attempt fired.
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"ok","verdicts":[]}`}
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Review item with an active re-review already running",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-review-uuid",
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr)
	assert.Empty(t, pool.calls, "must not start a second headless review call while one is already active")
}

// TestAutoRespawnReview_NoActiveSession_TriggersReReview verifies the success
// path: an abandoned review item with no active session gets a fresh review pass,
// and (mirroring TestTriggerReReview_HeadlessPassAutoTransitionsToDone) a PASS
// verdict from that respawned review carries the item all the way to done —
// proving the respawn is not just "detected," it actually unsticks the item.
func TestAutoRespawnReview_NoActiveSession_TriggersReReview(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("hello\n"), 0o644))
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"looks good","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"verified"}],"tool_reads":["README.md"]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	createResp, err := svc.CreateBacklogItem(context.Background(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "Abandoned review item",
		RepoPath: repoDir,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
		string(session.BacklogStatusReview),
	} {
		_, err = svc.TransitionBacklogItemStatus(context.Background(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), itemID)
	require.NoError(t, respawnErr)
	assert.NotEmpty(t, pool.calls, "must actually invoke the headless review call, not just detect the item")

	updated, err := svc.GetBacklogItem(context.Background(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), updated.Msg.Item.Status,
		"a PASS verdict from the respawned review should carry the item to done, same as a manual TriggerReReview call")
}

// --- AutoReopenForPRFix: proactive branch sync with main (Task 2.1.6d) ---
//
// Before AutoReopenForPRFix respawns a fix session, it now merges main into the
// worktree behind the currently open, failing PR — preventive sync rather than
// reactive (the PR #157 pattern: a branch drifted from main with nobody proactively
// resyncing it until it hit a hard conflict). These three tests cover the merge, the
// conflict, and the no-op cases via syncPRBranchWithMain.

// chmodRecursive sets mode on every entry under root (root included). A plain
// os.Chmod(root, mode) only affects root itself, which isn't enough to make a git
// remote's push targets (.git/objects/**, .git/refs/**) unwritable — git creates new
// files a few directories deep, and those subdirectories keep their own (writable)
// permissions unless changed individually.
func chmodRecursive(t *testing.T, root string, mode os.FileMode) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, mode)
	})
	require.NoError(t, err)
}

// runGitTestCmd runs a git command in dir and fails the test on error.
func runGitTestCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:norawexec // test helper, blocking CombinedOutput, no zombie risk
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}

// setupPRFixSyncRepo builds an "origin" repo and a clone of it (repoPath) with an
// "origin" remote already configured — mirroring production, where item.RepoPath is a
// checkout whose worktrees (git worktree add) share the same .git and inherit that
// remote. It returns the origin dir and the clone (repoPath), both with a branch
// explicitly named "main" (prFixMainBranch's target).
func setupPRFixSyncRepo(t *testing.T) (originDir, repoPath string) {
	t.Helper()
	originDir = t.TempDir()
	initGitRepoWithCommit(t, originDir)
	runGitTestCmd(t, originDir, "branch", "-M", "main")

	repoPath = filepath.Join(t.TempDir(), "clone")
	cloneCmd := exec.Command("git", "clone", originDir, repoPath) //nolint:norawexec // test helper
	out, err := cloneCmd.CombinedOutput()
	require.NoError(t, err, "git clone failed: %s", out)
	runGitTestCmd(t, repoPath, "config", "user.email", "test@example.com")
	runGitTestCmd(t, repoPath, "config", "user.name", "Test")
	return originDir, repoPath
}

// attachPRFixWorkSession records a completed work ItemSession for item plus the
// GitWorktreeData needed for syncPRBranchWithMain's GetWorktreeDataBySessionUUID
// lookup to find worktreePath — the worktree behind the currently open PR.
//
// Persists straight through repo.Create (the same low-level path
// session/ent_repository_test.go's TestEntRepository_Worktree uses) rather than going
// through session.FromInstanceData/storage.AddInstance: constructing a live *Instance
// pulls in gitManager/state-machine machinery that isn't needed here and, for a
// worktree directory git created directly (not through the app's own worktree
// registry), triggered an unrelated existence-check cleanup that deleted the very
// worktree this helper is trying to register.
func attachPRFixWorkSession(t *testing.T, storage *session.Storage, repo *session.EntRepository, item *session.BacklogItemData, sessionUUID, repoPath, worktreePath, branchName string) {
	t.Helper()
	is, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now().Add(-time.Hour)))

	baseCommitSHA := strings.TrimSpace(runGitTestCmd(t, worktreePath, "rev-parse", "HEAD"))
	now := time.Now()
	require.NoError(t, repo.Create(context.Background(), session.InstanceData{
		Title:      sessionUUID,
		UUID:       sessionUUID,
		Path:       worktreePath,
		WorkingDir: worktreePath,
		Branch:     branchName,
		Status:     session.Paused,
		Program:    "claude",
		CreatedAt:  now,
		UpdatedAt:  now,
		Worktree: session.GitWorktreeData{
			RepoPath:      repoPath,
			WorktreePath:  worktreePath,
			SessionName:   sessionUUID,
			BranchName:    branchName,
			BaseCommitSHA: baseCommitSHA,
		},
	}))
}

// createTestStorageWithRepo is createTestStorage but also returns the underlying
// *session.EntRepository, needed by attachPRFixWorkSession to persist worktree data
// via the low-level repo.Create path.
func createTestStorageWithRepo(t *testing.T) (*session.Storage, *session.EntRepository) {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage, repo
}

// TestAutoReopenForPRFix_should_MergeAndPushMain_When_BranchIsStaleButMergesCleanly
// verifies the preventive-sync path: a fix landed on main after the PR's branch was
// created (drift unrelated to the PR's own diff). AutoReopenForPRFix must merge main
// into the PR's branch, push the merge back to origin, and tell the spawned session it
// did so.
func TestAutoReopenForPRFix_should_MergeAndPushMain_When_BranchIsStaleButMergesCleanly(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-clean-merge"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// The PR's own work: an unrelated new file, committed on the PR branch.
	require.NoError(t, os.WriteFile(filepath.Join(workWT, "pr-work.txt"), []byte("pr work\n"), 0o644))
	runGitTestCmd(t, workWT, "add", "pr-work.txt")
	runGitTestCmd(t, workWT, "commit", "-m", "PR work")

	// A fix lands on main after the branch was created — the drift this sync is meant
	// to catch preventively.
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "main-fix.txt")
	runGitTestCmd(t, originDir, "commit", "-m", "fix landed on main")

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item that drifted from main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 201,
		PrURL:    "https://github.com/example/repo/pull/201",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "clean-merge-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
	require.Len(t, creator.calls, 1, "a new fix session must be spawned")
	assert.Contains(t, creator.calls[0].prompt, "[Branch sync]",
		"the spawned session's prompt must mention the branch sync outcome")
	assert.Contains(t, creator.calls[0].prompt, "pushed it",
		"a clean merge must be pushed, not just merged locally")

	// The push must have landed on origin: it must now have the PR branch, containing
	// both the PR's own work and the fix that had landed on main. (workWT itself is
	// gone by this point — SpawnSessionFromItem's reopen path cleans up the prior work
	// session's worktree once the new fix session is safely persisted; the merge's
	// staying power comes from having been pushed, not from the local worktree.)
	_, refErr := exec.Command("git", "-C", originDir, "rev-parse", "refs/heads/"+workBranch).CombinedOutput() //nolint:norawexec // test assertion
	require.NoError(t, refErr, "the merge must be pushed to the PR's branch on origin")
	_, prWorkErr := exec.Command("git", "-C", originDir, "cat-file", "-e", workBranch+":pr-work.txt").CombinedOutput() //nolint:norawexec // test assertion
	assert.NoError(t, prWorkErr, "pushed branch must still contain the PR's own work")
	_, mainFixErr := exec.Command("git", "-C", originDir, "cat-file", "-e", workBranch+":main-fix.txt").CombinedOutput() //nolint:norawexec // test assertion
	assert.NoError(t, mainFixErr, "pushed branch must contain the fix that had landed on main")
}

// TestAutoReopenForPRFix_should_IncludeConflictsInFixContext_When_MergingMainConflicts
// verifies the conflict path: when main and the PR's branch touch the same lines, the
// merge must be aborted (leaving the worktree clean and nothing pushed) and the
// conflicting file paths must be folded into the fix context handed to the spawned
// session, so resolving them against main becomes part of the fix.
func TestAutoReopenForPRFix_should_IncludeConflictsInFixContext_When_MergingMainConflicts(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-conflict"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// The PR branch edits README.md (created by initGitRepoWithCommit).
	require.NoError(t, os.WriteFile(filepath.Join(workWT, "README.md"), []byte("# PR Edit\n"), 0o644))
	runGitTestCmd(t, workWT, "add", "README.md")
	runGitTestCmd(t, workWT, "commit", "-m", "PR edits README")

	// Main edits the same line differently.
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "README.md")
	runGitTestCmd(t, originDir, "commit", "-m", "main edits README")

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item that conflicts with main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 202,
		PrURL:    "https://github.com/example/repo/pull/202",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "conflict-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: merge conflict risk")
	require.NoError(t, reopenErr)

	require.Len(t, creator.calls, 1, "a new fix session must be spawned even when the sync conflicts")
	assert.Contains(t, creator.calls[0].prompt, "conflicts", "the fix context must mention the merge conflict")
	assert.Contains(t, creator.calls[0].prompt, "README.md", "the fix context must name the conflicting file")

	// Nothing must have been pushed. (The worktree's own clean-abort behavior is
	// covered directly by session/git's TestMergeMainIntoWorktree_should_ReportConflictedAndAbort_*;
	// workWT itself is gone by this point — see the comment at the end of the clean-merge
	// test above for why.)
	_, refErr := exec.Command("git", "-C", originDir, "rev-parse", "refs/heads/"+workBranch).CombinedOutput() //nolint:norawexec // test assertion
	assert.Error(t, refErr, "a conflicted merge must never be pushed to origin")
}

// TestAutoReopenForPRFix_should_SkipSyncNote_When_BranchAlreadyUpToDateWithMain verifies
// the no-op case: when the PR's branch already contains everything on main, the sync
// must do nothing observable — no push, no "[Branch sync]" note cluttering the fix
// context handed to the spawned session.
func TestAutoReopenForPRFix_should_SkipSyncNote_When_BranchAlreadyUpToDateWithMain(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-up-to-date"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// No further commits anywhere — the branch already contains main's tip.

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item already in sync with main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 203,
		PrURL:    "https://github.com/example/repo/pull/203",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "up-to-date-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr)

	require.Len(t, creator.calls, 1)
	assert.NotContains(t, creator.calls[0].prompt, "[Branch sync]",
		"an already-synced branch must not add sync noise to the fix context")
}

// TestAutoReopenForPRFix_should_ReportUnpushedMerge_When_PushFails verifies the
// merge-succeeds-but-push-fails path: the merge must still happen locally, and the fix
// context must say the merge could not be pushed and give an explicit, actionable
// command against the shared repo checkout (not the worktree, which SpawnSessionFromItem
// deletes once the new fix session is spawned) — see syncPRBranchWithMain's push-error
// branch.
func TestAutoReopenForPRFix_should_ReportUnpushedMerge_When_PushFails(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-push-fails"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// A fix lands on main so there's something to merge (and therefore push).
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "main-fix.txt")
	runGitTestCmd(t, originDir, "commit", "-m", "fix landed on main")

	// Make origin's .git tree unwritable (recursively — a top-level chmod alone leaves
	// .git/objects and .git/refs writable) so the fetch+merge (read-only) still
	// succeeds but the subsequent push fails. Restored before t.TempDir() cleanup runs.
	originGitDir := filepath.Join(originDir, ".git")
	chmodRecursive(t, originGitDir, 0o555)
	t.Cleanup(func() { chmodRecursive(t, originGitDir, 0o755) })

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item whose merge can't be pushed",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 204,
		PrURL:    "https://github.com/example/repo/pull/204",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "push-fails-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr, "a push failure during sync must not block the fix spawn")

	require.Len(t, creator.calls, 1, "a new fix session must be spawned even when the sync's push fails")
	prompt := creator.calls[0].prompt
	assert.Contains(t, prompt, "could not push", "the fix context must say the merge could not be pushed")
	assert.Contains(t, prompt, workBranch, "the fix context must name the affected branch")
	assert.Contains(t, prompt, "git -C "+repoPath, "the fix context must give an actionable command against the shared repo checkout")
}

// TestAutoReopenForPRFix_should_SpawnNormally_When_SyncFetchFails verifies that a sync
// failure unrelated to the merge outcome (here: origin unreachable, so the fetch itself
// errors) is swallowed exactly like syncPRBranchWithMain's other best-effort failure
// paths — no "[Branch sync]" note, and the fix session is spawned normally rather than
// being blocked by a sync-layer problem.
func TestAutoReopenForPRFix_should_SpawnNormally_When_SyncFetchFails(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-fetch-fails"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// Break the "origin" remote so MergeMainIntoWorktree's fetch fails outright.
	runGitTestCmd(t, workWT, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item whose sync fetch fails",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 205,
		PrURL:    "https://github.com/example/repo/pull/205",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "fetch-fails-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr, "a sync fetch failure must not block the fix spawn")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
	require.Len(t, creator.calls, 1, "a new fix session must still be spawned when the sync's fetch fails")
	assert.NotContains(t, creator.calls[0].prompt, "[Branch sync]",
		"a swallowed sync error must not add a sync note to the fix context")
}

// --- Epic 1.6: ItemSessionSummary.PipelineModeSnapshot/SnapshotHash ---

// TestSpawnSessionFromItem_should_SnapshotResolvedModeSlugAndContentHash_When_SessionFirstStarts
// verifies that spawning a work session for an item with a non-default PipelineMode
// records BOTH the resolved mode slug and its content hash (as computed by
// PipelineEngine.ContentHashFor at that moment) onto the new ItemSession row — Story
// 1.6.2's core acceptance criterion. NOTE: svc.pipelineEngine is wired directly here
// (this package's test can reach the unexported field) since NewBacklogService's
// constructor does not yet accept a PipelineEngine parameter — that wiring is Epic
// 1.5's job. See the field's doc comment on BacklogService for the full rationale.
func TestSpawnSessionFromItem_should_SnapshotResolvedModeSlugAndContentHash_When_SessionFirstStarts(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "quick-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	wantHash, ok := engine.ContentHashFor(session.PipelineMode("quick"))
	require.True(t, ok, "setup: 'quick' must resolve to a content hash")
	require.NotEmpty(t, wantHash)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	sessions, err := storage.ListItemSessions(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "quick", sessions[0].PipelineModeSnapshot)
	assert.Equal(t, wantHash, sessions[0].PipelineModeSnapshotHash)
}

// TestSpawnSessionFromItem_should_SnapshotEmptyHash_When_PipelineModeIsDefaultOrUnresolved
// covers both zero-hash edge cases from Story 1.6.2's acceptance criteria: the default
// mode ("") short-circuits ContentHashFor without touching the cache, and an unresolved
// slug (absent from the cache) falls back via ContentHashFor's ok=false path — both
// must produce PipelineModeSnapshotHash == "", ignoring the ok bool per spec.
func TestSpawnSessionFromItem_should_SnapshotEmptyHash_When_PipelineModeIsDefaultOrUnresolved(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "quick-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	testCases := []struct {
		name         string
		pipelineMode *string // nil == omitted (default mode)
	}{
		{name: "default mode omitted", pipelineMode: nil},
		{name: "unresolved slug", pipelineMode: strPtr("missing-mode")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:        "item: " + tc.name,
				RepoPath:     repoPath,
				PipelineMode: tc.pipelineMode,
				AcceptanceCriteria: []*sessionv1.AcCriterion{
					{Index: 0, Text: "test", Status: "pending"},
				},
				SkipTriage:   true,
				SkipPlanning: true,
			}))
			require.NoError(t, err)
			itemID := createResp.Msg.Item.Id

			_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
				ItemId:       itemID,
				TargetStatus: "ready",
			}))
			require.NoError(t, err)

			_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
			require.NoError(t, err)

			sessions, err := storage.ListItemSessions(ctx, itemID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			assert.Empty(t, sessions[0].PipelineModeSnapshotHash)
		})
	}
}

// --- Epic 1.5: PipelineEngine wired into the 4 call sites ---

// readCommandFiles reads every file under worktreePath/.claude/commands/backlog/ into
// a name->content map, for comparing two independently-written slash-command sets.
func readCommandFiles(t *testing.T, worktreePath string) map[string]string {
	t.Helper()
	dir := filepath.Join(worktreePath, ".claude", "commands", "backlog")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		files[e.Name()] = string(data)
	}
	return files
}

// TestSpawnAndAttachSessionFromItem_should_ProduceIdenticalCommandFiles_When_SameItemAndModeUsedByBothCallers
// (Story 1.5.2) is the direct regression test for "2 independent WriteSlashCommands
// callers must not drift" (research/pitfalls.md §5 point 1): SpawnSessionFromItem and
// AttachSessionToItem both write .claude/commands/backlog/*.md for the same item, and
// both must go through the SAME shared PipelineEngine so a non-default PipelineMode's
// rendered content is identical regardless of which caller wrote it.
func TestSpawnAndAttachSessionFromItem_should_ProduceIdenticalCommandFiles_When_SameItemAndModeUsedByBothCallers(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Enabled:               true,
		StatusCommandTemplate: "status for {{item_title}} ({{item_id}})",
		DoneCommandTemplate:   "done {{criteria_index}}: {{criteria_text}}",
		FailCommandTemplate:   "fail {{criteria_index}}: {{criteria_text}}",
		ReviewCommandTemplate: "review {{item_title}}",
		ShipCommandTemplate:   "ship {{item_title}}",
		HelpCommandTemplate:   "help {{item_title}}",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode parity item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Caller 1: SpawnSessionFromItem writes into the worktree the mock creator reports.
	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	spawnPath := creator.calls[0].path

	// Caller 2: AttachSessionToItem writes into a second, independent directory.
	attachPath := t.TempDir()
	const attachUUID = "attach-parity-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "attach-target",
		UUID:  attachUUID,
		Path:  attachPath,
		// Paused (not Active) so LoadInstances doesn't attempt a real cold-restore
		// tmux/claude process start — see the identical comment in
		// TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions
		// (backlog_service_test.go).
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	_, err = svc.AttachSessionToItem(ctx, connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: attachUUID,
	}))
	require.NoError(t, err)

	spawnFiles := readCommandFiles(t, spawnPath)
	attachFiles := readCommandFiles(t, attachPath)
	assert.Equal(t, spawnFiles, attachFiles,
		"SpawnSessionFromItem and AttachSessionToItem must write byte-identical slash-command file sets for the same item+mode")
	// Sanity: the mode-specific content actually rendered (not the default set).
	assert.Contains(t, spawnFiles["status.md"], "status for quick-mode parity item")
}

// TestTriggerTriage_should_UseModeSpecificTriagePrompt_When_ItemHasNonDefaultPipelineModeAndFirstTriageBranch
// (Story 1.5.3) verifies the FIRST-triage (non-retriage) branch routes through
// PipelineEngine.TriagePromptFor: the LLM call receives the mode's rendered
// TriagePromptTemplate, not BuildHeadlessTriagePrompt's default boilerplate.
func TestTriggerTriage_should_UseModeSpecificTriagePrompt_When_ItemHasNonDefaultPipelineModeAndFirstTriageBranch(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(t.Context(), session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "QUICK MODE TRIAGE: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "quick-mode triage item",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     repoPath,
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, 5*time.Second, 50*time.Millisecond, "expected exactly one headless triage call")

	gotPrompt := pool.firstCall().userPrompt
	assert.Contains(t, gotPrompt, "QUICK MODE TRIAGE: quick-mode triage item",
		"expected the mode-specific rendered triage prompt, got: %s", gotPrompt)
	assert.NotContains(t, gotPrompt, "Perform pre-implementation triage",
		"sanity: the default BuildHeadlessTriagePrompt's boilerplate must not appear when a non-default mode is wired")
}

// TestTriggerTriage_should_UseUnmodifiedRetriagePrompt_When_RetriagingRegardlessOfPipelineMode
// (Story 1.5.3) proves the retriage (feedback-driven refine) branch stays on
// BuildHeadlessRetriagePrompt directly, even when item.PipelineMode is non-default with a
// custom TriagePromptTemplate — "refine the existing plan" is mode-independent
// (research/architecture.md §3), and this seam must not have accidentally routed it
// through PipelineEngine too.
func TestTriggerTriage_should_UseUnmodifiedRetriagePrompt_When_RetriagingRegardlessOfPipelineMode(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(t.Context(), session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "QUICK MODE TRIAGE: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "quick-mode refine item",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     t.TempDir(),
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	// Initial (first-triage) call — mode-specific, per the companion test above.
	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "initial triage should mark item ready")

	// Refine with feedback — the retriage branch under test.
	_, refineErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: "This missed the mobile case entirely.",
	}))
	require.NoError(t, refineErr)
	require.Eventually(t, func() bool {
		return pool.callCount() == 2
	}, 5*time.Second, 50*time.Millisecond, "refine should make a second headless call")

	retriagePrompt := pool.callAt(1).userPrompt
	assert.Contains(t, retriagePrompt, "## Prior triage result",
		"expected the default BuildHeadlessRetriagePrompt output, got: %s", retriagePrompt)
	assert.NotContains(t, retriagePrompt, "QUICK MODE TRIAGE:",
		"the retriage branch must NOT be routed through PipelineEngine even when PipelineMode is non-default")
}

// TestSpawnSessionFromItem_should_UseModeSpecificInitialPrompt_When_AutoSpawnSessionAndNonDefaultPipelineMode
// (Story 1.5.5) verifies inst.Prompt (and therefore NewAutonomousDriver's goal) contains
// the mode's rendered InitialPromptTemplate, not BuildTokenBudgetedPrompt's default
// output — proving this seam is not cosmetic for autonomous-mode sessions.
func TestSpawnSessionFromItem_should_UseModeSpecificInitialPrompt_When_AutoSpawnSessionAndNonDefaultPipelineMode(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Enabled:               true,
		InitialPromptTemplate: "QUICK MODE INITIAL PROMPT: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode autonomous item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	gotPrompt := creator.calls[0].prompt
	assert.Contains(t, gotPrompt, "QUICK MODE INITIAL PROMPT: quick-mode autonomous item",
		"expected the mode-specific rendered initial prompt, got: %s", gotPrompt)
}

// TestSpawnSessionFromItem_should_UseDefaultInitialPrompt_When_PipelineModeIsDefault
// (Story 1.5.5) is the zero-regression companion: default mode still produces
// BuildTokenBudgetedPrompt's unmodified output.
func TestSpawnSessionFromItem_should_UseDefaultInitialPrompt_When_PipelineModeIsDefault(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "default-mode item",
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Snapshot the item's pre-spawn state (status "ready") — SpawnSessionFromItem
	// transitions it to "in_progress" as its LAST step, after the prompt has already
	// been built, so re-fetching AFTER the call would reconstruct a different
	// (post-transition) BuildTokenBudgetedPrompt rendering than what was actually sent.
	preSpawn, err := storage.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)

	wantPrompt := session.BuildTokenBudgetedPrompt(preSpawn, nil)
	assert.Equal(t, wantPrompt, creator.calls[0].prompt,
		"default PipelineMode must still produce BuildTokenBudgetedPrompt's unmodified output")
}
