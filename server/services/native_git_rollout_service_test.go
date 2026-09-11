package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedNativeGitRolloutService creates a NativeGitRolloutService backed
// by a fresh temporary directory, preventing config state from leaking
// between tests (same isolation pattern as newIsolatedTymuxRolloutService).
func newIsolatedNativeGitRolloutService(t *testing.T) *NativeGitRolloutService {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	return NewNativeGitRolloutService()
}

// TestNativeGitRolloutService_SetThenGet_ReflectsLiveState exercises all five
// RPCs at least once, per Task 4.2.2c: each Set* call's returned status
// reflects the mutation immediately, and a subsequent GetNativeGitRolloutStatus
// call confirms it was actually persisted rather than only returned in the
// mutating RPC's own response.
func TestNativeGitRolloutService_SetThenGet_ReflectsLiveState(t *testing.T) {
	s := newIsolatedNativeGitRolloutService(t)
	ctx := context.Background()
	forceTrue := true
	forceFalse := false

	// Fresh state: no overrides set.
	status, err := s.GetNativeGitRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetNativeGitRolloutStatusRequest{}))
	require.NoError(t, err)
	assert.Nil(t, status.Msg.WorktreeGlobalOverride)
	assert.Empty(t, status.Msg.WorktreeSessionOverrides)
	assert.Nil(t, status.Msg.MergeGlobalOverride)
	assert.Empty(t, status.Msg.MergeWorktreeOverrides)

	// SetNativeWorktreeGlobalOverride.
	resp, err := s.SetNativeWorktreeGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetNativeWorktreeGlobalOverrideRequest{
		ForceNative: &forceTrue,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.WorktreeGlobalOverride)
	assert.True(t, *resp.Msg.WorktreeGlobalOverride)

	status, err = s.GetNativeGitRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetNativeGitRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, status.Msg.WorktreeGlobalOverride)
	assert.True(t, *status.Msg.WorktreeGlobalOverride)

	// SetNativeWorktreeSessionOverride.
	resp, err = s.SetNativeWorktreeSessionOverride(ctx, connect.NewRequest(&sessionv1.SetNativeWorktreeSessionOverrideRequest{
		SessionName: "backlog-fix-142",
		ForceNative: &forceTrue,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.WorktreeSessionOverrides, 1)
	assert.Equal(t, "backlog-fix-142", resp.Msg.WorktreeSessionOverrides[0].SessionName)
	assert.True(t, resp.Msg.WorktreeSessionOverrides[0].ForceNative)

	status, err = s.GetNativeGitRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetNativeGitRolloutStatusRequest{}))
	require.NoError(t, err)
	require.Len(t, status.Msg.WorktreeSessionOverrides, 1)
	assert.Equal(t, "backlog-fix-142", status.Msg.WorktreeSessionOverrides[0].SessionName)

	// Clearing the session override (unset ForceNative).
	resp, err = s.SetNativeWorktreeSessionOverride(ctx, connect.NewRequest(&sessionv1.SetNativeWorktreeSessionOverrideRequest{
		SessionName: "backlog-fix-142",
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.WorktreeSessionOverrides)

	// SetNativeMergeGlobalOverride.
	resp, err = s.SetNativeMergeGlobalOverride(ctx, connect.NewRequest(&sessionv1.SetNativeMergeGlobalOverrideRequest{
		ForceNative: &forceFalse,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.MergeGlobalOverride)
	assert.False(t, *resp.Msg.MergeGlobalOverride)

	status, err = s.GetNativeGitRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetNativeGitRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, status.Msg.MergeGlobalOverride)
	assert.False(t, *status.Msg.MergeGlobalOverride)

	// SetNativeMergeWorktreeOverride.
	resp, err = s.SetNativeMergeWorktreeOverride(ctx, connect.NewRequest(&sessionv1.SetNativeMergeWorktreeOverrideRequest{
		WorktreePath: "/tmp/worktree-1",
		ForceNative:  &forceTrue,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.MergeWorktreeOverrides, 1)
	assert.Equal(t, "/tmp/worktree-1", resp.Msg.MergeWorktreeOverrides[0].WorktreePath)
	assert.True(t, resp.Msg.MergeWorktreeOverrides[0].ForceNative)

	status, err = s.GetNativeGitRolloutStatus(ctx, connect.NewRequest(&sessionv1.GetNativeGitRolloutStatusRequest{}))
	require.NoError(t, err)
	require.Len(t, status.Msg.MergeWorktreeOverrides, 1)
	assert.Equal(t, "/tmp/worktree-1", status.Msg.MergeWorktreeOverrides[0].WorktreePath)

	// Clearing the merge worktree override (unset ForceNative).
	resp, err = s.SetNativeMergeWorktreeOverride(ctx, connect.NewRequest(&sessionv1.SetNativeMergeWorktreeOverrideRequest{
		WorktreePath: "/tmp/worktree-1",
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.MergeWorktreeOverrides)
}

// TestNativeGitRolloutService_should_ReturnError_When_SessionOverrideKeyIsEmpty
// guards against silently keying an empty-string session override, per
// validation.md's P4 RPC requirement.
func TestNativeGitRolloutService_should_ReturnError_When_SessionOverrideKeyIsEmpty(t *testing.T) {
	s := newIsolatedNativeGitRolloutService(t)
	forceTrue := true

	_, err := s.SetNativeWorktreeSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetNativeWorktreeSessionOverrideRequest{
		SessionName: "",
		ForceNative: &forceTrue,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestNativeGitRolloutService_should_ReturnError_When_MergeOverrideKeyIsEmpty
// mirrors the above for the merge section's worktree-path key.
func TestNativeGitRolloutService_should_ReturnError_When_MergeOverrideKeyIsEmpty(t *testing.T) {
	s := newIsolatedNativeGitRolloutService(t)
	forceTrue := true

	_, err := s.SetNativeMergeWorktreeOverride(context.Background(), connect.NewRequest(&sessionv1.SetNativeMergeWorktreeOverrideRequest{
		WorktreePath: "",
		ForceNative:  &forceTrue,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
