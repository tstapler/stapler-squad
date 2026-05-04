package services

import (
	"context"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// setupUWSFixture creates an UnfinishedWorkService wired with real SQLite storage,
// a Scanner, and a StateStore backed by a temp directory.
func setupUWSFixture(t *testing.T) (svc *UnfinishedWorkService, cleanup func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "uws-test-*")
	require.NoError(t, err)

	stateStore, err := unfinished.NewStateStore(tmpDir + "/unfinished-state.json")
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	scanner := unfinished.NewScanner(bus, stateStore)

	storage := createTestStorage(t)

	svc = NewUnfinishedWorkService(scanner, stateStore, bus, storage)

	cleanup = func() {
		bus.Close()
		os.RemoveAll(tmpDir)
	}
	return
}

// --------------------------------------------------------------------------
// GetUnfinishedWorkConfig
// --------------------------------------------------------------------------

// TestGetUnfinishedWorkConfig_ReturnsConfig verifies that GetUnfinishedWorkConfig
// returns a valid config without error on a fresh service.
func TestGetUnfinishedWorkConfig_ReturnsConfig(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	resp, err := svc.GetUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.GetUnfinishedWorkConfigRequest{}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Config, "config must not be nil")
}

// --------------------------------------------------------------------------
// UpdateUnfinishedWorkConfig
// --------------------------------------------------------------------------

// TestUpdateUnfinishedWorkConfig_NilConfigReturnsError verifies that passing a
// nil config returns CodeInvalidArgument.
func TestUpdateUnfinishedWorkConfig_NilConfigReturnsError(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.UpdateUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
			Config: nil,
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpdateUnfinishedWorkConfig_ValidConfig verifies that a well-formed config
// update succeeds and the returned config reflects the submitted values.
func TestUpdateUnfinishedWorkConfig_ValidConfig(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	cfg := &sessionv1.UnfinishedWorkConfig{
		AutoSpiderSessions: false,
		WatchDirs:          []string{},
		PinnedRepos:        []string{},
	}

	resp, err := svc.UpdateUnfinishedWorkConfig(
		context.Background(),
		connect.NewRequest(&sessionv1.UpdateUnfinishedWorkConfigRequest{
			Config: cfg,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Config)
	require.Equal(t, false, resp.Msg.Config.AutoSpiderSessions)
}

// --------------------------------------------------------------------------
// GetWorktreeAISummary
// --------------------------------------------------------------------------

// TestGetWorktreeAISummary_UnknownWorktree verifies that requesting an AI
// summary for a worktree that has not been scanned returns CodeNotFound.
func TestGetWorktreeAISummary_UnknownWorktree(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.GetWorktreeAISummary(
		context.Background(),
		connect.NewRequest(&sessionv1.GetWorktreeAISummaryRequest{
			RepoPath: "/nonexistent/repo",
			Branch:   "main",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// QuickCommitPush
// --------------------------------------------------------------------------

// TestQuickCommitPush_EmptyCommitMessage verifies that an empty commit message
// returns CodeInvalidArgument.
func TestQuickCommitPush_EmptyCommitMessage(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.QuickCommitPush(
		context.Background(),
		connect.NewRequest(&sessionv1.QuickCommitPushRequest{
			RepoPath:      "/some/repo",
			Branch:        "main",
			CommitMessage: "",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestQuickCommitPush_UnknownWorktree verifies that a valid commit message but
// an untracked worktree returns CodeNotFound.
func TestQuickCommitPush_UnknownWorktree(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.QuickCommitPush(
		context.Background(),
		connect.NewRequest(&sessionv1.QuickCommitPushRequest{
			RepoPath:      "/nonexistent/repo",
			Branch:        "main",
			CommitMessage: "wip: test commit",
		}),
	)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// UndismissWorktree
// --------------------------------------------------------------------------

// TestUndismissWorktree_NoOpOnUnknown verifies that undismissing a worktree
// that was never dismissed returns no error (it is a no-op).
func TestUndismissWorktree_NoOpOnUnknown(t *testing.T) {
	svc, cleanup := setupUWSFixture(t)
	t.Cleanup(cleanup)

	_, err := svc.UndismissWorktree(
		context.Background(),
		connect.NewRequest(&sessionv1.UndismissWorktreeRequest{
			RepoPath: "/some/repo",
			Branch:   "main",
		}),
	)

	// Undismiss of an unknown entry is expected to succeed (no-op).
	require.NoError(t, err)
}
