package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// TestReviewGateRunner_SkipReviewGate verifies that Run returns immediately
// without consulting the session creator or calling onPass when
// item.SkipReviewGate is true.
func TestReviewGateRunner_SkipReviewGate(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := &BacklogItemData{
		ID:             uuid.New().String(),
		RepoPath:       "/some/repo",
		SkipReviewGate: true,
	}
	is := ItemSessionSummary{
		ID:          uuid.New().String(),
		SessionUUID: uuid.New().String(),
	}

	var onPassCalled atomic.Bool

	spawner := &mockReviewGateSpawner{}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)

	runner.Run(context.Background(), item, is, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when SkipReviewGate is true")
	assert.False(t, onPassCalled.Load(), "onPass must not be called when SkipReviewGate is true")
}

// TestReviewGateRunner_SpawnsReviewSession_Success verifies the happy path: Run
// builds the review prompt, calls SpawnReviewSession, and persists an ItemSession
// linking the review role to the real Instance UUID returned by the spawner.
func TestReviewGateRunner_SpawnsReviewSession_Success(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	itemData := BacklogItemData{
		Title:              "Spawn Review Session Test",
		Description:        "Testing the sessionCreator-spawn path",
		AcceptanceCriteria: `[]`,
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
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:       createdItemData.ID,
		RepoPath: createdItemData.RepoPath,
	}

	reviewInstance := &Instance{UUID: uuid.New().String()}
	spawner := &mockReviewGateSpawner{instance: reviewInstance}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	require.Equal(t, 1, spawner.getCallCount(), "SpawnReviewSession must be called exactly once")
	assert.False(t, onPassCalled.Load(), "onPass must never be called directly by Run anymore — the verdict is only known once the spawned session exits")

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	var reviewEntry *ItemSessionSummary
	for i := range sessions {
		if sessions[i].Role == SessionRoleReview {
			reviewEntry = &sessions[i]
		}
	}
	require.NotNil(t, reviewEntry, "a review ItemSession must be created")
	assert.Equal(t, reviewInstance.UUID, reviewEntry.SessionUUID, "the review ItemSession must be linked to the real Instance UUID returned by SpawnReviewSession")
}

// TestReviewGateRunner_RoutesPromptThroughPipelineEngine_When_ItemHasCustomPipelineMode
// is the regression guard for the "custom PipelineMode's ReviewPromptTemplate has
// zero effect on the review most items actually receive" gap (docs/tasks/
// backlog-feature-improvement.md, 2026-07-19 update, bucket [3]): it proves that
// when a ReviewGateRunner is constructed with a non-nil PipelineEngine and the
// item's PipelineMode resolves to a custom mode, the prompt handed to
// SpawnReviewSession is that mode's rendered ReviewPromptTemplate — not the
// hardcoded BuildReviewPrompt content.
func TestReviewGateRunner_RoutesPromptThroughPipelineEngine_When_ItemHasCustomPipelineMode(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{
				Slug:                 "quick",
				Name:                 "Quick Fix",
				ReviewPromptTemplate: "CUSTOM REVIEW MARKER for {{item_id}}",
			}}, nil
		},
	}
	engine, err := NewPipelineEngine(repo)
	require.NoError(t, err)

	itemData := BacklogItemData{
		Title:              "Custom Pipeline Mode Review Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           newNonEmptyDiffGitRepo(t),
		PipelineMode:       "quick",
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

	item := &BacklogItemData{
		ID:           createdItemData.ID,
		RepoPath:     createdItemData.RepoPath,
		PipelineMode: "quick",
	}

	spawner := &mockReviewGateSpawner{instance: &Instance{UUID: uuid.New().String()}}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, engine)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, spawner.getCallCount(), "SpawnReviewSession must be called exactly once")
	prompt := spawner.getLastPrompt()
	assert.Contains(t, prompt, "CUSTOM REVIEW MARKER for "+item.ID,
		"a custom PipelineMode's ReviewPromptTemplate must drive the real review gate's prompt")
	assert.NotContains(t, prompt, "Call submit_review_verdict ONCE",
		"the hardcoded BuildReviewPrompt content must not leak through once a custom mode resolves")
}

// TestReviewGateRunner_SpawnerError_LogsAndReturns verifies that a SpawnReviewSession
// error is logged and Run returns cleanly without persisting a review ItemSession.
func TestReviewGateRunner_SpawnerError_LogsAndReturns(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	itemData := BacklogItemData{
		Title:              "Spawner Error Test",
		AcceptanceCriteria: `[]`,
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
	})
	require.NoError(t, err)

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: createdItemData.RepoPath}

	spawner := &mockReviewGateSpawner{err: errors.New("boom: could not create tmux session")}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)

	require.NotPanics(t, func() {
		runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})
	})

	assert.Equal(t, 1, spawner.getCallCount())

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	for _, s := range sessions {
		assert.NotEqual(t, SessionRoleReview, s.Role, "no review ItemSession should be created when SpawnReviewSession fails")
	}
}

// TestReviewGateRunner_NilSessionCreator_LogsAndReturns verifies that Run logs and
// returns cleanly (does not panic) when no review mechanism is configured at all.
func TestReviewGateRunner_NilSessionCreator_LogsAndReturns(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	itemData := BacklogItemData{
		Title:              "Nil Session Creator Test",
		AcceptanceCriteria: `[]`,
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
	})
	require.NoError(t, err)

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: createdItemData.RepoPath}

	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return nil }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)

	require.NotPanics(t, func() {
		runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})
	})

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	for _, s := range sessions {
		assert.NotEqual(t, SessionRoleReview, s.Role, "no review ItemSession should be created when no session creator is configured")
	}
}

// TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt verifies that verification
// evidence recorded on the work session (via request_review's verification_notes
// argument) reaches the reviewer prompt passed to SpawnReviewSession, not just the
// diff and AC list. This is the regression guard for the UNVERIFIABLE-despite-real-
// verification gap: criteria describing test runs or manual UI checks are invisible
// in the diff, so the reviewer's only window into that evidence is this
// threaded-through text.
func TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt(t *testing.T) {
	t.Parallel()
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

	spawner := &mockReviewGateSpawner{}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, spawner.getCallCount(), "SpawnReviewSession must be called exactly once")

	prompt := spawner.getLastPrompt()
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
// passed to SpawnReviewSession.
func TestReviewGateRunner_MergesLiveCriterionNoteIntoStalePromptWhenSnapshotPredatesIt(t *testing.T) {
	t.Parallel()
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

	spawner := &mockReviewGateSpawner{}
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	runner := NewReviewGateRunner(storage, getAutoReopener, func() Notifier { return nil }, getSessionCreator, nil)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, spawner.getCallCount(), "SpawnReviewSession must be called exactly once")

	prompt := spawner.getLastPrompt()
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

func runGitOutputOrFail(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
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
	er := storage.repo

	db, err := sql.Open("sqlite", er.dbPath)
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
// range produces a real, non-empty diff.
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
	t.Parallel()
	// RecoverBaseCommitSHA fails here and hits review_gate.go's WarningLog.Printf
	// directly; redirect it so this write serializes against every other test's
	// concurrent swapWarningLog redirection instead of landing in one of their buffers.
	_ = swapWarningLog(t)
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Real git repo with one commit so `git diff <bogus-sha>..HEAD` fails with a real
	// "unknown revision" error rather than "not a git repository". The repo's only
	// branch is deliberately named "feature" — none of RecoverBaseCommitSHA's default
	// candidates (main/master/develop/trunk) exist here — so the worktree's recorded
	// BranchName below ("feature") both matches what's actually checked out (passing
	// the worktree-identity check) and still drives every one of RecoverBaseCommitSHA's
	// `git merge-base HEAD <candidate>` attempts to fail outright (unknown revision),
	// exercising the "auto-repair itself cannot resolve any candidate" sub-branch of the
	// repair logic, distinct from
	// TestReviewGateRunner_DiffComputationFailure_RecoveredButEmptyDiff_StillBlocks's
	// "repair resolves but the recovered diff is empty" sub-branch below.
	repoDir := t.TempDir()
	runGitOrFail(t, repoDir, "init", "-b", "feature")
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
		repoDir, repoDir, "diff-error-test", "feature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	// The session creator must never be consulted — the review should be blocked
	// before reaching it.
	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when the diff could not be computed")
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
// was silently indistinguishable from a legitimate empty diff. This constructs that
// exact shape: no worktree recorded (SaveInstances is never called for this session),
// and a LastCommitSha that is well-formed but does not resolve, so `git diff
// <bogus>..HEAD` genuinely errors rather than returning an empty diff. Since no
// worktree means wt.BranchName == "", auto-repair is never attempted (see
// TestReviewGateRunner_NoBranchName_DiffComputationFailure_SkipsRepairAndBlocksImmediately
// for the dedicated regression test of that skip condition) and the failure must fall
// straight through to the same block-and-notify path as the worktree-present case.
func TestReviewGateRunner_NoWorktreeRecorded_DiffComputationFailure_BlocksReview(t *testing.T) {
	t.Parallel()
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

	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when the diff could not be computed")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a genuine diff-computation failure on the no-worktree path must block with FAIL, not silently fall through")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the cap/notify machinery eventually engages")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_EmptyCommittedDiff_BlocksReviewInsteadOfFalsePass is the
// regression test for the "no real work happened" class of false-verification bug:
// unlike the diff-computation-failure tests above, this reproduces a session whose
// diff computes successfully — no git error at all — but is empty because
// BaseCommitSha and the repo's current HEAD are the same commit (e.g. a work
// session that ended without ever committing, then got swept into review by
// ReconcileStuckItems purely because its sessions had all ended). Before this fix,
// Run had no check for this case at all: it would proceed to spawn a real review
// session with nothing to review, risking a false PASS/UNVERIFIABLE verdict that
// marks the item done despite zero shipped work — the same failure shape as
// BUG-047/BUG-065 in the sibling reconciliation paths (backlog_lifecycle.go,
// backlog_lifecycle_pr.go). The fix blocks with a distinct FAIL verdict and still
// feeds the auto-reopen machinery, exactly like the diff-error blocks it sits next
// to.
func TestReviewGateRunner_EmptyCommittedDiff_BlocksReviewInsteadOfFalsePass(t *testing.T) {
	t.Parallel()
	// Empty committed diff hits review_gate.go's WarningLog.Printf directly; redirect
	// it so this write serializes against every other test's concurrent
	// swapWarningLog redirection instead of landing in one of their buffers.
	_ = swapWarningLog(t)
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
	headSHA := strings.TrimSpace(runGitOutputOrFail(t, repoDir, "rev-parse", "HEAD"))

	itemData := BacklogItemData{
		Title:              "Empty committed diff test",
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
	// No SaveInstances call, so GetWorktreeDataBySessionUUID finds nothing and Run
	// takes the no-worktree branch, diffing BaseCommitSha against repoPath's HEAD —
	// which are the same commit, so the diff is genuinely empty with no error.
	workIS.BaseCommitSha = headSHA

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when the committed diff is empty")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "an empty committed diff must block with FAIL, not proceed to a real review that could false-PASS")

	assert.Contains(t, notifier.titles(), "Review blocked — no changes to review")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the item returns to in_progress for real rework")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_DiffComputationFailure_AutoRepairsFromDivergentBranch is the
// positive counterpart to the blocking test above: it reproduces the exact live-data
// shape of backlog item ae1e2070-db02-4ad7-8580-633ef9904f31 — a real feature branch
// with real committed work, whose recorded base_commit_sha is a well-formed but
// nonexistent SHA (simulating a pruned/corrupted commit) — and verifies the review
// proceeds on the recovered (real) diff instead of blocking, because the feature
// branch and "main" have a genuine common ancestor that RecoverBaseCommitSHA can find
// and a non-empty diff results. Uses a real, separate `git worktree add` checkout for
// wt.WorktreePath (distinct from repoDir/item.RepoPath) rather than pointing both at
// the same directory — matching real worktree topology (a worktree always has its own
// branch checked out) is what exercises RecoverBaseCommitSHA's actual fix (backlog item
// e7664cbf: recompute inside the worktree, not repoPath's ambient checkout) and avoids
// tripping the worktree-identity guard, which would otherwise correctly flag a
// same-directory fixture as a mismatch (repoDir can only have one branch checked out
// at a time). Since review now always spawns a real session rather than computing a
// verdict inline, "proceeds" means SpawnReviewSession is called with the recovered
// diff in its prompt.
func TestReviewGateRunner_DiffComputationFailure_AutoRepairsFromDivergentBranch(t *testing.T) {
	t.Parallel()
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

	// Real feature branch with real committed work, checked out in its own dedicated
	// worktree directory — the same shape as ae1e2070's stelekit worktree, which had a
	// genuine 302+/88- fix already committed.
	worktreeDir := t.TempDir()
	runGitOrFail(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir, "main")
	require.NoError(t, os.WriteFile(worktreeDir+"/feature.txt", []byte("real work\n"), 0o644))
	runGitOrFail(t, worktreeDir, "add", "feature.txt")
	runGitOrFail(t, worktreeDir, "commit", "-m", "real fix")

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
	// GetGitDiff(worktree) fails the same way GetGitDiffRef(repo fallback) does,
	// exactly as observed for ae1e2070.
	inst := newTestInstance("diff-repair-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, worktreeDir, "diff-repair-test", "feature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	require.Equal(t, 1, spawner.getCallCount(), "SpawnReviewSession must actually be called with the recovered diff instead of the review being blocked")
	assert.False(t, onPassCalled.Load(), "onPass is never called directly by Run anymore")

	prompt := spawner.getLastPrompt()
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
	t.Parallel()
	// The recovered-but-empty diff path hits review_gate.go's WarningLog.Printf
	// directly; redirect it so this write serializes against every other test's
	// concurrent swapWarningLog redirection instead of landing in one of their buffers.
	_ = swapWarningLog(t)
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

	// The session creator must never be consulted — a recovered-but-empty diff must
	// still block before ever reaching the reviewer.
	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must never be consulted when the recovered diff is still empty")
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
	t.Parallel()
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

	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	redirectInfoLog(t)
	warnBuf := swapWarningLog(t)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when the diff could not be computed")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")
	assert.NotContains(t, warnBuf.String(), "RecoverBaseCommitSHA", "repair must never be attempted when the worktree has no BranchName")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a distinct FAIL verdict must be recorded, not a silent pass-through")

	assert.Contains(t, notifier.titles(), "Review blocked — diff computation failed")
}

// TestReviewGateRunner_WorktreeBranchMismatch_BlocksReviewWithDistinctVerdict is a
// regression test for backlog item e7664cbf's "worktree reset to an unrelated empty
// repo" symptom: worktree paths are resolved by title-derived branch name, not
// item/session UUID (session/git/worktree.go's findExistingWorktreeForBranch), so a
// session's recorded worktree row can point at a directory that has since been
// recreated/reused for a different item's branch. Mirrors
// TestReconcileBouncingItems_should_stillFlag_When_WorktreePathWasRecycledToAnotherBranch's
// fixture shape (session/backlog_lifecycle_stuck_test.go): the row still names this
// item's own branch ("feature"), but the directory is actually checked out on a
// different branch. Before this fix, Run would either diff (or auto-repair) against
// whatever happened to be there; it must now fail closed with a distinct verdict
// before ever computing a diff.
func TestReviewGateRunner_WorktreeBranchMismatch_BlocksReviewWithDistinctVerdict(t *testing.T) {
	t.Parallel()
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

	// The path this session's worktree row will claim — but it's checked out on
	// "other-item-branch" (a later, unrelated item's work), not this session's own
	// "feature" branch. Simulates the path having been recycled/recreated after this
	// session's own worktree was torn down.
	recycledPath := t.TempDir()
	runGitOrFail(t, repoDir, "worktree", "add", "-b", "other-item-branch", recycledPath, "main")
	require.NoError(t, os.WriteFile(recycledPath+"/other-item.txt", []byte("a later item's own commit\n"), 0o644))
	runGitOrFail(t, recycledPath, "add", "other-item.txt")
	runGitOrFail(t, recycledPath, "commit", "-m", "later item's real work")

	itemData := BacklogItemData{
		Title:              "Worktree identity mismatch test",
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

	inst := newTestInstance("worktree-mismatch-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		repoDir, recycledPath, "worktree-mismatch-test", "feature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}

	// The session creator must never be consulted — the review should be blocked
	// before any diff/branch-sync operation ever touches the mismatched worktree.
	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must never be consulted for a worktree identity mismatch")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a distinct FAIL verdict must be recorded, not a silent pass-through against the wrong worktree")

	assert.Contains(t, notifier.titles(), "Review blocked — worktree identity mismatch")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so the cap/notify machinery eventually engages")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
}

// TestReviewGateRunner_WorktreeBranchMismatch_RepeatedFailure_StillDedupsViaIsRepeatedFailure
// confirms IsRepeatedFailure's exact-summary-match dedup (session/stuck_decisions.go)
// still trips against the new worktree-identity-mismatch verdict text: two consecutive
// review attempts against the same unresolved mismatch must produce byte-identical
// Summary strings, so the existing rework-cap backstop is not silently defeated by the
// new verdict's message text varying between attempts.
func TestReviewGateRunner_WorktreeBranchMismatch_RepeatedFailure_StillDedupsViaIsRepeatedFailure(t *testing.T) {
	t.Parallel()
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

	recycledPath := t.TempDir()
	runGitOrFail(t, repoDir, "worktree", "add", "-b", "other-item-branch", recycledPath, "main")

	itemData := BacklogItemData{
		Title:              "Worktree identity mismatch repeated-failure test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           repoDir,
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }
	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	// Two independent rework attempts, same unresolved mismatch each time.
	for i := 0; i < 2; i++ {
		workSessionUUID := uuid.New().String()
		workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      createdItemData.ID,
			SessionUUID: workSessionUUID,
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)

		sessionName := fmt.Sprintf("worktree-mismatch-repeat-test-%d", i)
		inst := newTestInstance(sessionName)
		inst.UUID = workSessionUUID
		inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
			repoDir, recycledPath, sessionName, "feature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		require.NoError(t, storage.SaveInstances([]*Instance{inst}))

		item := &BacklogItemData{ID: createdItemData.ID, RepoPath: repoDir}
		runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})
	}

	recent, err := storage.GetRecentReviewVerdictSummaries(ctx, createdItemData.ID, 2)
	require.NoError(t, err)
	require.Len(t, recent, 2)
	assert.True(t, IsRepeatedFailure(recent), "IsRepeatedFailure must still trip against two consecutive worktree-identity-mismatch verdicts with identical Summary text")
}

// cloneWithOrigin clones originDir into a fresh temp directory with a real "origin"
// remote pointing back at originDir, so git.EnsureBranchSyncedWithMain's fetch/merge/
// push calls have something real to talk to (unlike this file's other fixtures, which
// use a single standalone repo with no remote at all — fine for GetGitDiff, but not
// for exercising the BUG-044 drift precondition, which fetches before every check).
func cloneWithOrigin(t *testing.T, originDir string) string {
	t.Helper()
	parent := t.TempDir()
	cloneDir := filepath.Join(parent, "clone")
	runGitOrFail(t, parent, "clone", originDir, cloneDir)
	runGitOrFail(t, cloneDir, "config", "user.email", "test@example.com")
	runGitOrFail(t, cloneDir, "config", "user.name", "Test")
	return cloneDir
}

// commitOnRepo adds n trivial commits to dir, simulating unrelated upstream activity
// landing on main while a work session's branch sits open.
func commitOnRepo(t *testing.T, dir string, n int, prefix string) {
	t.Helper()
	for i := 0; i < n; i++ {
		fname := fmt.Sprintf("%s-%d.txt", prefix, i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fname), []byte("content\n"), 0o644))
		runGitOrFail(t, dir, "add", fname)
		runGitOrFail(t, dir, "commit", "-m", fmt.Sprintf("%s commit %d", prefix, i))
	}
}

// TestReviewGateRunner_BranchDrift_BlocksReviewWithConflictDetails_When_AutoSyncConflicts
// is the core regression guard for BUG-044: a work session's branch that has drifted
// past the drift threshold, where main has since diverged on the same lines the
// branch itself touched, must not proceed to review on a diff that will be dominated
// by unrelated upstream drift (the live 693c2700 failure mode — "the diff contains no
// code related to the feature at all"). Instead the branch-sync precondition attempts
// an auto-merge, finds a real conflict, and blocks with an explicit, actionable
// message naming the conflicted file — never silently letting an inflated/misleading
// diff reach the reviewer.
func TestReviewGateRunner_BranchDrift_BlocksReviewWithConflictDetails_When_AutoSyncConflicts(t *testing.T) {
	t.Parallel()
	// The unresolvable branch-drift conflict hits review_gate.go's WarningLog.Printf
	// directly; redirect it so this write serializes against every other test's
	// concurrent swapWarningLog redirection instead of landing in one of their buffers.
	_ = swapWarningLog(t)
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	origin := t.TempDir()
	runGitOrFail(t, origin, "init", "-b", "main")
	runGitOrFail(t, origin, "config", "user.email", "test@example.com")
	runGitOrFail(t, origin, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# base\n"), 0o644))
	runGitOrFail(t, origin, "add", "README.md")
	runGitOrFail(t, origin, "commit", "-m", "initial commit")

	work := cloneWithOrigin(t, origin)
	runGitOrFail(t, work, "checkout", "-b", "feature")
	baseSHA := strings.TrimSpace(runGitCapture(t, work, "rev-parse", "HEAD"))

	// The feature branch makes its own real edit to README.md.
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGitOrFail(t, work, "add", "README.md")
	runGitOrFail(t, work, "commit", "-m", "feature: edit README")

	// Main diverges on the same line AND drifts well past the default threshold (50).
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGitOrFail(t, origin, "add", "README.md")
	runGitOrFail(t, origin, "commit", "-m", "main: edit README")
	commitOnRepo(t, origin, 55, "upstream")

	itemData := BacklogItemData{
		Title:              "Branch drift conflict test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           origin,
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

	inst := newTestInstance("branch-drift-conflict-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(origin, work, "branch-drift-conflict-test", "feature", baseSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: origin}

	// The session creator must never be consulted — review must be blocked before
	// ever reaching it, so no reviewer is ever handed the (would-be) drift-inflated diff.
	spawner := &mockReviewGateSpawner{}
	getSessionCreator := func() ReviewGateSpawner { return spawner }

	reopener := newFakeAutoReopenSpawner()
	getAutoReopener := func() AutoReopenSpawner { return reopener }

	notifier := &fakeNotifier{}
	getNotifier := func() Notifier { return notifier }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.Equal(t, 0, spawner.getCallCount(), "session creator must not be consulted when the branch could not be auto-synced")
	assert.False(t, onPassCalled.Load(), "onPass must not fire for a blocked review")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdictFail, outcome, "a distinct FAIL verdict must be recorded, not a silent pass-through")

	assert.Contains(t, notifier.titles(), "Review blocked — branch drifted too far behind main")

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID, "auto-reopen must still be invoked so a fix session can resolve the conflict")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}

	// The worktree must have been left clean — the aborted merge must not strand a
	// half-merged tree for whatever touches this worktree next.
	status := runGitCapture(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status))
}

// TestReviewGateRunner_BranchDrift_SyncsAutomaticallyAndProceeds_When_NoConflict is
// the positive counterpart: a branch drifted past the threshold but with no real
// conflict against main is synced and pushed automatically, and review proceeds
// normally — the branch-sync precondition must never block a review that could have
// just been fixed transparently.
func TestReviewGateRunner_BranchDrift_SyncsAutomaticallyAndProceeds_When_NoConflict(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	origin := t.TempDir()
	runGitOrFail(t, origin, "init", "-b", "main")
	runGitOrFail(t, origin, "config", "user.email", "test@example.com")
	runGitOrFail(t, origin, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# base\n"), 0o644))
	runGitOrFail(t, origin, "add", "README.md")
	runGitOrFail(t, origin, "commit", "-m", "initial commit")

	work := cloneWithOrigin(t, origin)
	runGitOrFail(t, work, "checkout", "-b", "feature")
	baseSHA := strings.TrimSpace(runGitCapture(t, work, "rev-parse", "HEAD"))

	// The feature branch's own unrelated change.
	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644))
	runGitOrFail(t, work, "add", "feature.txt")
	runGitOrFail(t, work, "commit", "-m", "feature work")

	// Main drifts well past the threshold, but on unrelated files.
	commitOnRepo(t, origin, 55, "upstream")

	itemData := BacklogItemData{
		Title:              "Branch drift clean-sync test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           origin,
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

	inst := newTestInstance("branch-drift-clean-sync-test")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(origin, work, "branch-drift-clean-sync-test", "feature", baseSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	item := &BacklogItemData{ID: createdItemData.ID, RepoPath: origin}

	reviewInstance := &Instance{UUID: uuid.New().String()}
	spawner := &mockReviewGateSpawner{instance: reviewInstance}
	getSessionCreator := func() ReviewGateSpawner { return spawner }
	getAutoReopener := func() AutoReopenSpawner { return nil }
	getNotifier := func() Notifier { return nil }

	runner := NewReviewGateRunner(storage, getAutoReopener, getNotifier, getSessionCreator, nil)

	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	assert.Equal(t, 1, spawner.getCallCount(), "a cleanly-synced branch must proceed to a real review, not stay blocked")

	// The sync must have actually landed on origin — otherwise the next reconciliation
	// tick (or a human inspecting the PR) sees the drift reappear.
	originTip := strings.TrimSpace(runGitCapture(t, origin, "rev-parse", "feature"))
	workTip := strings.TrimSpace(runGitCapture(t, work, "rev-parse", "feature"))
	assert.Equal(t, workTip, originTip, "the merge commit must be pushed to origin, not left local-only")
}

// runGitCapture runs a git command in dir and returns its combined output, failing
// the test on error. Distinct from runGitOrFail (which discards output) — used where
// the test needs the command's stdout (rev-parse, status --porcelain).
func runGitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}
