package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// ---------------------------------------------------------------------------
// resolveSessionType – pure routing logic, no I/O
// ---------------------------------------------------------------------------

func TestResolveSessionType_ExplicitDirectory(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_ExplicitNewWorktree(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
	}
	assert.Equal(t, session.SessionTypeNewWorktree, resolveSessionType(msg, "my-branch"))
}

func TestResolveSessionType_ExplicitExistingWorktree(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE,
		ExistingWorktree: "/some/worktree",
	}
	assert.Equal(t, session.SessionTypeExistingWorktree, resolveSessionType(msg, ""))
}

func TestResolveSessionType_UnspecifiedDefaultsToDirectory(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_UnspecifiedBranchInfersNewWorktree(t *testing.T) {
	// Backward-compat: a resolved branch with no explicit session_type → new_worktree.
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
	}
	assert.Equal(t, session.SessionTypeNewWorktree, resolveSessionType(msg, "feat/my-feature"))
}

func TestResolveSessionType_UnspecifiedExistingWorktreeInfersExistingWorktree(t *testing.T) {
	// Backward-compat: ExistingWorktree field present → existing_worktree (takes priority over branch).
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED,
		ExistingWorktree: "/path/to/worktree",
	}
	assert.Equal(t, session.SessionTypeExistingWorktree, resolveSessionType(msg, "feat/branch"))
}

func TestResolveSessionType_OneOffOverridesExplicitNewWorktree(t *testing.T) {
	// one_off flag wins over any explicit SessionType.
	msg := &sessionv1.CreateSessionRequest{
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		OneOff:      true,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, "some-branch"))
}

func TestResolveSessionType_OneOffOverridesExistingWorktree(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		SessionType:      sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE,
		ExistingWorktree: "/worktree",
		OneOff:           true,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_OneOffUnspecifiedIsDirectory(t *testing.T) {
	msg := &sessionv1.CreateSessionRequest{
		OneOff: true,
	}
	assert.Equal(t, session.SessionTypeDirectory, resolveSessionType(msg, ""))
}

func TestResolveSessionType_UnknownExplicitTypeDefaultsToDirectory(t *testing.T) {
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
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:  "my-session",
		Path:   "",
		OneOff: false,
	}))

	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "path is required")
}

func TestCreateSession_EmptyPath_OneOff_PassesPathValidation(t *testing.T) {
	// one_off=true must NOT fail with "path is required".
	// If tmux is available the call succeeds (err == nil); if not, it fails with
	// CodeInternal at the tmux step — either way, CodeInvalidArgument must not appear.
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	baseDir := t.TempDir()
	t.Setenv("HOME", baseDir)

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:  "scratch-session",
		Path:   "",
		OneOff: true,
	}))

	if err != nil {
		assertNotConnectCode(t, err, connect.CodeInvalidArgument, "one-off session must not fail path validation")
	} else {
		// tmux is available: session created successfully — clean it up.
		require.NotNil(t, resp.Msg.Session)
		destroyCreatedSession(t, svc, resp.Msg.Session.Id)
	}
}

func TestCreateSession_DuplicateTitle_ReturnsAlreadyExists(t *testing.T) {
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

	baseDir := t.TempDir()
	t.Setenv("HOME", baseDir) // ~/oneoff resolves under here

	expectedBase := filepath.Join(baseDir, "oneoff")

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:  "my-scratch",
		Path:   "",
		OneOff: true,
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

	baseDir := t.TempDir()
	t.Setenv("HOME", baseDir)
	expectedBase := filepath.Join(baseDir, "oneoff")

	for i, title := range []string{"session-a", "session-b"} {
		resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
			Title:  title,
			OneOff: true,
		}))
		if err == nil {
			destroyCreatedSession(t, svc, resp.Msg.Session.Id)
		}
		entries, err := os.ReadDir(expectedBase)
		require.NoError(t, err)
		assert.Len(t, entries, i+1, "after %d one-off creation(s), should have %d directories", i+1, i+1)
	}
}

func TestCreateSession_OneOff_BadBaseDir_ReturnsInternalError(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	// Point HOME at a file (not a directory) so ~/oneoff cannot be created.
	tmpFile, err := os.CreateTemp("", "not-a-dir-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	// Set HOME to the file's directory and give an explicit base that is the file itself.
	// We do this by setting ONE_OFF_BASE_DIR via config — but since config is loaded
	// from disk and we can't inject it here, we instead make the HOME trick: point HOME
	// to a path whose parent does not allow mkdir.
	//
	// Simpler: make the base dir a regular file so os.MkdirAll fails.
	bogusHome := tmpFile.Name() // HOME = a file; ~/oneoff = file + "/oneoff" which can't be created
	t.Setenv("HOME", bogusHome)

	_, err = svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:  "cant-make-dir",
		OneOff: true,
	}))

	require.Error(t, err)
	// Should be CodeInternal (failed to create one-off directory), not CodeInvalidArgument.
	assertConnectCode(t, err, connect.CodeInternal)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newCreateTestService(t *testing.T, storage *session.Storage) *SessionService {
	t.Helper()
	bus := events.NewEventBus(16)
	svc := NewSessionService(storage, bus)
	return svc
}

// destroyCreatedSession cleans up a session that was successfully created during a test.
// Errors are soft-logged so cleanup failures don't mask the actual test assertion.
func destroyCreatedSession(t *testing.T, svc *SessionService, id string) {
	t.Helper()
	_, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: id}))
	if err != nil {
		t.Logf("destroyCreatedSession: cleanup for %q failed (non-fatal): %v", id, err)
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
