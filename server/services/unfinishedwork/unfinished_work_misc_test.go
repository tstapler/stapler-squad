package unfinishedwork

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// The worktreeHeadSHA tests swap the process-wide git backend, so they are not parallel.

const headSHA = "0123456789abcdef0123456789abcdef01234567"

func TestWorktreeHeadSHA_RoutesResolveRefThroughBackend(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return headSHA, nil }

	assert.Equal(t, headSHA, worktreeHeadSHA(context.Background(), "/wt"))
	rig.RequireOnlyGoGit(t, backend.OpResolveRef)
	assert.Equal(t, "HEAD", rig.GoGit.Calls()[0].Arg)
}

func TestWorktreeHeadSHA_EmptyOnError(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnResolveRef = func(backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) { return "", backend.ErrUnborn }

	assert.Empty(t, worktreeHeadSHA(context.Background(), "/wt"))
}

// ---------------------------------------------------------------------------
// GetWorktreeDiff (UnfinishedWorkService)
// ---------------------------------------------------------------------------

// newMinimalUnfinishedWorkService constructs an UnfinishedWorkService with a
// real (but empty) Scanner backed by a temp-dir StateStore. No background
// scan goroutines are started, so it is safe to use in unit tests.
func newMinimalUnfinishedWorkService(t *testing.T) *UnfinishedWorkService {
	t.Helper()
	tmpDir := t.TempDir()
	stateStore, err := unfinished.NewStateStore(tmpDir + "/state.json")
	require.NoError(t, err)
	bus := events.NewEventBus(16)
	scanner := unfinished.NewScanner(bus, stateStore)
	return NewUnfinishedWorkService(scanner, stateStore, bus, nil, nil)
}

// TestGetWorktreeDiff_EmptyRepoPath verifies that missing repo_path or branch
// yields CodeNotFound (the scanner returns !ok, which the handler maps to
// CodeNotFound — the handler has no explicit empty-field validation).
func TestGetWorktreeDiff_EmptyRepoPath(t *testing.T) {
	t.Parallel()
	svc := newMinimalUnfinishedWorkService(t)

	_, err := svc.GetWorktreeDiff(context.Background(), connect.NewRequest(&sessionv1.GetWorktreeDiffRequest{
		RepoPath: "",
		Branch:   "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestGetWorktreeDiff_SessionNotFound verifies that a repo+branch pair that
// has not been scanned returns CodeNotFound.
func TestGetWorktreeDiff_SessionNotFound(t *testing.T) {
	t.Parallel()
	svc := newMinimalUnfinishedWorkService(t)

	_, err := svc.GetWorktreeDiff(context.Background(), connect.NewRequest(&sessionv1.GetWorktreeDiffRequest{
		RepoPath: "/nonexistent/repo",
		Branch:   "main",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
