package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// forkTestFixture sets up a SessionService wired with a ReviewQueuePoller so
// that findInstance() works correctly (it requires the poller to be set).
type forkTestFixture struct {
	svc     *SessionService
	bus     *events.EventBus
	storage *session.Storage
	poller  *session.ReviewQueuePoller
	cleanup func()
}

func setupForkTestFixture(t *testing.T) *forkTestFixture {
	t.Helper()

	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	svc := NewSessionService(storage, bus)

	// t.Cleanup runs in LIFO order, so registering the bus teardown here —
	// before svc.Shutdown()'s cleanup below — guarantees Shutdown() (which
	// blocks on any in-flight trackCleanup-tracked goroutines, e.g. from
	// DeleteSession/Destroy()) always runs BEFORE the bus is closed. The repo
	// is closed via its own t.Cleanup registered inside NewTestEntRepository,
	// which (being registered first, earlier in this function) runs last.
	// Callers historically invoked the returned cleanup field themselves (via
	// t.Cleanup(fix.cleanup) or defer fix.cleanup()) which raced ahead of
	// Shutdown's own t.Cleanup — see
	// TestSessionRetentionSweeper_ConvergesWhenAllSiblingsBecomeEligible's
	// "Fail in goroutine after test has completed" panic. The field is now a
	// no-op so those existing call sites remain harmless.
	t.Cleanup(func() {
		bus.Close()
	})
	t.Cleanup(func() { svc.Shutdown() })

	// Wire the ReviewQueuePoller so findInstance() can resolve instances.
	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	return &forkTestFixture{
		svc:     svc,
		bus:     bus,
		storage: storage,
		poller:  poller,
		cleanup: func() {},
	}
}

// addInstanceToPoller registers a session instance directly with the poller so
// that findInstance() can resolve it by title (the normal production path).
func addInstanceToPoller(poller *session.ReviewQueuePoller, inst *session.Instance) {
	poller.SetInstances(append(poller.GetInstances(), inst))
}

// makeInstanceWithCheckpoint creates a minimal Instance that has one checkpoint.
func makeInstanceWithCheckpoint(title string) (*session.Instance, string) {
	cpID := "test-checkpoint-id"
	inst := &session.Instance{
		Title:   title,
		Path:    "/tmp/test",
		Status:  session.Active,
		Program: "claude",
		Checkpoints: session.CheckpointList{
			{
				ID:        cpID,
				Label:     "baseline",
				Timestamp: time.Now(),
			},
		},
	}
	return inst, cpID
}

// --------------------------------------------------------------------------
// Input validation tests
// --------------------------------------------------------------------------

func TestForkSession_MissingSessionID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "",
		CheckpointId: "cp-1",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_MissingCheckpointID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "some-session",
		CheckpointId: "",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_MissingNewTitle(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "some-session",
		CheckpointId: "cp-1",
		NewTitle:     "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestForkSession_SessionNotFound(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "nonexistent-session",
		CheckpointId: "cp-1",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

func TestForkSession_DuplicateTitle(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Register two instances: source "src" and another with the would-be fork title "fork".
	src, _ := makeInstanceWithCheckpoint("src")
	existing, _ := makeInstanceWithCheckpoint("fork")
	addInstanceToPoller(fix.poller, src)
	addInstanceToPoller(fix.poller, existing)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: "cp-1",
		NewTitle:     "fork", // already exists
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}

func TestForkSession_CheckpointNotFound(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	src, _ := makeInstanceWithCheckpoint("src")
	addInstanceToPoller(fix.poller, src)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: "no-such-checkpoint",
		NewTitle:     "forked",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	// ForkFromCheckpoint returns an error when checkpoint is not found, which
	// the handler wraps as CodeFailedPrecondition.
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

func TestForkSession_Success(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	src, cpID := makeInstanceWithCheckpoint("src")
	addInstanceToPoller(fix.poller, src)

	resp, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    "src",
		CheckpointId: cpID,
		NewTitle:     "forked",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "forked", resp.Msg.Session.Title)
}

// --------------------------------------------------------------------------
// GetSessionDiff – UUID ID lookup (regression: frontend passes UUIDs)
// --------------------------------------------------------------------------

// TestGetSessionDiff_FindsByUUID verifies that GetSessionDiff resolves the
// incoming session ID via UUID using the ReviewQueuePoller's MatchesID check.
// Before the fix, only Title was matched, so UUID callers got CodeNotFound.
func TestGetSessionDiff_FindsByUUID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	const testUUID = "22222222-2222-2222-2222-222222222222"
	addInstanceToPoller(fix.poller, &session.Instance{
		UUID:    testUUID,
		Title:   "diff-session",
		Path:    "/tmp/test-diff",
		Status:  session.Active,
		Program: "claude",
	})

	resp, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: testUUID,
	}))

	// Must NOT return CodeNotFound. /tmp/test-diff is not a git repo, so the
	// diff will be empty — but the session must be found by UUID.
	require.NoError(t, err, "GetSessionDiff should find the session by UUID")
	require.NotNil(t, resp)
}

// TestGetSessionDiff_FindsByTitle verifies that legacy Title-based lookups
// still work after the UUID migration.
func TestGetSessionDiff_FindsByTitle(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addInstanceToPoller(fix.poller, &session.Instance{
		Title:   "diff-by-title",
		Path:    "/tmp/test-diff-title",
		Status:  session.Active,
		Program: "claude",
	})

	resp, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: "diff-by-title",
	}))

	require.NoError(t, err, "GetSessionDiff should still find the session by Title")
	require.NotNil(t, resp)
}

// TestGetSessionDiff_UnknownIDReturnsNotFound verifies that an ID matching no
// session UUID or Title produces CodeNotFound.
func TestGetSessionDiff_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestGetSessionDiff_CompletedDirectoryModeSessionReturnsRealDiff guards the
// fix for a completed (not live) session with no git worktree
// (Worktree.WorktreePath == "") but a real Path — a "directory" session.
// Before the fix, GetSessionDiff's completed-session branch only computed
// diff stats when Worktree.WorktreePath was set, so a directory-mode
// session's real changes were silently discarded in favor of an empty
// DiffStats{} — this must fail against that pre-fix code (no else-if branch
// at all) and pass now that one exists.
func TestGetSessionDiff_CompletedDirectoryModeSessionReturnsRealDiff(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "new-file.txt"), []byte("uncommitted change\n"), 0o644))

	// Persisted (not live — never registered with the poller), directory-mode:
	// Path set, no Worktree. GetSessionDiff's findInstance() must miss this
	// session and fall through to the completed-session storage lookup.
	const testUUID = "33333333-3333-3333-3333-333333333333"
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		UUID:    testUUID,
		Title:   "directory-mode-diff-session",
		Path:    repoPath,
		Status:  session.Stopped,
		Program: "claude",
	}))

	resp, err := fix.svc.GetSessionDiff(context.Background(), connect.NewRequest(&sessionv1.GetSessionDiffRequest{
		Id: testUUID,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.DiffStats)
	assert.Positive(t, resp.Msg.DiffStats.Added,
		"a completed directory-mode session's real uncommitted changes must be reflected in the diff, not silently discarded")
}
