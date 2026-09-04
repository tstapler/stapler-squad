// Package services contains integration tests for the workspace service.
package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/vc"
)

// stubLiveFinder is a test double for LiveInstanceFinder that returns a fixed instance.
type stubLiveFinder struct {
	inst *session.Instance
}

func (s *stubLiveFinder) FindLiveInstance(_ string) *session.Instance {
	return s.inst
}

// workspaceTestFixture holds the WorkspaceService and its dependencies for a
// single test. The storage is backed by a real SQLite database in a temp dir.
type workspaceTestFixture struct {
	svc     *WorkspaceService
	bus     *events.EventBus
	storage *session.Storage
	cleanup func()
}

func setupWorkspaceTestFixture(t *testing.T) *workspaceTestFixture {
	t.Helper()

	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	svc := NewWorkspaceService(storage, bus)

	cleanup := func() {
		bus.Close()
	}

	return &workspaceTestFixture{
		svc:     svc,
		bus:     bus,
		storage: storage,
		cleanup: cleanup,
	}
}

// seedInstance creates a minimal Instance with the given title and persists it
// via AddInstance so that WorkspaceService.findInstance can resolve it.
//
// Status is set to Paused to avoid triggering tmux session start in LoadInstances;
// Running status causes FromInstanceData to call Start(), which spawns PTY
// subprocesses and times out in environments without a real tmux server.
func seedInstance(t *testing.T, storage *session.Storage, title string) {
	t.Helper()
	inst := &session.Instance{
		Title:   title,
		Path:    "/tmp/test-workspace",
		Status:  session.Stopped, // Stopped avoids tmux Start() in FromInstanceData
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(inst))
}

// --------------------------------------------------------------------------
// Input validation tests
// --------------------------------------------------------------------------

func TestWorkspaceService_SwitchWorkspace_MissingID(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     "",
		Target: "main",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestWorkspaceService_SwitchWorkspace_MissingTarget(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     "some-session",
		Target: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// --------------------------------------------------------------------------
// Concurrent switch guard tests
// --------------------------------------------------------------------------

// TestWorkspaceService_ConcurrentSwitchReturnsUnavailable verifies that a
// second SwitchWorkspace call for the same session ID while one is already
// in progress returns CodeUnavailable.
//
// The test simulates an in-progress switch by pre-loading the session ID into
// inFlightSwitches before issuing the RPC call, mirroring exactly what the handler
// does at the top of SwitchWorkspace.
func TestWorkspaceService_ConcurrentSwitchReturnsUnavailable(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionID := "concurrent-switch-session"
	seedInstance(t, fix.storage, sessionID)

	// Simulate an in-progress switch by pre-storing the session ID in the guard
	// map, exactly as the handler does with LoadOrStore.
	fix.svc.inFlightSwitches.Store(sessionID, true)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
	assert.Contains(t, connectErr.Message(), sessionID,
		"error message should mention the session ID")
	assert.Contains(t, connectErr.Message(), "already in progress",
		"error message should indicate the switch is already in progress")
}

// TestWorkspaceService_SwitchGuardCleansUpOnCompletion verifies that the
// concurrent switch guard key is removed from inFlightSwitches after SwitchWorkspace
// returns, regardless of whether the call succeeds or fails. The test exercises
// the failure path (session not found) because spinning up a real VCS workspace
// is not required to verify the guard lifecycle.
//
// After the call completes, a second call must NOT receive CodeUnavailable —
// demonstrating that the defer ws.inFlightSwitches.Delete(req.Msg.Id) fired.
func TestWorkspaceService_SwitchGuardCleansUpOnCompletion(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionID := "cleanup-guard-session"
	// Do NOT seed the session so the first call fails with CodeNotFound after
	// passing the guard check. The guard key must still be cleaned up.

	// First call: passes the guard (no pre-existing entry), fails with
	// CodeNotFound because the session does not exist in storage.
	_, firstErr := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))
	require.Error(t, firstErr)
	var firstConnectErr *connect.Error
	require.ErrorAs(t, firstErr, &firstConnectErr)
	assert.Equal(t, connect.CodeNotFound, firstConnectErr.Code(),
		"first call should fail with CodeNotFound (session not in storage), not CodeUnavailable")

	// Verify the guard key is gone after the call returned.
	_, stillLocked := fix.svc.inFlightSwitches.Load(sessionID)
	assert.False(t, stillLocked,
		"inFlightSwitches entry must be deleted after SwitchWorkspace returns")

	// Second call must also fail with CodeNotFound — NOT CodeUnavailable — which
	// proves the guard was cleaned up and does not block subsequent requests.
	_, secondErr := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))
	require.Error(t, secondErr)
	var secondConnectErr *connect.Error
	require.ErrorAs(t, secondErr, &secondConnectErr)
	assert.Equal(t, connect.CodeNotFound, secondConnectErr.Code(),
		"second call should still fail with CodeNotFound, not CodeUnavailable — guard was cleaned up")
}

// TestWorkspaceService_SwitchGuardIsPerSession verifies that concurrent switch
// guards are session-scoped: locking session A must not block a concurrent call
// for session B.
//
// Note: SwitchWorkspace wraps VCS-level failures into the response body (not
// a connect error), so a call that passes the guard returns nil error even when
// the underlying workspace operation fails. The key assertion is that the call
// for session B is NOT rejected with CodeUnavailable.
func TestWorkspaceService_SwitchGuardIsPerSession(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionA := "guard-session-a"
	sessionB := "guard-session-b"

	seedInstance(t, fix.storage, sessionB)

	// Simulate an in-progress switch for session A.
	fix.svc.inFlightSwitches.Store(sessionA, true)

	// A call for session B must NOT be blocked by session A's lock.
	// The call may succeed (nil error with response body) or fail with a
	// non-Unavailable code — but never with CodeUnavailable.
	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionB,
		Target: "main",
	}))

	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeUnavailable, connectErr.Code(),
			"lock on session A must not block calls for session B")
	}
	// err == nil also means the guard did not trigger, which is the desired
	// outcome: session B proceeded past the guard independently.
}

// --------------------------------------------------------------------------
// UUID ID lookup tests (regression: frontend now passes UUIDs, not Titles)
// --------------------------------------------------------------------------

// TestWorkspaceService_GetVCSStatus_FindsByUUID verifies that GetVCSStatus
// accepts a session UUID as the ID parameter. Before the UUID migration, only
// the Title was accepted; now MatchesID checks both UUID and Title.
func TestWorkspaceService_GetVCSStatus_FindsByUUID(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	const testUUID = "11111111-1111-1111-1111-111111111111"
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		UUID:    testUUID,
		Title:   "my-uuid-session",
		Path:    "/tmp/test-workspace",
		Status:  session.Paused, // Paused avoids tmux setup in LoadInstances (no CI tmux)
		Program: "claude",
	}))

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: testUUID,
	}))

	// Must NOT return CodeNotFound — UUID lookup must succeed.
	// /tmp/test-workspace is not a VCS dir, so the response body may carry
	// an error string, but the RPC itself returns nil.
	require.NoError(t, err, "GetVCSStatus should find the session by UUID")
}

// TestWorkspaceService_GetVCSStatus_FindsByTitle verifies that the legacy
// Title-based lookup still works after the UUID migration.
func TestWorkspaceService_GetVCSStatus_FindsByTitle(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "title-only-session",
		Path:    "/tmp/test-workspace",
		Status:  session.Paused, // Paused avoids tmux setup in LoadInstances (no CI tmux)
		Program: "claude",
	}))

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "title-only-session",
	}))

	require.NoError(t, err, "GetVCSStatus should still find the session by Title")
}

// TestWorkspaceService_GetVCSStatus_UnknownIDReturnsNotFound verifies that an
// ID that matches neither UUID nor Title of any session returns CodeNotFound.
func TestWorkspaceService_GetVCSStatus_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// LiveInstanceFinder fast-path tests
// --------------------------------------------------------------------------

// TestWorkspaceService_FindInstanceFast_LiveFinderHit_BypassesStorage verifies
// that when SetLiveFinder is wired and FindLiveInstance returns an instance,
// that instance is used directly without consulting storage.
func TestWorkspaceService_FindInstanceFast_LiveFinderHit_BypassesStorage(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	liveInst := &session.Instance{
		Title:   "live-session",
		Path:    "/tmp/live-workspace",
		Status:  session.Active,
		Program: "claude",
	}
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: liveInst})

	// Session is NOT in storage — only the live finder can resolve it.
	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "live-session",
	}))

	// GetVCSStatus will fail (no real VCS dir), but it must NOT be CodeNotFound —
	// that would mean the session wasn't resolved at all.
	require.NoError(t, err, "live instance with no working dir should return a response with an error field, not a gRPC error")
}

// TestWorkspaceService_FindInstanceFast_LiveFinderMiss_FallsBackToStorage verifies
// that when the live finder returns nil, findInstanceFast falls back to storage.
func TestWorkspaceService_FindInstanceFast_LiveFinderMiss_FallsBackToStorage(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Live finder always misses.
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: nil})

	// Session is in storage.
	seedInstance(t, fix.storage, "storage-session")

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "storage-session",
	}))

	// Same logic: a missing working dir produces a response with an error field,
	// but the session must be found (no CodeNotFound).
	require.NoError(t, err, "session found via storage fallback should not return a gRPC error")
}

// --------------------------------------------------------------------------
// fileChangeToProto: Additions/Deletions threading (numstat feature)
// --------------------------------------------------------------------------

// TestFileChangeToProto verifies that fileChangeToProto correctly threads
// every vc.FileChange field — including the Additions/Deletions numstat
// counts added alongside per-file insertion/deletion stats — through to the
// sessionv1.FileChange proto.
func TestFileChangeToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   vc.FileChange
		want *sessionv1.FileChange
	}{
		{
			name: "modified file threads additions and deletions",
			in: vc.FileChange{
				Path:      "main.go",
				Status:    vc.FileModified,
				IsStaged:  false,
				Additions: 12,
				Deletions: 3,
			},
			want: &sessionv1.FileChange{
				Path:      "main.go",
				Status:    sessionv1.FileStatus_FILE_STATUS_MODIFIED,
				IsStaged:  false,
				OldPath:   "",
				Additions: 12,
				Deletions: 3,
			},
		},
		{
			name: "staged rename threads old path and stats",
			in: vc.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    vc.FileRenamed,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
			want: &sessionv1.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_RENAMED,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
		},
		{
			name: "untracked file has zero additions/deletions",
			in: vc.FileChange{
				Path:      "new.txt",
				Status:    vc.FileUntracked,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
			want: &sessionv1.FileChange{
				Path:      "new.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_UNTRACKED,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fileChangeToProto(tt.in)

			assert.Equal(t, tt.want.Path, got.Path)
			assert.Equal(t, tt.want.OldPath, got.OldPath)
			assert.Equal(t, tt.want.Status, got.Status)
			assert.Equal(t, tt.want.IsStaged, got.IsStaged)
			assert.Equal(t, tt.want.Additions, got.Additions)
			assert.Equal(t, tt.want.Deletions, got.Deletions)
		})
	}
}

// --------------------------------------------------------------------------
// GetVCSStatus: commit list / aggregate diff stat / StatusAsOf (Story 1.1.3,
// proto mapping added by Story 2.2.1)
//
// These tests check both the internal vc.VCSStatus cached in
// WorkspaceService's vcsStatusCache and the RPC response's VcsStatus proto —
// the proto assertions are the real acceptance criterion ("all populate on
// every GetVCSStatus response"); the cache assertions remain to pin down the
// internal state the proto mapping is derived from.
// --------------------------------------------------------------------------

// newVCSStatusTestRepo creates a temp git repo with two commits on "main":
// baseSHA (one file) then headSHA (a second file added) — so a caller with
// baseSHA recorded sees exactly one unshipped commit and one changed file.
func newVCSStatusTestRepo(t *testing.T) (dir, baseSHA, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "base commit")
	baseSHA = strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "second commit")
	headSHA = strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
	return dir, baseSHA, headSHA
}

func TestWorkspaceService_GetVCSStatus_DirectoryMode_PopulatesCommitsAndDiffStat(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, baseSHA, headSHA := newVCSStatusTestRepo(t)

	inst := &session.Instance{Title: "dir-mode-session", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA(baseSHA)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	resp, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "dir-mode-session",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error)

	cached, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok, "GetVCSStatus should have cached the computed status")
	entry := cached.(vcsStatusCacheEntry)

	assert.Len(t, entry.status.Commits, 1, "one commit shipped past the recorded base")
	assert.False(t, entry.status.CommitsTruncated)
	assert.False(t, entry.status.CommitsUnavailable)
	require.NotNil(t, entry.status.AggregateDiffStat)
	assert.Equal(t, 1, entry.status.AggregateDiffStat.FilesChanged)
	assert.False(t, entry.status.StatusAsOf.IsZero(), "StatusAsOf should be set on fresh compute")

	vcsStatus := resp.Msg.VcsStatus
	require.NotNil(t, vcsStatus)
	require.Len(t, vcsStatus.Commits, 1, "one commit shipped past the recorded base")
	assert.Equal(t, headSHA, vcsStatus.Commits[0].Sha)
	assert.Equal(t, "second commit", vcsStatus.Commits[0].Summary)
	assert.NotNil(t, vcsStatus.Commits[0].AuthoredAt)
	assert.False(t, vcsStatus.CommitsTruncated)
	assert.False(t, vcsStatus.CommitsUnavailable)
	require.NotNil(t, vcsStatus.AggregateDiffStat)
	assert.Equal(t, int32(1), vcsStatus.AggregateDiffStat.FilesChanged)
	assert.Equal(t, int32(1), vcsStatus.AggregateDiffStat.Additions, "b.txt added one line")
	assert.Equal(t, int32(0), vcsStatus.AggregateDiffStat.Deletions)
	assert.NotNil(t, vcsStatus.StatusAsOf, "status_as_of should be set on fresh compute")
}

func TestWorkspaceService_GetVCSStatus_WorktreeMode_PopulatesCommitsAndDiffStat(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, baseSHA, headSHA := newVCSStatusTestRepo(t)

	// repoPath == worktreePath here, matching the pattern already used by
	// create_pull_request_test.go's newDraftPRTestRepo call sites — the test
	// only needs a valid git repo at the effective path, not an actual
	// `git worktree add` checkout.
	wt := git.NewGitWorktreeFromStorage(dir, dir, "wt-mode-session", "feature/x", baseSHA)
	inst := &session.Instance{Title: "wt-mode-session", Status: session.Paused, Program: "claude"}
	inst.SetGitWorktree(wt)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	resp, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "wt-mode-session",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error)

	// NewGitWorktreeFromStorage symlink-resolves worktreePath on construction
	// (session/git/worktree.go's CanonicalizeWorktreePath, needed so worktree-reuse
	// identity checks compare consistently), so the cache key GetVCSStatus stores
	// under is the resolved path -- resolve dir the same way before looking it up,
	// or this spuriously fails wherever TMPDIR itself is a symlink (e.g. macOS's
	// /var -> /private/var).
	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	cached, ok := fix.svc.vcsStatusCache.Load(resolvedDir)
	require.True(t, ok, "GetVCSStatus should have cached the computed status")
	entry := cached.(vcsStatusCacheEntry)

	assert.Len(t, entry.status.Commits, 1, "one commit shipped past the recorded base")
	assert.False(t, entry.status.CommitsUnavailable)
	require.NotNil(t, entry.status.AggregateDiffStat)
	assert.Equal(t, 1, entry.status.AggregateDiffStat.FilesChanged)
	assert.False(t, entry.status.StatusAsOf.IsZero(), "StatusAsOf should be set on fresh compute")

	vcsStatus := resp.Msg.VcsStatus
	require.NotNil(t, vcsStatus)
	require.Len(t, vcsStatus.Commits, 1, "one commit shipped past the recorded base")
	assert.Equal(t, headSHA, vcsStatus.Commits[0].Sha)
	assert.Equal(t, "second commit", vcsStatus.Commits[0].Summary)
	assert.False(t, vcsStatus.CommitsUnavailable)
	require.NotNil(t, vcsStatus.AggregateDiffStat)
	assert.Equal(t, int32(1), vcsStatus.AggregateDiffStat.FilesChanged)
	assert.Equal(t, int32(1), vcsStatus.AggregateDiffStat.Additions, "b.txt added one line")
	assert.Equal(t, int32(0), vcsStatus.AggregateDiffStat.Deletions)
	assert.NotNil(t, vcsStatus.StatusAsOf, "status_as_of should be set on fresh compute")
}

func TestWorkspaceService_GetVCSStatus_NoBaseSHA_LeavesCommitsAndDiffStatUnset(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, _, _ := newVCSStatusTestRepo(t)

	// No SetDirBaseSHA call: GetBaseCommitSHA() returns "".
	inst := &session.Instance{Title: "no-base-session", Path: dir, Status: session.Paused, Program: "claude"}
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	resp, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "no-base-session",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error)

	cached, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok)
	entry := cached.(vcsStatusCacheEntry)

	assert.Nil(t, entry.status.Commits)
	assert.Nil(t, entry.status.AggregateDiffStat)
	assert.False(t, entry.status.CommitsUnavailable, "no recorded base SHA is not a fetch failure")

	vcsStatus := resp.Msg.VcsStatus
	require.NotNil(t, vcsStatus)
	assert.Empty(t, vcsStatus.Commits)
	assert.Nil(t, vcsStatus.AggregateDiffStat)
	assert.False(t, vcsStatus.CommitsUnavailable, "no recorded base SHA is not a fetch failure")
	assert.NotNil(t, vcsStatus.StatusAsOf, "status_as_of should still be set even with no base SHA")
}

// TestWorkspaceService_GetVCSStatus_HeadResolutionFailure_SetsCommitsUnavailable
// uses a freshly `git init`ed repo with zero commits — HEAD is an unborn
// branch ref, so git.GetHeadCommitSHA fails via both its go-git and CLI
// fallback paths, while GitProvider.GetStatus still succeeds (git status
// works fine before the first commit). This exercises the same failure mode
// the plan's "point workDir at a path with no .git" example was after
// (GetHeadCommitSHA failing) without tripping GetVCSStatus's earlier
// NewGitProvider/GetStatus calls, which must succeed for this code path to
// even run.
func TestWorkspaceService_GetVCSStatus_HeadResolutionFailure_SetsCommitsUnavailable(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")

	inst := &session.Instance{Title: "unborn-head-session", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA("0000000000000000000000000000000000dead")
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	resp, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "unborn-head-session",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error, "GetStatus itself should still succeed on a commit-less repo")

	cached, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok)
	entry := cached.(vcsStatusCacheEntry)

	assert.True(t, entry.status.CommitsUnavailable)
	assert.Nil(t, entry.status.Commits)

	vcsStatus := resp.Msg.VcsStatus
	require.NotNil(t, vcsStatus)
	assert.True(t, vcsStatus.CommitsUnavailable)
	assert.Empty(t, vcsStatus.Commits)
}

// TestWorkspaceService_GetVCSStatus_ListShippedCommitsFailure_SetsCommitsUnavailable
// covers the sibling failure branch to the HEAD-resolution test above: HEAD
// resolves fine, but the recorded base SHA doesn't exist in the repo, so
// ListShippedCommits itself fails resolving baseSHA. Distinct code path
// (workspace_service.go's listErr != nil branch, not headErr != nil) — a
// regression here would silently render a stale/empty commit list
// indistinguishable from "genuinely zero commits."
func TestWorkspaceService_GetVCSStatus_ListShippedCommitsFailure_SetsCommitsUnavailable(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, _, _ := newVCSStatusTestRepo(t)

	inst := &session.Instance{Title: "bad-base-sha-session", Path: dir, Status: session.Paused, Program: "claude"}
	// HEAD resolves fine; this base SHA is well-formed but doesn't exist in
	// the repo, so ListShippedCommits's CommitObject(baseSHA) lookup fails.
	inst.SetDirBaseSHA("0000000000000000000000000000000000dead")
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	resp, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "bad-base-sha-session",
	}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Error)

	cached, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok)
	entry := cached.(vcsStatusCacheEntry)

	assert.True(t, entry.status.CommitsUnavailable)
	assert.Nil(t, entry.status.Commits)

	vcsStatus := resp.Msg.VcsStatus
	require.NotNil(t, vcsStatus)
	assert.True(t, vcsStatus.CommitsUnavailable)
	assert.Empty(t, vcsStatus.Commits)
}

func TestWorkspaceService_GetVCSStatus_CacheHit_ReturnsSameStatusAsOf(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, baseSHA, _ := newVCSStatusTestRepo(t)

	inst := &session.Instance{Title: "cache-hit-session", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA(baseSHA)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	req := connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "cache-hit-session"})

	resp1, err := fix.svc.GetVCSStatus(context.Background(), req)
	require.NoError(t, err)
	cached1, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok)
	entry1 := cached1.(vcsStatusCacheEntry)
	require.False(t, entry1.cachedAt.IsZero())

	// A short sleep so a wrongly-recomputed second call would produce a
	// detectably different time.Now() value, not one that happens to match
	// by coincidence.
	time.Sleep(5 * time.Millisecond)

	resp2, err := fix.svc.GetVCSStatus(context.Background(), req)
	require.NoError(t, err)
	cached2, ok := fix.svc.vcsStatusCache.Load(dir)
	require.True(t, ok)
	entry2 := cached2.(vcsStatusCacheEntry)

	assert.True(t, entry1.cachedAt.Equal(entry2.cachedAt),
		"second call within the 15s cache window must not recompute (cachedAt must not change)")
	assert.True(t, entry2.status.StatusAsOf.Equal(entry1.cachedAt),
		"cache-hit branch must set StatusAsOf from the cache entry's stored timestamp")

	require.NotNil(t, resp1.Msg.VcsStatus.StatusAsOf)
	require.NotNil(t, resp2.Msg.VcsStatus.StatusAsOf)
	assert.True(t, resp1.Msg.VcsStatus.StatusAsOf.AsTime().Equal(resp2.Msg.VcsStatus.StatusAsOf.AsTime()),
		"RPC response's status_as_of must also stay pinned to the cache entry's timestamp across a cache hit")
}

// TestWorkspaceService_GetVCSStatus_ConcurrentCacheHits_NoRace warms the cache
// with a single synchronous call, then fires concurrent reads that should ALL
// take the cache-hit fast path. It isolates whether the cache-hit branch's
// StatusAsOf write races with itself or with vcsStatusToProto's concurrent
// read of the same cached *vc.VCSStatus — run with `-race` to catch it.
func TestWorkspaceService_GetVCSStatus_ConcurrentCacheHits_NoRace(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	dir, baseSHA, _ := newVCSStatusTestRepo(t)
	inst := &session.Instance{Title: "race-warm-session", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA(baseSHA)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})

	req := connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "race-warm-session"})

	// Warm-up: populate the cache synchronously.
	if _, err := fix.svc.GetVCSStatus(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = fix.svc.GetVCSStatus(context.Background(), req)
		}()
	}
	wg.Wait()
}
