package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// ---------------------------------------------------------------------------
// resolveSessionType – pure routing logic, no I/O
// ---------------------------------------------------------------------------

func TestResolveSessionType_ExplicitDirectory(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_ExplicitNewWorktree(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
	}
	assert.Equal(t, session.SessionTypeNewWorktree, resolveSessionType(msg, "my-branch"))
}

func TestResolveSessionType_ExplicitExistingWorktree(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE,
		ExistingWorktree: "/some/worktree",
	}
	assert.Equal(t, session.SessionTypeExistingWorktree, resolveSessionType(msg, ""))
}

func TestResolveSessionType_UnspecifiedDefaultsToDirectory(t *testing.T) {
	t.Parallel()
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_UnspecifiedBranchInfersNewWorktree(t *testing.T) {
	t.Parallel()
	// Backward-compat: a resolved branch with no explicit session_type → new_worktree.
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	assert.Equal(t, session.SessionTypeNewWorktree, resolveSessionType(msg, "feat/my-feature"))
}

func TestResolveSessionType_UnspecifiedExistingWorktreeInfersExistingWorktree(t *testing.T) {
	t.Parallel()
	// Backward-compat: ExistingWorktree field present → existing_worktree (takes priority over branch).
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
		ExistingWorktree: "/path/to/worktree",
	}
	assert.Equal(t, session.SessionTypeExistingWorktree, resolveSessionType(msg, "feat/branch"))
}

func TestResolveSessionType_OneOff_ReturnsSessionTypeOneOff(t *testing.T) {
	t.Parallel()
	// SESSION_TYPE_ONE_OFF maps to SessionTypeOneOff (caller converts to directory after path gen).
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
	}
	assert.Equal(t, session.SessionTypeOneOff, resolveSessionType(msg, "some-branch"))
}

func TestResolveSessionType_UnknownExplicitTypeDefaultsToDirectory(t *testing.T) {
	t.Parallel()
	// A proto enum value we don't recognise yet should degrade gracefully.
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType(999),
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

// ---------------------------------------------------------------------------
// CreateSession – request validation (errors returned before tmux is touched)
// ---------------------------------------------------------------------------

func TestCreateSession_EmptyTitle_ReturnsInvalidArgument(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "",
		Path:  t.TempDir(),
	}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "title is required")
}

func TestCreateSession_EmptyPath_NonOneOff_ReturnsInvalidArgument(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "my-session",
		Path:  "",
	}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "path is required")
}

func TestCreateSession_should_RejectAutoApprove_When_ProgramUnsupported(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "my-session",
		Path:        t.TempDir(),
		Program:     "codex",
		AutoApprove: true,
	}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "auto_approve")
}

func TestCreateSession_should_SetAutoApprove_When_ProgramIsClaude(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "my-session",
		Path:        t.TempDir(),
		Program:     "claude",
		AutoApprove: true,
	}))

	require.NoError(t, err)
	assert.True(t, resp.Msg.Session.AutoApprove)
}

func TestCreateSession_EmptyPath_OneOff_PassesPathValidation(t *testing.T) {
	// one_off=true must NOT fail with "path is required".
	// If tmux is available the call succeeds (err == nil); if not, it fails with
	// CodeInternal at the tmux step — either way, CodeInvalidArgument must not appear.
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	withFakeHome(t)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "scratch-session",
		Path:        "",
		SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
	}))

	if err != nil {
		assertNotConnectCode(t, err, connect.CodeInvalidArgument, "one-off session must not fail path validation")
	} else {
		// tmux is available: session created successfully — clean it up.
		require.NotNil(t, resp.Msg.Session)
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}
}

// TestCreateSession_EmptyPath_Autonomous_PassesPathValidation is a regression test for
// a bug where autonomous sessions created via the omnibar (SessionType=DIRECTORY,
// AutonomousMode=true, Path="") were rejected with "path is required": the guard
// only exempted SESSION_TYPE_ONE_OFF, not AutonomousMode, even though the omnibar
// always sends an empty path for autonomous sessions and relies on the server to
// generate a scratch directory (mirroring one-off).
func TestCreateSession_EmptyPath_Autonomous_PassesPathValidation(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	withFakeHome(t)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:          "autonomous-session",
		Path:           "",
		SessionType:    sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
		AutonomousMode: true,
	}))

	if err != nil {
		assertNotConnectCode(t, err, connect.CodeInvalidArgument, "autonomous session must not fail path validation")
	} else {
		require.NotNil(t, resp.Msg.Session)
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}
}

// TestCreateSession_Autonomous_ExplicitPath_DoesNotGenerateScratchDir guards the
// asymmetry between the path guard (which exempts AutonomousMode unconditionally)
// and the directory-generation block (which only fires when resolvedPath == "").
// An autonomous request that supplies its own path must use that path as-is, not
// have it silently replaced by a generated ~/oneoff scratch directory.
func TestCreateSession_Autonomous_ExplicitPath_DoesNotGenerateScratchDir(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	baseDir := withFakeHome(t)
	explicitPath := t.TempDir()
	oneOffDir := filepath.Join(baseDir, "oneoff")

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:          "autonomous-explicit-path-test",
		Path:           explicitPath,
		SessionType:    sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
		AutonomousMode: true,
	}))
	if err == nil {
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}

	_, statErr := os.Stat(oneOffDir)
	assert.True(t, os.IsNotExist(statErr), "autonomous session with an explicit path must not generate a scratch directory")
}

func TestCreateSession_DuplicateTitle_ReturnsAlreadyExists(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	// Seed an existing session with the same title.
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:   "duplicate-name",
		Path:    t.TempDir(),
		Program: "claude",
		Status:  session.Paused,
	}))

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "duplicate-name",
		Path:  t.TempDir(),
	}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeAlreadyExists)
	assert.Contains(t, err.Error(), "duplicate-name")
}

func TestCreateSession_EmptyTitleAndPath_TitleErrorFirst(t *testing.T) {
	t.Parallel()
	// Both title and path are missing; title validation must fire first.
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "title is required")
}

// ---------------------------------------------------------------------------
// CreateSession – one-off directory creation (observable side-effect before tmux)
// ---------------------------------------------------------------------------

func TestCreateSession_OneOff_CreatesDirectoryInBaseDir(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	baseDir := withFakeHome(t) // ~/oneoff resolves under here

	expectedBase := filepath.Join(baseDir, "oneoff")

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "my-scratch",
		Path:        "",
		SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
	}))
	if err == nil {
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}

	// Whether or not tmux started, the generated directory must have been created.
	entries, err := os.ReadDir(expectedBase)
	require.NoError(t, err, "one-off base dir should have been created")
	require.Len(t, entries, 1, "exactly one generated directory should exist")

	name := entries[0].Name()
	assert.True(t, entries[0].IsDir(), "generated entry should be a directory")
	// Format: YYYYMMDD-adj-noun-NN
	parts := strings.Split(name, "-")
	require.GreaterOrEqual(t, len(parts), 4, "name should have at least 4 hyphen-separated parts: got %q", name)
	assert.Len(t, parts[0], 8, "first part should be an 8-digit date: got %q", parts[0])
}

func TestCreateSession_OneOff_TwoCallsCreateTwoDistinctDirectories(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	baseDir := withFakeHome(t)
	expectedBase := filepath.Join(baseDir, "oneoff")

	for i, title := range []string{"session-a", "session-b"} {
		resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
			Title:       title,
			SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
		}))
		if err == nil {
			destroyCreatedSession(t, svc, resp.Msg.Session.Id)
		}
		entries, err := os.ReadDir(expectedBase)
		require.NoError(t, err)
		assert.Len(t, entries, i+1, "after %d one-off creation(s), should have %d directories", i+1, i+1)
	}
}

// TestCreateSession_Autonomous_CreatesDirectoryInBaseDir verifies that, like one-off
// sessions, an autonomous session with an empty path gets a generated scratch
// directory rather than being rejected before directory-generation logic runs.
func TestCreateSession_Autonomous_CreatesDirectoryInBaseDir(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	baseDir := withFakeHome(t)
	expectedBase := filepath.Join(baseDir, "oneoff")

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:          "autonomous-scratch",
		Path:           "",
		SessionType:    sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
		AutonomousMode: true,
	}))
	if err == nil {
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}

	// Whether or not tmux started, the generated directory must have been created.
	entries, err := os.ReadDir(expectedBase)
	require.NoError(t, err, "autonomous session should generate a scratch directory")
	require.Len(t, entries, 1, "exactly one generated directory should exist")
	assert.True(t, entries[0].IsDir(), "generated entry should be a directory")
}

func TestCreateSession_OneOff_BadBaseDir_ReturnsInternalError(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	// Point HOME at a file (not a directory) so ~/oneoff cannot be created.
	withFakeHomeAsFile(t)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "cant-make-dir",
		SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF,
	}))

	require.Error(t, err)
	// Should be CodeInternal (failed to create one-off directory), not CodeInvalidArgument.
	assertConnectCode(t, err, connect.CodeInternal)
}

// ---------------------------------------------------------------------------
// Request-scoped timeout (AC5)
// ---------------------------------------------------------------------------

// TestCreateSessionTimeout_ComfortablyAboveGitCloneBound guards against the
// constant silently regressing below the documented worst case (git clone up
// to ~120s) — see the comment on createSessionTimeout.
func TestCreateSessionTimeout_ComfortablyAboveGitCloneBound(t *testing.T) {
	t.Parallel()
	assert.Greater(t, createSessionTimeout, 120*time.Second)
}

// TestCreateSession_GitHubURLResolution_BoundedByContext verifies that
// CreateSession's GitHub URL resolution step (the one known synchronous
// sub-operation that can block for up to ~120s on a real clone) is bound to
// the request context rather than able to hang the RPC forever. An
// already-expired context is threaded all the way down to
// safeexec.CommandContext for the underlying `git clone` subprocess, so the
// subprocess itself should be killed at/near start rather than merely having
// the RPC give up while a clone keeps running in the background. This makes
// the test deterministic and fast: ctx.Done() is already closed before the
// subprocess can do any meaningful network I/O.
func TestCreateSession_GitHubURLResolution_BoundedByContext(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already expired before CreateSession even starts

	start := time.Now()
	_, err := svc.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "github-timeout-test",
		Path:  "https://github.com/octocat/Hello-World",
	}))
	elapsed := time.Since(start)

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeDeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second, "CreateSession must fail fast on an expired context, not hang on GitHub resolution")
}

// ---------------------------------------------------------------------------
// Status manager wiring regression
// ---------------------------------------------------------------------------

// TestCreateSession_StatusManagerWiredBeforeDriver is a regression test for the bug
// where CreateSession's async goroutine never called instance.SetStatusManager before
// StartSessionDriver, so GetDetectedStatus() always returned StatusUnknown on newly
// created sessions (rather than e.g. StatusNeedsApproval when a "Do you want to
// proceed?" permission prompt was displayed).
//
// The fix (server/services/session_service.go) wires the status manager inside the
// goroutine before the driver starts. This test verifies that wiring happens within
// a reasonable time after the RPC returns.
//
// Requires tmux to be installed; skipped automatically otherwise.
func TestCreateSession_StatusManagerWiredBeforeDriver(t *testing.T) {
	// Not t.Parallel(): this test's goleak.IgnoreCurrent()/VerifyNone() baseline
	// would be polluted by goroutines started/stopped in sibling parallel tests.
	baseline := goleak.IgnoreCurrent()
	// Registered first so it runs LAST (t.Cleanup is LIFO) — after
	// destroyCreatedSession/svc.Shutdown/bus.Close have fully torn everything down.
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })

	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	// Wire a status manager AND a poller so FindLiveInstance resolves the live pointer.
	statusMgr := session.NewInstanceStatusManager()
	queue := session.NewReviewQueue()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetStatusManager(statusMgr)

	// CreateSession returns no error when tmux is absent — the async goroutine
	// handles the failure and sets Status=Stopped. A sync error here means a
	// pre-goroutine validation failure (storage, config) which should never be silently skipped.
	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "status-wiring-regression",
		Path:    t.TempDir(),
		Program: "claude",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	// The instance is added to the live poller synchronously (before the goroutine fires),
	// so FindLiveInstance is available immediately after CreateSession returns.
	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst, "instance must appear in live poller immediately after CreateSession")

	// Poll until the status manager is wired (tmux-available path) or the ceiling is hit.
	// GetStatusManager uses atomic.Pointer.Load(), so polling is race-free.
	// We avoid testify's Eventually here because its condition runs in a goroutine,
	// which prevents t.Skip from working correctly.
	var managerWired bool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if inst.GetStatusManager() != nil {
			managerWired = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !managerWired {
		// When tmux is absent the goroutine sets Status=Stopped and returns early,
		// never reaching SetStatusManager. By 30 s the goroutine is long finished.
		if session.Status(inst.GetStatus()) == session.Stopped {
			t.Skip("tmux not available; skipping status-manager wiring assertion")
		}
		t.Error("status manager was never wired within 30 s — regression in CreateSession goroutine")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newCreateTestService(t *testing.T, storage *session.Storage) *SessionService {
	t.Helper()
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	// Wire a ReviewQueuePoller so FindLiveInstance (used by destroyCreatedSession to
	// join the driver goroutine) resolves the live instance instead of always nil.
	statusMgr := session.NewInstanceStatusManager()
	queue := session.NewReviewQueue()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	return svc
}

// destroyCreatedSession cleans up a session that was successfully created during a test.
// Errors are soft-logged so cleanup failures don't mask the actual test assertion.
//
// It waits for DeleteSession's background Destroy() cleanup (waitForPendingCleanup)
// before returning: these tests set HOME to a t.TempDir(), and since t.Cleanup runs
// LIFO, that TempDir's RemoveAll (registered after the service, later in the test
// body) would otherwise race Destroy() for files under HOME (e.g. the tmux exec-gate
// directory) — see waitForPendingCleanup's doc comment.
//
// It also joins the session's SessionDriver goroutine (session.JoinSessionDriver):
// Destroy()'s call to StopSessionDriver only bounds its wait to driverStopTimeout and
// proceeds anyway on timeout, so without this join the driver goroutine can still be
// polling Preview() — which resolves the tmux exec-gate directory via the process-wide
// HOME/config dir — after waitForPendingCleanup returns and the test's t.TempDir() is
// removed, intermittently producing "directory not empty" from RemoveAll.
func destroyCreatedSession(t *testing.T, svc *SessionService, id string) {
	t.Helper()
	inst := svc.FindLiveInstance(id)
	_, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: id}))
	if err != nil {
		t.Logf("destroyCreatedSession: cleanup for %q failed (non-fatal): %v", id, err)
	}
	svc.waitForPendingCleanup()
	if inst != nil {
		session.JoinSessionDriver(inst)
	}
}

func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce, "expected a connect.Error")
	assert.Equal(t, want, ce.Code(), "expected connect code %v, got %v", want, ce.Code())
}

func assertNotConnectCode(t *testing.T, err error, notWant connect.Code, msg string) {
	t.Helper()
	var ce *connect.Error
	if !assert.ErrorAs(t, err, &ce) {
		return
	}
	assert.NotEqual(t, notWant, ce.Code(), msg)
}
