package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// ---------------------------------------------------------------------------
// GetWorkspaceInfo
// ---------------------------------------------------------------------------

// TestGetWorkspaceInfo_EmptyID verifies that GetWorkspaceInfo returns
// CodeInvalidArgument when the session id is empty.
func TestGetWorkspaceInfo_EmptyID(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetWorkspaceInfo(context.Background(), connect.NewRequest(&sessionv1.GetWorkspaceInfoRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestGetWorkspaceInfo_SessionNotFound verifies that GetWorkspaceInfo returns
// CodeNotFound when no session matches the given id.
func TestGetWorkspaceInfo_SessionNotFound(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetWorkspaceInfo(context.Background(), connect.NewRequest(&sessionv1.GetWorkspaceInfoRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestGetWorkspaceInfo_NoSession verifies that GetWorkspaceInfo returns a
// successful response (with an error field in the body) for a session that
// exists but whose path is not a VCS repository.
func TestGetWorkspaceInfo_NoSession(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "non-vcs-session",
		Path:    "/tmp",
		Status:  session.Paused,
		Program: "claude",
	}))

	// GetWorkspaceInfo returns nil error even when the path is not a VCS dir;
	// the error is encoded in the response body's Error field.
	resp, err := fix.svc.GetWorkspaceInfo(context.Background(), connect.NewRequest(&sessionv1.GetWorkspaceInfoRequest{
		Id: "non-vcs-session",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
}

// ---------------------------------------------------------------------------
// ListWorkspaceTargets
// ---------------------------------------------------------------------------

// TestListWorkspaceTargets_EmptyID verifies that ListWorkspaceTargets returns
// CodeInvalidArgument when the session id is empty.
func TestListWorkspaceTargets_EmptyID(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ListWorkspaceTargets(context.Background(), connect.NewRequest(&sessionv1.ListWorkspaceTargetsRequest{
		Id: "",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestListWorkspaceTargets_SessionNotFound verifies that ListWorkspaceTargets
// returns CodeNotFound when no session matches the given id.
func TestListWorkspaceTargets_SessionNotFound(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.ListWorkspaceTargets(context.Background(), connect.NewRequest(&sessionv1.ListWorkspaceTargetsRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestListWorkspaceTargets_NoConfig verifies that ListWorkspaceTargets returns
// a response (with an error in the body) for a session whose path has no VCS
// configuration. The RPC must not return a connect error in this case.
func TestListWorkspaceTargets_NoConfig(t *testing.T) {
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "non-vcs-targets-session",
		Path:    "/tmp",
		Status:  session.Paused,
		Program: "claude",
	}))

	resp, err := fix.svc.ListWorkspaceTargets(context.Background(), connect.NewRequest(&sessionv1.ListWorkspaceTargetsRequest{
		Id: "non-vcs-targets-session",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
}

// ---------------------------------------------------------------------------
// ListWorktrees (PathCompletionService)
// ---------------------------------------------------------------------------

// TestListWorktrees_EmptyPath verifies that ListWorktrees with an empty
// repo_path does not return an error. When an empty path is provided,
// the implementation uses the working directory for the git command.
// The response may contain worktrees (if the cwd is a git repo) or be empty —
// both are valid; the important invariant is no error.
func TestListWorktrees_EmptyPath(t *testing.T) {
	svc := NewPathCompletionService()

	resp, err := svc.ListWorktrees(context.Background(), connect.NewRequest(&sessionv1.ListWorktreesRequest{
		RepoPath: "",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	// Response may or may not contain worktrees depending on the cwd; no error is the invariant.
}

// TestListWorktrees_NonGitPath verifies that ListWorktrees returns an empty
// list (not an error) when the given path is not a git repository.
func TestListWorktrees_NonGitPath(t *testing.T) {
	svc := NewPathCompletionService()

	resp, err := svc.ListWorktrees(context.Background(), connect.NewRequest(&sessionv1.ListWorktreesRequest{
		RepoPath: t.TempDir(),
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Worktrees)
}
