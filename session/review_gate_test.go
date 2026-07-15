package session

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// TestReviewGateRunner_SkipReviewGate verifies that Run returns immediately
// without calling the pool or onPass when item.SkipReviewGate is true.
func TestReviewGateRunner_SkipReviewGate(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Construct an item with SkipReviewGate set — pool and onPass must not be called.
	item := &BacklogItemData{
		ID:             uuid.New().String(),
		RepoPath:       "/some/repo",
		SkipReviewGate: true,
	}
	is := ItemSessionSummary{
		ID:          uuid.New().String(),
		SessionUUID: uuid.New().String(),
	}

	var poolCalled atomic.Bool
	var onPassCalled atomic.Bool

	// If the pool is consulted, panic so the test fails loudly.
	getPool := func() *headless.Pool {
		poolCalled.Store(true)
		return nil
	}
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)

	runner.Run(context.Background(), item, is, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, poolCalled.Load(), "pool getter must not be consulted when SkipReviewGate is true")
	assert.False(t, onPassCalled.Load(), "onPass must not be called when SkipReviewGate is true")
}

// TestReviewGateRunner_HeadlessPassPath verifies the happy path where the headless
// pool returns a PASS verdict: onPass is called and a review ItemSession with a
// PASS verdict is persisted to storage.
func TestReviewGateRunner_HeadlessPassPath(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Persist a BacklogItem so CreateItemSessionWithVerdict can satisfy its FK.
	itemData := BacklogItemData{
		Title:              "Headless Pass Test",
		Description:        "Testing the headless PASS path",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           newNonEmptyDiffGitRepo(t), // real repo/diff — empty diff now routes through the codebase-read/WorkDir path (Story 2.2.2), which needs a real *ProcessRunner rather than the FakeRunner this test uses
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Persist a work ItemSession so the runner can look it up.
	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:       createdItemData.ID,
		RepoPath: createdItemData.RepoPath,
	}

	// Build the JSON response expected by pool.CallBlocking.
	// The outer envelope is firstCallJSONResult; its "result" field contains the
	// verdict JSON that ParseHeadlessVerdictResult will parse.
	verdictJSON := `{"overall":"PASS","summary":"all criteria met","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)

	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.True(t, onPassCalled.Load(), "onPass must be called when headless pool returns PASS")
	assert.Equal(t, 1, fakeRunner.CallCount(), "pool must be called exactly once")
}

// TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt verifies that verification
// evidence recorded on the work session (via request_review's verification_notes
// argument) reaches the headless reviewer prompt, not just the diff and AC list.
// This is the regression guard for the UNVERIFIABLE-despite-real-verification gap:
// criteria describing test runs or manual UI checks are invisible in the diff, so the
// reviewer's only window into that evidence is this threaded-through text.
func TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	itemData := BacklogItemData{
		Title:              "Verification Notes Threading Test",
		Description:        "Testing that verification_notes reaches the reviewer prompt",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           newNonEmptyDiffGitRepo(t),
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	verificationNotes := "ran `go test ./session/...` -> ok (41 tests); confirmed via UI that sessions group under Category=Backlog"

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:            createdItemData.ID,
		SessionUUID:       workSessionUUID,
		SessionRole:       SessionRoleWork,
		VerificationNotes: verificationNotes,
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:       createdItemData.ID,
		RepoPath: createdItemData.RepoPath,
	}

	verdictJSON := `{"overall":"PASS","summary":"all criteria met","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)

	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, fakeRunner.CallCount(), "pool must be called exactly once")

	// The user prompt is passed via stdin (not args) so it doesn't leak into
	// /proc/<pid>/cmdline — see Pool.call in headless/caller.go.
	prompt := fakeRunner.StdinForCall(0)
	assert.True(t,
		strings.Contains(prompt, "Verification Evidence") && strings.Contains(prompt, "Category=Backlog"),
		"reviewer prompt must contain the labeled Verification Evidence section with the work session's reported notes; got prompt: %s", prompt)
}

// TestReviewGateRunner_MergesLiveCriterionNoteIntoStalePromptWhenSnapshotPredatesIt
// is a regression test for AC-snapshot staleness: the work session's AcSnapshot is
// captured at spawn time, but report_progress can write a Note onto the live item's
// AcceptanceCriteria afterward. Without merging the live Note into the snapshot, the
// reviewer never sees it. This constructs an is.AcSnapshot without a note and an
// item.AcceptanceCriteria with one, and asserts the note text reaches the prompt
// sent to the fake pool.
func TestReviewGateRunner_MergesLiveCriterionNoteIntoStalePromptWhenSnapshotPredatesIt(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	staleSnapshot := `[{"index":0,"text":"Do the thing","status":"pending"}]`
	liveNote := "finished implementing in foo.go, all tests pass"
	liveAC := fmt.Sprintf(`[{"index":0,"text":"Do the thing","status":"done","note":%q}]`, liveNote)

	itemData := BacklogItemData{
		Title:              "AC Snapshot Staleness Test",
		Description:        "Testing that a live Note reaches the reviewer prompt despite a stale snapshot",
		AcceptanceCriteria: AcCriteriaJSON(liveAC),
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           newNonEmptyDiffGitRepo(t),
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
		AcSnapshot:  AcCriteriaJSON(staleSnapshot),
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:                 createdItemData.ID,
		RepoPath:           createdItemData.RepoPath,
		AcceptanceCriteria: AcCriteriaJSON(liveAC),
	}

	verdictJSON := `{"overall":"PASS","summary":"all criteria met","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)

	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, fakeRunner.CallCount(), "pool must be called exactly once")

	prompt := fakeRunner.StdinForCall(0)
	assert.Contains(t, prompt, liveNote,
		"reviewer prompt must contain the live Note merged from item.AcceptanceCriteria even though it postdates the ItemSession's AcSnapshot")
}

// fakeAutoReopenSpawner is a test double implementing AutoReopenSpawner, recording
// every call and signaling on a channel so async callers (spawnReviewGate invokes
// it in a goroutine) can be synchronized with in tests.
type fakeAutoReopenSpawner struct {
	called chan string // item IDs, one per call
}

func newFakeAutoReopenSpawner() *fakeAutoReopenSpawner {
	return &fakeAutoReopenSpawner{called: make(chan string, 8)}
}

func (f *fakeAutoReopenSpawner) AutoReopenAfterFailedReview(ctx context.Context, itemID string) error {
	f.called <- itemID
	return nil
}

// runGitOrFail runs a git command in dir and fails the test on error, for building
// the small real repo fixtures below.
func runGitOrFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
}

// forceEmptyBranchNameViaRawSQL forces the branch_name column of the worktrees row
// for sessionName to ” via a raw SQL UPDATE against storage's own on-disk SQLite
// file, bypassing the ent Worktree schema's Go-level NotEmpty() validator (which
// makes it impossible to persist an empty BranchName through the normal
// Storage/SaveInstances path). The worktrees.branch_name column itself is only
// NOT NULL (no length CHECK) at the SQL level, so this reproduces data that predates
// the validator, or was written by some path other than the ent ORM — used by
// TestReviewGateRunner_NoBranchName_DiffComputationFailure_SkipsRepairAndBlocksImmediately
// to exercise the `wt.BranchName != ""` repair-skip guard in ReviewGateRunner.Run.
func forceEmptyBranchNameViaRawSQL(t *testing.T, storage *Storage, sessionName string) {
	t.Helper()
	er, ok := storage.repo.(*EntRepository)
	require.True(t, ok, "forceEmptyBranchNameViaRawSQL requires an EntRepository-backed Storage")

	db, err := sql.Open("sqlite3", er.dbPath)
	require.NoError(t, err)
	defer db.Close()

	res, err := db.Exec("UPDATE worktrees SET branch_name = '' WHERE session_name = ?", sessionName)
	require.NoError(t, err)
	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected, "expected exactly one worktrees row for session_name=%q", sessionName)
}

// newNonEmptyDiffGitRepo creates a small real git repo with two commits (an initial
// commit plus a second one adding change.txt), so GetGitDiff's implicit HEAD~1..HEAD
// range produces a real, non-empty diff. Tests that only care about the headless
// FakeRunner call/prompt mechanics (not diff content) use this instead of a non-git
// tempdir now that an empty diff routes through the codebase-read/WorkDir path (Story
// 2.2.2), which requires a real *ProcessRunner rather than a FakeRunner.
func newNonEmptyDiffGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "change.txt"), []byte("change\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "change.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "add change")
	return repoDir
}

// newEmptyDiffGitRepo creates a small real git repo with a single commit and returns
// both the repo directory and that commit's SHA. Tests that need a legitimately EMPTY
// diff (as opposed to a diff-computation ERROR) on the no-worktree-recorded path use
// this instead of a non-git tempdir: set the returned SHA as the work ItemSession's
// LastCommitSha, so ReviewGateRunner.Run's no-worktree branch (GetGitDiff(item.RepoPath,
// is.LastCommitSha)) diffs headSHA..HEAD — the same commit against itself, legitimately
// empty rather than erroring. A non-git tempdir can no longer be used for this: Run now
// treats a diff-computation ERROR on that branch as an infrastructure failure (assigned
// to worktreeDiffErr) rather than silently falling through to the codebase-read path —
// see the fix for the "dangling session_uuid" bug in ReviewGateRunner.Run.
func newEmptyDiffGitRepo(t *testing.T) (repoDir, headSHA string) {
	t.Helper()
	repoDir = t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")
	sha, shaErr := GetGitHeadSHA(repoDir)
	require.NoError(t, shaErr)
	return repoDir, sha
}

// TestReviewGateRunner_DiffComputationFailure_BlocksReviewInsteadOfFalseUnverifiable
// is a regression test for a live-data bug: when a session's worktree exists but its
// recorded base_commit_sha points at a git object that no longer exists (e.g. pruned,
// corrupted, or copied from elsewhere), GetGitDiff and its GetGitDiffRef fallback both
// error out. The runner used to silently swallow both errors and proceed to call the
// reviewer with an empty diff, producing a false UNVERIFIABLE/FAIL verdict indistinguishable
// from "no changes were made" — masking a real infrastructure bug and, via the
// auto-reopen loop, causing the item to spin in review forever. The fix blocks the
// review entirely, records a distinct FAIL verdict describing the diff failure, notifies,
// and still feeds the auto-reopen/cap machinery so persistent failures eventually reach
// notifyReworkCapHit instead of looping silently.
func TestReviewGateRunner_DiffComputationFailure_BlocksReviewInsteadOfFalseUnverifiable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Real git repo with one commit so `git diff <bogus-sha>..HEAD` fails with a real
	// "unknown revision" error rather than "not a git repository". The branch is
	// explicitly pinned to "main" (rather than relying on the host's
	// init.defaultBranch config, as other tests in this file also avoid) so that the
	// worktree's recorded BranchName below ("master") deterministically does NOT
	// match any real branch in this repo — this drives RecoverBaseCommitSHA's `git
	// merge-base HEAD master` to fail outright (unknown revision), exercising the
	// "auto-repair itself cannot resolve the branch" sub-branch of the repair logic,
	// distinct from TestReviewGateRunner_DiffComputationFailure_RecoveredButEmptyDiff_StillBlocks's
	// "repair resolves but the recovered diff is empty" sub-branch below.
	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(repoDir+"/file.txt", []byte("hello\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "file.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "initial commit")

	itemData := BacklogItemData{
		Title:              "Diff compute failure test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// Record a worktree for the work session whose base_commit_sha is a well-formed but
	// nonexistent SHA — this is what a pruned/corrupted commit looks like in practice.
	inst := newTestInstance("diff-error-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, repoDir, "diff-error-test", "master", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	// The pool must never be consulted — the review should be blocked before reaching it.
	fakeRunner := headless.NewFakeRunner(`{"session_id":"unused","result":"unused","cost_usd":0}`)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)
	getPool := func() *headless.Pool { return pool }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, getNotifier, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, fakeRunner.CallCount(), "reviewer pool must not be called when the diff could not be computed")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a distinct FAIL verdict must be recorded, not a silent pass-through")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the cap/notify machinery eventually engages")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_NoWorktreeRecorded_DiffComputationFailure_BlocksReview is a
// regression test for the "dangling session_uuid" case: no worktree row exists for
// this session at all (directory-mode session, or the worktree row was pruned), so
// Run takes the else branch of the worktree/no-worktree diff-computation split and
// calls GetGitDiff(item.RepoPath, is.LastCommitSha) directly. Before the fix, that
// branch's error was only logged, never assigned to worktreeDiffErr — so a genuine
// diff-computation failure here (as opposed to "no worktree, no changes by design")
// was silently indistinguishable from a legitimate empty diff and fell through to the
// codebase-read fallback instead of the block-and-notify path. This constructs that
// exact shape: no worktree recorded (SaveInstances is never called for this session),
// and a LastCommitSha that is well-formed but does not resolve, so `git diff
// <bogus>..HEAD` genuinely errors rather than returning an empty diff. Since no
// worktree means wt.BranchName == "", auto-repair is never attempted (see
// TestReviewGateRunner_NoBranchName_DiffComputationFailure_SkipsRepairAndBlocksImmediately
// for the dedicated regression test of that skip condition) and the failure must fall
// straight through to the same block-and-notify path as the worktree-present case.
func TestReviewGateRunner_NoWorktreeRecorded_DiffComputationFailure_BlocksReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(repoDir+"/file.txt", []byte("hello\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "file.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "initial commit")

	itemData := BacklogItemData{
		Title:              "No worktree recorded, diff compute failure test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	// A well-formed but nonexistent SHA, and — critically — no SaveInstances call, so
	// GetWorktreeDataBySessionUUID finds nothing and Run takes the no-worktree branch.
	workIS.LastCommitSha = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	fakeRunner := headless.NewFakeRunner(`{"session_id":"unused","result":"unused","cost_usd":0}`)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)
	getPool := func() *headless.Pool { return pool }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, getNotifier, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, fakeRunner.CallCount(), "reviewer pool must not be called when the diff could not be computed")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a genuine diff-computation failure on the no-worktree path must block with FAIL, not silently fall through to codebase-read")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the cap/notify machinery eventually engages")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_DiffComputationFailure_AutoRepairsFromDivergentBranch is the
// positive counterpart to the blocking test above: it reproduces the exact live-data
// shape of backlog item ae1e2070-db02-4ad7-8580-633ef9904f31 — a real feature branch
// with real committed work, whose recorded base_commit_sha is a well-formed but
// nonexistent SHA (simulating a pruned/corrupted commit) — and verifies the review
// proceeds on the recovered (real) diff instead of blocking, because repoPath's
// checked-out HEAD ("main") and the work branch ("feature") have a genuine common
// ancestor that RecoverBaseCommitSHA can find and a non-empty diff results.
func TestReviewGateRunner_DiffComputationFailure_AutoRepairsFromDivergentBranch(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")

	// Real feature branch with real committed work — the same shape as ae1e2070's
	// stelekit worktree, which had a genuine 302+/88- fix already committed.
	runGitOrFail(t, repoDir, "branch", "feature")
	runGitOrFail(t, repoDir, "checkout", "feature")
	require.NoError(t, os.WriteFile(repoDir+"/feature.txt", []byte("real work\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "feature.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "real fix")
	runGitOrFail(t, repoDir, "checkout", "main")

	itemData := BacklogItemData{
		Title:              "Diff auto-repair test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// Same corruption as the blocking test: a well-formed but nonexistent base SHA.
	// worktreePath == repoDir keeps the fixture simple; GetGitDiff(worktree) fails the
	// same way GetGitDiffRef(repo fallback) does, exactly as observed for ae1e2070.
	inst := newTestInstance("diff-repair-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, repoDir, "diff-repair-test", "feature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	verdictJSON := `{"overall":"PASS","summary":"all good","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)
	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)
	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, getNotifier, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	require.Equal(t, 1, fakeRunner.CallCount(), "the reviewer must actually be called on the recovered diff instead of the review being blocked")
	assert.True(t, onPassCalled.Load(), "onPass must fire — the recovered diff contains real, reviewable work")

	prompt := fakeRunner.StdinForCall(0)
	assert.Contains(t, prompt, "feature.txt", "the reviewer prompt must contain the real diff recovered via merge-base, not an empty one")

	assert.NotContains(t, notifier.titles(), "Review blocked — diff computation failed", "the review must not be reported as blocked when auto-repair succeeded")
	assert.Contains(t, notifier.titles(), "Review auto-repaired a broken diff", "the operator must still be told a repair happened, since the stored base_commit_sha is still wrong")
}

// TestReviewGateRunner_DiffComputationFailure_RecoveredButEmptyDiff_StillBlocks is a
// regression test for the auto-repair safety guard: RecoverBaseCommitSHA can
// successfully resolve a merge-base, but if that recovered base produces an EMPTY
// diff (e.g. because the branch never diverged from repoPath's checked-out HEAD in
// the first place), the recovery must NOT be treated as successful — an empty
// "recovered" diff is exactly as unsafe to hand the reviewer as the original failure
// (see the comment on that branch in ReviewGateRunner.Run). This is the exact bug
// class the repair feature exists to prevent, and previously had zero direct test
// coverage. The fixture here is deliberately single-branch (only "main", no divergent
// "feature" branch as in the AutoRepairsFromDivergentBranch test above): the worktree's
// recorded BranchName ("main") matches the repo's one real branch, so
// RecoverBaseCommitSHA's merge-base lookup trivially succeeds and returns HEAD itself
// — and diffing HEAD against HEAD is empty by construction.
func TestReviewGateRunner_DiffComputationFailure_RecoveredButEmptyDiff_StillBlocks(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")
	// Deliberately no divergent branch: "main" is the only branch and repoPath's own
	// checked-out HEAD already sits on it, so merge-base(HEAD, "main") collapses to
	// HEAD itself and the "recovered" diff against it is empty.

	itemData := BacklogItemData{
		Title:              "Diff auto-repair recovered-but-empty test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// Same corruption shape as the other diff-computation-failure tests: a well-formed
	// but nonexistent base SHA, forcing GetGitDiff/GetGitDiffRef to fail and trigger
	// the auto-repair attempt below.
	inst := newTestInstance("diff-repair-empty-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, repoDir, "diff-repair-empty-test", "main", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	// The pool must never be consulted — a recovered-but-empty diff must still block
	// before ever reaching the reviewer.
	fakeRunner := headless.NewFakeRunner(`{"session_id":"unused","result":"unused","cost_usd":0}`)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)
	getPool := func() *headless.Pool { return pool }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, getNotifier, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, fakeRunner.CallCount(), "reviewer pool must never be called when the recovered diff is still empty")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a recovered-but-empty diff must be recorded as FAIL, not UNVERIFIABLE and not PASS")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")
	assert.NotContains(t, notifier.titles(), "Review auto-repaired a broken diff", "an untrusted (empty) recovery must never be reported to the operator as a successful repair")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the cap/notify machinery eventually engages")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_NoBranchName_DiffComputationFailure_SkipsRepairAndBlocksImmediately
// is a regression test for the `wt.BranchName != ""` guard on the auto-repair attempt:
// a worktree record with no BranchName (directory-mode/branchless sessions) has no
// branch to recompute a merge-base against, so repair must be skipped entirely rather
// than calling RecoverBaseCommitSHA with an empty ref (which would itself error, but
// for the wrong reason, and would show up as a RecoverBaseCommitSHA log line this test
// asserts never appears). The review must still fall straight through to the
// block-and-notify path.
func TestReviewGateRunner_NoBranchName_DiffComputationFailure_SkipsRepairAndBlocksImmediately(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(repoDir+"/file.txt", []byte("hello\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "file.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "initial commit")

	itemData := BacklogItemData{
		Title:              "No branch name, diff compute failure test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// A worktree IS recorded (so Run takes the worktree branch, not the no-worktree
	// branch covered by TestReviewGateRunner_NoWorktreeRecorded_DiffComputationFailure_BlocksReview),
	// but its BranchName is empty — directory-mode/branchless sessions have no branch
	// to recompute a merge-base against.
	//
	// The ent Worktree schema's branch_name field has a Go-level NotEmpty() validator
	// (session/ent/schema/worktree.go), so SaveInstances cannot persist an empty
	// BranchName directly — it would silently fail to create the worktree row at all
	// (best-effort logging, no error returned), which would make this fixture
	// indistinguishable from the no-worktree-recorded case above rather than exercising
	// this guard. The SQLite column itself is only NOT NULL (no length CHECK), so a
	// normal worktree row is created first and then branch_name is forced to '' via a
	// raw SQL UPDATE against the same on-disk DB file, bypassing the Go-level validator
	// entirely to reproduce data that predates it (or was written by a path other than
	// the ent ORM) — exactly the shape this guard exists to defend against.
	inst := newTestInstance("diff-error-no-branch-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, repoDir, "diff-error-no-branch-test", "placeholder-branch", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	forceEmptyBranchNameViaRawSQL(t, storage, "diff-error-no-branch-test")

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	fakeRunner := headless.NewFakeRunner(`{"session_id":"unused","result":"unused","cost_usd":0}`)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)
	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, getNotifier, nil)

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)
	var warnBuf bytes.Buffer
	origWarning := log.WarningLog
	log.WarningLog = stdlog.New(&warnBuf, "WARNING: ", 0)
	t.Cleanup(func() { log.WarningLog = origWarning })

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, fakeRunner.CallCount(), "reviewer pool must not be called when the diff could not be computed")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")
	assert.NotContains(t, warnBuf.String(), "RecoverBaseCommitSHA", "repair must never be attempted when the worktree has no BranchName")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a distinct FAIL verdict must be recorded, not a silent pass-through")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")
}

// ─── Story 2.2.2/2.2.4: BuildReviewCallOptions wiring + timeout degrade ──────

// writeOccupyAwareFakeClaudeScript writes a fake `claude` binary that reads stdin and
// — if the stdin content contains "OCCUPY" — sleeps long enough to hold the pool's
// concurrency semaphore for the duration of a test. Otherwise it immediately emits
// outerJSON (a firstCallJSONResult envelope) to stdout.
func writeOccupyAwareFakeClaudeScript(t *testing.T, scriptDir, outerJSON string) string {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf(`#!/bin/sh
input="$(cat)"
case "$input" in
  *OCCUPY*) sleep 30 ;;
esac
cat <<'HEADLESSTESTEOF'
%s
HEADLESSTESTEOF
`, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

// occupyPoolSemaphore holds pool's single concurrency slot for the lifetime of
// occupyCtx by making a real call through it (stdin payload "OCCUPY", matched by
// writeOccupyAwareFakeClaudeScript). Pool.call acquires the concurrency semaphore
// synchronously before returning (session/headless/caller.go), so by the time this
// function returns the slot is guaranteed held — no polling needed. A subsequent call
// through the same pool therefore deterministically observes its own context's Done()
// case in the semaphore-acquire select, instead of racing against an always-ready
// buffered-channel send.
func occupyPoolSemaphore(t *testing.T, pool *headless.Pool, occupyCtx context.Context) {
	t.Helper()
	ch, callErr := pool.Call(occupyCtx, "occupy-key", "sys", "OCCUPY")
	require.NoError(t, callErr, "occupying call must acquire the semaphore synchronously")
	go func() {
		for range ch { //nolint:revive // draining is the point
		}
	}()
}

// shortRealTimeoutCtx returns a context with a short (but real, non-zero) deadline, for
// tests that need a genuine, natural context.DeadlineExceeded without waiting anywhere
// close to the real CodebaseReadCallTimeout (600s) or DefaultCallTimeout (900s)
// durations that production code actually uses (those exact durations are covered by
// TestBuildReviewCallOptions_* in backlog_review_test.go).
func shortRealTimeoutCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 300*time.Millisecond)
}

// TestReviewGateRunner_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt verifies that an
// empty-diff review call is routed through BuildReviewCallOptions' codebase-access
// branch: the real subprocess runs with cwd set to the codebase work dir and receives
// --allowedTools/--permission-mode flags scoped to read-only access (ADR-001).
func TestReviewGateRunner_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// A real git repo (not a non-git tempdir) with LastCommitSha pinned to the
	// current HEAD, so GetGitDiff legitimately computes an EMPTY diff (HEAD..HEAD)
	// instead of erroring — a non-git dir would error here too (see Fix for the
	// "no worktree recorded" branch in ReviewGateRunner.Run, which now assigns that
	// error to worktreeDiffErr and routes it to the block-and-notify path instead of
	// silently falling through to this codebase-read path, exactly the behavior this
	// test wants to exercise on a genuinely empty diff, not a diff computation error).
	repoDir, headSHA := newEmptyDiffGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	itemData := BacklogItemData{
		Title:              "Codebase Access Wiring Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — falls back to GetGitDiff(item.RepoPath, LastCommitSha); pinning it to HEAD makes the diff legitimately empty rather than erroring

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	argsPath := filepath.Join(scriptDir, "args.txt")
	pwdPath := filepath.Join(scriptDir, "pwd.txt")
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)

	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho \"$@\" > %s\npwd > %s\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", argsPath, pwdPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Bypass the Story 2.2.6 capability self-check: it would otherwise consume the
	// fake script's single scripted response for its own marker-file smoke test
	// before the real review call this test is asserting on ever runs.
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	argsBytes, readErr := os.ReadFile(argsPath)
	require.NoError(t, readErr, "fake-claude.sh must have been invoked")
	argsStr := string(argsBytes)
	assert.Contains(t, argsStr, "--allowedTools")
	assert.Contains(t, argsStr, "Read,Grep,Glob")
	assert.Contains(t, argsStr, "--permission-mode")
	assert.Contains(t, argsStr, PermissionModeBypassPermissions)

	resolvedRepoDir, evalErr := filepath.EvalSymlinks(repoDir)
	require.NoError(t, evalErr)
	pwdBytes, readErr := os.ReadFile(pwdPath)
	require.NoError(t, readErr)
	assert.Equal(t, resolvedRepoDir, strings.TrimSpace(string(pwdBytes)), "subprocess must run with cwd set to the codebase work dir")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictPass, outcome)
}

// TestReviewGateRunner_NonEmptyDiff_UsesPlainCallOptions verifies that a non-empty-diff
// review call is NOT granted tool access (no --allowedTools/--permission-mode) and uses
// the plain (no-tool-access) system prompt — the pre-existing, unchanged behavior.
func TestReviewGateRunner_NonEmptyDiff_UsesPlainCallOptions(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")
	baseSHA, shaErr := GetGitHeadSHA(repoDir)
	require.NoError(t, shaErr)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("real work\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "feature.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "add feature")

	itemData := BacklogItemData{
		Title:              "Plain Call Options Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = baseSHA // no worktree recorded — falls back to this + item.RepoPath

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	verdictJSON := `{"overall":"PASS","summary":"looks good","verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictEncoded)
	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, fakeRunner.CallCount())
	assert.False(t, fakeRunner.HasArg("--allowedTools"), "non-empty-diff review must not request tool access")
	assert.False(t, fakeRunner.HasArg("--permission-mode"), "non-empty-diff review must not set a permission mode")

	args := fakeRunner.ArgsForCall(0)
	require.NotNil(t, args)
	assert.Contains(t, args, headless.HeadlessReviewSystemPrompt(), "the plain (non-codebase-access) system prompt must be used for a non-empty diff")

	prompt := fakeRunner.StdinForCall(0)
	assert.Contains(t, prompt, "feature.txt", "the real diff must reach the reviewer prompt")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictPass, outcome)
}

// TestReviewGateRunner_CodebaseReadTimeout_RecordsUnverifiableNotFail verifies that a
// context.DeadlineExceeded on the codebase-read (empty-diff) path degrades to
// UNVERIFIABLE — never the generic FAIL path used for other call errors — per
// ADR-001's 2026-07-14 Repair Pass Addendum.
func TestReviewGateRunner_CodebaseReadTimeout_RecordsUnverifiableNotFail(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoDir, headSHA := newEmptyDiffGitRepo(t)

	itemData := BacklogItemData{
		Title:              "Codebase Read Timeout Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(context.Background(), itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(context.Background(), ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	scriptPath := writeOccupyAwareFakeClaudeScript(t, scriptDir, `{"session_id":"s1","result":"unused","cost_usd":0}`)
	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1}, runner)

	occupyCtx, occupyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer occupyCancel()
	occupyPoolSemaphore(t, pool, occupyCtx)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Bypass the Story 2.2.6 capability self-check so this test exercises the
	// call-timeout degrade path it's actually named for, not the (also-UNVERIFIABLE,
	// but different) capability-self-check-failure path.
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	shortCtx, shortCancel := shortRealTimeoutCtx()
	defer shortCancel()

	var onPassCalled atomic.Bool
	runnerUnderTest.Run(shortCtx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, onPassCalled.Load(), "onPass must not fire for a timed-out review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictUnverifiable, outcome, "a codebase-read timeout must degrade to UNVERIFIABLE, never be recorded as FAIL")
}

// TestReviewGateRunner_EmptyDiff_UsesShorterCodebaseReadTimeout verifies the
// codebase-read path is treated distinctly from the diff path on a call timeout: an
// empty-diff review degrades to UNVERIFIABLE (the dedicated codebase-read timeout
// handling), while a non-empty-diff review under the identical timeout condition takes
// the ordinary FAIL path — proving BuildReviewCallOptions' path-specific timeout
// selection actually drives different runtime behavior, not just a different constant
// value (that value is separately covered by
// TestBuildReviewCallOptions_EmptyDiff_ReturnsCodebaseAccessOptionsAndShortTimeout).
func TestReviewGateRunner_EmptyDiff_UsesShorterCodebaseReadTimeout(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	gitRepoDir := t.TempDir()
	runGitOrFail(t, gitRepoDir, "init", "-b", "main")
	runGitOrFail(t, gitRepoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, gitRepoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(gitRepoDir, "README.md"), []byte("base\n"), 0o644))
	runGitOrFail(t, gitRepoDir, "add", "README.md")
	runGitOrFail(t, gitRepoDir, "commit", "-m", "initial")
	baseSHA, shaErr := GetGitHeadSHA(gitRepoDir)
	require.NoError(t, shaErr)
	require.NoError(t, os.WriteFile(filepath.Join(gitRepoDir, "feature.txt"), []byte("real work\n"), 0o644))
	runGitOrFail(t, gitRepoDir, "add", "feature.txt")
	runGitOrFail(t, gitRepoDir, "commit", "-m", "add feature")

	// A real git repo (not a non-git tempdir) with lastCommitSha pinned to HEAD below,
	// so the diff is legitimately empty rather than a diff-computation error — see
	// newEmptyDiffGitRepo's doc comment.
	emptyDiffRepoDir, emptyDiffHeadSHA := newEmptyDiffGitRepo(t)

	// runScenario spawns a fresh script/marker/pool per scenario so occupying-call
	// synchronization is never shared or raced across scenarios.
	runScenario := func(t *testing.T, repoDir, lastCommitSha string) ReviewOutcome {
		t.Helper()
		itemData := BacklogItemData{
			Title:              "Timeout comparison",
			AcceptanceCriteria: `[]`,
			Priority:           1,
			Status:             string(BacklogStatusInProgress),
			RepoPath:           repoDir,
		}
		createdItemData, err := storage.CreateBacklogItem(context.Background(), itemData)
		require.NoError(t, err)

		workIS, err := storage.CreateItemSession(context.Background(), ItemSessionData{
			ItemID:      createdItemData.ID,
			SessionUUID: uuid.New().String(),
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)
		workIS.LastCommitSha = lastCommitSha

		item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

		scriptDir := t.TempDir()
		scriptPath := writeOccupyAwareFakeClaudeScript(t, scriptDir, `{"session_id":"s1","result":"unused","cost_usd":0}`)
		runner := headless.NewProcessRunnerForTesting(scriptPath)
		pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1}, runner)

		occupyCtx, occupyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer occupyCancel()
		occupyPoolSemaphore(t, pool, occupyCtx)

		getPool := func() *headless.Pool { return pool }
		runnerUnderTest := NewReviewGateRunner(storage, getPool, func() AutoReopenSpawner { return nil }, func() Notifier { return nil }, nil)
		// Bypass the Story 2.2.6 capability self-check so this scenario exercises the
		// call-timeout degrade path, not the capability-self-check-failure path.
		runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

		shortCtx, shortCancel := shortRealTimeoutCtx()
		defer shortCancel()
		runnerUnderTest.Run(shortCtx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

		outcome, err := storage.GetMostRecentReviewVerdictForItem(context.Background(), item.ID)
		require.NoError(t, err)
		return outcome
	}

	emptyDiffOutcome := runScenario(t, emptyDiffRepoDir, emptyDiffHeadSHA)
	nonEmptyDiffOutcome := runScenario(t, gitRepoDir, baseSHA)

	assert.Equal(t, ReviewVerdictUnverifiable, emptyDiffOutcome, "the codebase-read (empty-diff) path must degrade a call timeout to UNVERIFIABLE")
	assert.Equal(t, ReviewVerdictFail, nonEmptyDiffOutcome, "the diff (non-empty-diff) path must take the generic FAIL path on a call timeout, not the codebase-read UNVERIFIABLE treatment")
}

// TestReviewGateRunner_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable verifies
// that a PASS verdict returned on the codebase-read path with an empty tool_reads list
// is downgraded to UNVERIFIABLE before being persisted — the reviewer claimed a
// verdict without evidence of having actually used its granted tool access.
func TestReviewGateRunner_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir, headSHA := newEmptyDiffGitRepo(t)

	itemData := BacklogItemData{
		Title:              "Empty Tool Reads Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	verdictJSON := `{"overall":"PASS","summary":"trust me, it's already implemented","tool_reads":[],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Bypass the Story 2.2.6 capability self-check: the fake script has only one
	// scripted response, reserved for the real review call this test asserts on.
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	var onPassCalled atomic.Bool
	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, onPassCalled.Load(), "onPass must not fire for a downgraded (non-PASS) verdict")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictUnverifiable, outcome, "a PASS with no tool_reads evidence on the codebase-read path must be downgraded")
}

// TestReviewGateRunner_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable
// verifies that a PASS verdict citing a tool_reads path that does not actually exist
// under the codebase work dir is downgraded to UNVERIFIABLE — a fabricated citation is
// treated the same as no evidence at all.
func TestReviewGateRunner_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// deliberately no additional files beyond README.md/.git — "does/not/exist.go"
	// (referenced below) must not exist under repoDir.
	repoDir, headSHA := newEmptyDiffGitRepo(t)

	itemData := BacklogItemData{
		Title:              "Fabricated Tool Reads Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["does/not/exist.go"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Bypass the Story 2.2.6 capability self-check: the fake script has only one
	// scripted response, reserved for the real review call this test asserts on.
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	var onPassCalled atomic.Bool
	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, onPassCalled.Load(), "onPass must not fire for a downgraded (non-PASS) verdict")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictUnverifiable, outcome, "a PASS citing a fabricated tool_reads path must be downgraded")
}

// ─── Story 2.2.6: CodebaseReadCapabilitySelfCheck wiring ─────────────────────

// countLines returns the number of newline-terminated lines in the file at path,
// or 0 if the file does not exist yet.
func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

// TestReviewGateRunner_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall
// verifies that when the codebase-read capability self-check has already failed, the
// review gate records an UNVERIFIABLE verdict directly and never attempts the real
// AllowedTools/PermissionMode-bearing codebase-read call — a degraded claude CLI/config
// would otherwise silently return zero real evidence.
func TestReviewGateRunner_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir, headSHA := newEmptyDiffGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	itemData := BacklogItemData{
		Title:              "Capability Self-Check Failure Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	callCountPath := filepath.Join(scriptDir, "calls.txt")
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho call >> %s\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", callCountPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runnerUnderTest.SetCapabilityCheck(headless.NewFailedCapabilitySelfCheckForTesting())

	var onPassCalled atomic.Bool
	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, onPassCalled.Load(), "onPass must not fire when the capability self-check has failed")
	assert.Equal(t, 0, countLines(t, callCountPath), "no real codebase-read call should have been attempted once the capability self-check has failed")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictUnverifiable, outcome, "a failed capability self-check must degrade the review to UNVERIFIABLE")
}

// TestReviewGateRunner_CapabilitySelfCheckSucceeds_ProceedsNormallyAndOnlyChecksOnce
// verifies that a passing capability self-check lets the review proceed normally, and
// that the self-check itself only runs once across two separate review gate
// invocations sharing the same ReviewGateRunner (and therefore the same
// *CodebaseReadCapabilitySelfCheck instance) — the second review's headless call count
// should include only the real review call, not another self-check smoke test.
func TestReviewGateRunner_CapabilitySelfCheckSucceeds_ProceedsNormallyAndOnlyChecksOnce(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	scriptDir := t.TempDir()
	callCountPath := filepath.Join(scriptDir, "calls.txt")

	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)

	// The script distinguishes the self-check's fixed marker-read prompt (via stdin
	// content) from the real review prompt, responding with the capability-check
	// marker for the former and the scripted PASS verdict for the latter. The marker
	// literal must match headless.capabilityCheckMarkerValue exactly.
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf(`#!/bin/sh
input="$(cat)"
echo call >> %s
case "$input" in
  *"Read the file marker.txt in your current working directory"*)
    cat <<'SELFCHECKEOF'
{"session_id":"s1","result":"STAPLER_SQUAD_CAPABILITY_CHECK_9f3a2b71","cost_usd":0}
SELFCHECKEOF
    ;;
  *)
    cat <<'REVIEWEOF'
%s
REVIEWEOF
    ;;
esac
`, callCountPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Fresh (not pre-passed) instance: the real self-check must run and succeed via
	// the fake script's marker-echo branch above.
	runnerUnderTest.SetCapabilityCheck(&headless.CodebaseReadCapabilitySelfCheck{})

	makeItem := func(title string) (*BacklogItemData, ItemSessionSummary) {
		repoDir, headSHA := newEmptyDiffGitRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))
		itemData := BacklogItemData{
			Title:              title,
			AcceptanceCriteria: `[]`,
			Priority:           1,
			Status:             string(BacklogStatusInProgress),
			RepoPath:           repoDir,
		}
		createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
		require.NoError(t, err)
		workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      createdItemData.ID,
			SessionUUID: uuid.New().String(),
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)
		workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error
		return &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}, workIS
	}

	item1, workIS1 := makeItem("Capability Self-Check Success Test 1")
	runnerUnderTest.Run(ctx, item1, workIS1, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	outcome1, err := storage.GetMostRecentReviewVerdictForItem(ctx, item1.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictPass, outcome1, "review must proceed normally once the capability self-check passes")
	assert.Equal(t, 2, countLines(t, callCountPath), "first review should trigger exactly 2 subprocess calls: the capability self-check, then the real review call")

	item2, workIS2 := makeItem("Capability Self-Check Success Test 2")
	runnerUnderTest.Run(ctx, item2, workIS2, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	outcome2, err := storage.GetMostRecentReviewVerdictForItem(ctx, item2.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictPass, outcome2)
	assert.Equal(t, 3, countLines(t, callCountPath), "second review must not re-run the capability self-check — only 1 more (real review) call should fire")
}

// ─── Epic 2.5: path=/duration_ms= observability logging ─────────────────────

// TestReviewGateRunner_LogCompletionLine_IncludesPathDiff_WhenNonEmptyDiff verifies
// that the "spawnReviewGate headless review complete" completion log line for a
// non-empty-diff review includes both path=diff and a duration_ms= field.
func TestReviewGateRunner_LogCompletionLine_IncludesPathDiff_WhenNonEmptyDiff(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "main")
	runGitOrFail(t, repoDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, repoDir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "README.md")
	runGitOrFail(t, repoDir, "commit", "-m", "initial")
	baseSHA, shaErr := GetGitHeadSHA(repoDir)
	require.NoError(t, shaErr)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("real work\n"), 0o644))
	runGitOrFail(t, repoDir, "add", "feature.txt")
	runGitOrFail(t, repoDir, "commit", "-m", "add feature")

	itemData := BacklogItemData{
		Title:              "Log Completion Line Diff Path Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = baseSHA

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	verdictJSON := `{"overall":"PASS","summary":"looks good","verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictEncoded)
	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)

	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	logOutput := buf.String()
	assert.Contains(t, logOutput, "spawnReviewGate headless review complete", "the completion log line must be emitted")
	assert.Contains(t, logOutput, "path=diff", "a non-empty-diff review's completion log line must report path=diff")
	assert.Contains(t, logOutput, "duration_ms=", "the completion log line must report duration_ms=")
}

// TestReviewGateRunner_LogCompletionLine_IncludesPathCodebaseReadVerifiedOrDegraded
// verifies that the completion log line for a codebase-read review reports one of the
// codebase-read-* path labels (here: codebase-read-verified, since tool_reads cites a
// real file) alongside a duration_ms= field.
func TestReviewGateRunner_LogCompletionLine_IncludesPathCodebaseReadVerifiedOrDegraded(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir, headSHA := newEmptyDiffGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	itemData := BacklogItemData{
		Title:              "Log Completion Line Codebase Read Path Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	scriptDir := t.TempDir()
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	// Bypass the Story 2.2.6 capability self-check: the fake script has only one
	// scripted response, reserved for the real review call this test asserts on.
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)

	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	logOutput := buf.String()
	assert.Contains(t, logOutput, "spawnReviewGate headless review complete", "the completion log line must be emitted")
	assert.Contains(t, logOutput, "path=codebase-read-verified", "a verified codebase-read review's completion log line must report path=codebase-read-verified")
	assert.Contains(t, logOutput, "duration_ms=", "the completion log line must report duration_ms=")
}

// ─── ReviewContextExtras wiring ─────────────────────────────────────────────

// TestReviewGateRunner_EmptyDiff_ContextExtrasReachPrompt is a wiring-level test
// proving that on the codebase-read (empty-diff) path, ReviewGateRunner.Run actually
// fetches prior review sessions (ListItemSessions), the full notes history
// (ListProgressNotesForItem), the item's Description, and — when a scrollback manager
// is wired — writes a searchable transcript file, and that all of this reaches the
// actual prompt text sent to the pool (captured via the fake script's stdin). It also
// verifies the transcript file is cleaned up (removed from codebaseWorkDir) once Run
// returns, on the success path.
func TestReviewGateRunner_EmptyDiff_ContextExtrasReachPrompt(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir, headSHA := newEmptyDiffGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	itemData := BacklogItemData{
		Title:              "Context Extras Wiring Test",
		Description:        "Add OAuth2 login support end to end",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// A prior review session+verdict — must surface in "## Prior Review Attempts".
	_, err = storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: "prior-review-" + uuid.New().String(),
		SessionRole: SessionRoleReview,
	}, ReviewVerdictData{
		OverallOutcome: ReviewVerdictUnverifiable,
		Summary:        "prior attempt could not locate satisfying evidence",
	})
	require.NoError(t, err)

	// A report_progress note — must surface in "## Full Notes History".
	require.NoError(t, storage.AppendProgressNote(ctx, createdItemData.ID, 0, "investigated the auth package first", "in_progress"))

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item, err := storage.GetBacklogItem(ctx, createdItemData.ID)
	require.NoError(t, err, "GetBacklogItem must succeed so item.StatusEvents/Description are populated, matching production wiring in backlog_lifecycle.go")

	// Seed scrollback data for the work session so WriteReviewTranscriptFile has
	// something to write — must surface in "## Session Transcript".
	sm := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())
	require.NoError(t, sm.AppendOutput(workSessionUUID, []byte("$ go test ./...\nok\n")))

	scriptDir := t.TempDir()
	stdinPath := filepath.Join(scriptDir, "stdin.txt")
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > %s\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", stdinPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())
	runnerUnderTest.SetScrollbackManager(sm)

	runnerUnderTest.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	promptBytes, readErr := os.ReadFile(stdinPath)
	require.NoError(t, readErr, "fake-claude.sh must have been invoked")
	prompt := string(promptBytes)

	assert.Contains(t, prompt, "## Prior Review Attempts", "prior review sessions must be fetched and rendered")
	assert.Contains(t, prompt, "prior attempt could not locate satisfying evidence")
	assert.Contains(t, prompt, "## Full Notes History", "progress notes must be fetched and rendered")
	assert.Contains(t, prompt, "investigated the auth package first")
	assert.Contains(t, prompt, "## Item Context", "item description must reach the prompt")
	assert.Contains(t, prompt, "Add OAuth2 login support end to end")
	assert.Contains(t, prompt, "## Session Transcript", "a searchable transcript file must be written and referenced when a scrollback manager is wired")

	transcriptPath := filepath.Join(repoDir, reviewTranscriptFileName(workSessionUUID))
	_, statErr := os.Stat(transcriptPath)
	assert.True(t, os.IsNotExist(statErr), "the transcript file must be cleaned up (removed) once Run returns")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictPass, outcome)
}

// TestReviewGateRunner_PassVerdict_TranscriptFileGoneBeforeOnPassFires is a regression
// test for a real bug: WriteReviewTranscriptFile writes a session-transcript file
// directly into codebaseWorkDir (the item's real worktree), and Run previously relied
// solely on a deferred cleanup() registered right after the write. On a PASS verdict,
// onPass (BacklogLifecycleListener.pushAndCreatePR in production) fires SYNCHRONOUSLY
// later in the same Run() call — deferred calls run at function RETURN, not
// immediately — so onPass's CommitChanges (which runs `git add .` when the worktree is
// dirty) could stage and commit the still-present transcript file into the real PR.
// The fix calls cleanup() explicitly right after the headless call returns, before any
// verdict-dependent branching that could invoke onPass. This test injects an onPass
// callback that itself asserts (via os.Stat, inside the callback — not after Run
// returns) that the transcript file is already gone by the time onPass is invoked,
// which only passes if the explicit early cleanup() actually ran before onPass.
func TestReviewGateRunner_PassVerdict_TranscriptFileGoneBeforeOnPassFires(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoDir, headSHA := newEmptyDiffGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	itemData := BacklogItemData{
		Title:              "Transcript Gone Before OnPass Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	// Seed scrollback data so WriteReviewTranscriptFile actually writes a file into
	// repoDir (codebaseWorkDir here, since no worktree is recorded for this session).
	sm := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())
	require.NoError(t, sm.AppendOutput(workSessionUUID, []byte("$ go build ./...\nok\n")))

	scriptDir := t.TempDir()
	stdinPath := filepath.Join(scriptDir, "stdin.txt")
	verdictJSON := `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`
	verdictEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%s,"cost_usd":0.02}`, verdictEncoded)
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat > %s\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", stdinPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 2}, runner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())
	runnerUnderTest.SetScrollbackManager(sm)

	transcriptPath := filepath.Join(repoDir, reviewTranscriptFileName(workSessionUUID))

	var onPassCalled bool
	var transcriptExistedWhenOnPassFired bool
	onPass := func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled = true
		if _, statErr := os.Stat(transcriptPath); statErr == nil {
			transcriptExistedWhenOnPassFired = true
		}
	}

	runnerUnderTest.Run(ctx, item, workIS, onPass)

	require.True(t, onPassCalled, "onPass must have been invoked on a PASS verdict — otherwise this test proves nothing")
	assert.False(t, transcriptExistedWhenOnPassFired, "the review transcript file must already be removed from codebaseWorkDir by the time onPass fires, so a subsequent `git add .`/CommitChanges cannot stage and commit it into the real PR")

	// Also confirm the file is gone after Run returns (the pre-existing deferred
	// cleanup backstop), not just inside onPass.
	_, statErr := os.Stat(transcriptPath)
	assert.True(t, os.IsNotExist(statErr), "the transcript file must remain cleaned up after Run returns")
}

// TestReviewGateRunner_CodebaseReadTimeout_CleansUpTranscriptFile verifies that the
// searchable transcript file written for the codebase-read path is still cleaned up
// (never left behind in codebaseWorkDir) even when the headless call itself times out
// — the deferred cleanup() must fire on every return path out of Run, not only the
// success path covered by TestReviewGateRunner_EmptyDiff_ContextExtrasReachPrompt.
func TestReviewGateRunner_CodebaseReadTimeout_CleansUpTranscriptFile(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoDir, headSHA := newEmptyDiffGitRepo(t)

	itemData := BacklogItemData{
		Title:              "Transcript Cleanup On Timeout Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(context.Background(), itemData)
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(context.Background(), ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS.LastCommitSha = headSHA // no worktree recorded — pin to HEAD so the diff is legitimately empty, not an error

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	sm := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())
	require.NoError(t, sm.AppendOutput(workSessionUUID, []byte("$ go build ./...\nok\n")))

	scriptDir := t.TempDir()
	scriptPath := writeOccupyAwareFakeClaudeScript(t, scriptDir, `{"session_id":"s1","result":"unused","cost_usd":0}`)
	runner := headless.NewProcessRunnerForTesting(scriptPath)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{MaxCallsPerSession: 1, MaxConcurrentSessions: 1}, runner)

	occupyCtx, occupyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer occupyCancel()
	occupyPoolSemaphore(t, pool, occupyCtx)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runnerUnderTest := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)
	runnerUnderTest.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())
	runnerUnderTest.SetScrollbackManager(sm)

	shortCtx, shortCancel := shortRealTimeoutCtx()
	defer shortCancel()
	runnerUnderTest.Run(shortCtx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	outcome, err := storage.GetMostRecentReviewVerdictForItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictUnverifiable, outcome, "a codebase-read timeout must degrade to UNVERIFIABLE")

	transcriptPath := filepath.Join(repoDir, reviewTranscriptFileName(workSessionUUID))
	_, statErr := os.Stat(transcriptPath)
	assert.True(t, os.IsNotExist(statErr), "the transcript file must be cleaned up even when the headless call times out")
}

// TestReviewGateRunner_ScrollbackManager_ConcurrentSetAndGet_NoRace is a regression
// test for a real data race: ReviewGateRunner.scrollbackManager used to be a bare,
// unguarded field with a doc comment claiming it was "set once during startup wiring
// before any concurrent Run() calls begin" — false in production, since
// server/dependencies.go starts a background goroutine (inst.StartController()) that
// can trigger Run() calls before SetScrollbackManager is called on a separate,
// concurrent code path, and the HTTP server can already be serving requests before all
// dependency wiring completes. This test concurrently calls SetScrollbackManager and
// the internal getScrollbackManager (the same accessor Run uses) many times; run with
// `go test -race`, it fails on the pre-fix bare field and passes once the field is
// guarded by scrollbackMu.
func TestReviewGateRunner_ScrollbackManager_ConcurrentSetAndGet_NoRace(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	getPool := func() *headless.Pool { return nil }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, func() Notifier { return nil }, nil)

	sm1 := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())
	sm2 := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				runner.SetScrollbackManager(sm1)
			} else {
				runner.SetScrollbackManager(sm2)
			}
		}(i)
		go func() {
			defer wg.Done()
			_ = runner.getScrollbackManager()
		}()
	}
	wg.Wait()
}
