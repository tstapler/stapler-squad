package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// attachDirtyWorktree gives inst a real worktree holding an untracked file and returns that
// file's path.
func attachDirtyWorktree(t *testing.T, inst *session.Instance, name string) string {
	t.Helper()
	wt, _, err := git.NewGitWorktree(newDraftPRTestRepo(t), name)
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	inst.SetGitWorktree(wt)

	precious := filepath.Join(wt.GetWorktreePath(), "precious.txt")
	require.NoError(t, os.WriteFile(precious, []byte("uncommitted work"), 0o644))
	return precious
}

func TestCleanupPartialCreation_should_KeepDirtyWorktree_When_NotForced(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "cleanup-dirty-keep"}
	precious := attachDirtyWorktree(t, inst, "cleanup-dirty-keep")

	err := cleanupPartialCreation(inst, false)

	require.ErrorIs(t, err, session.ErrWorktreeDirty)
	assert.FileExists(t, precious)
}

func TestCleanupPartialCreation_should_RemoveDirtyWorktree_When_Forced(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "cleanup-dirty-force"}
	precious := attachDirtyWorktree(t, inst, "cleanup-dirty-force")

	require.NoError(t, cleanupPartialCreation(inst, true))

	assert.NoFileExists(t, precious)
}

func TestDeleteSession_should_RefuseDirtyWorktree_When_NotForced(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	t.Cleanup(func() { svc.Shutdown() })
	inst := newCancelTestInstance(t, svc, storage, "delete-dirty-refused", false)
	precious := attachDirtyWorktree(t, inst, "delete-dirty-refused")

	_, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "delete-dirty-refused",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.NotNil(t, svc.reviewQueuePoller.FindInstance("delete-dirty-refused"), "a refused delete must leave the session registered")
	assert.FileExists(t, precious)
}

func TestDeleteSession_should_DiscardDirtyWorktree_When_Forced(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	t.Cleanup(func() { svc.Shutdown() })
	inst := newCancelTestInstance(t, svc, storage, "delete-dirty-forced", false)
	precious := attachDirtyWorktree(t, inst, "delete-dirty-forced")

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id:    "delete-dirty-forced",
		Force: true,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)

	svc.deleteCleanupWG.Wait() // cleanup runs in the background after the RPC returns
	assert.NoFileExists(t, precious)
}
