package workspacerpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// ---------------------------------------------------------------------------
// GetWorkspaceInfo
// ---------------------------------------------------------------------------

// TestGetWorkspaceInfo_EmptyID verifies that GetWorkspaceInfo returns
// CodeInvalidArgument when the session id is empty.
func TestGetWorkspaceInfo_EmptyID(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&instance.Instance{
		Title:   "non-vcs-session",
		Path:    "/tmp",
		Status:  domain.Paused,
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&instance.Instance{
		Title:   "non-vcs-targets-session",
		Path:    "/tmp",
		Status:  domain.Paused,
		Program: "claude",
	}))

	resp, err := fix.svc.ListWorkspaceTargets(context.Background(), connect.NewRequest(&sessionv1.ListWorkspaceTargetsRequest{
		Id: "non-vcs-targets-session",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
}
