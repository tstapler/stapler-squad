package services

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

var (
	errEmptySessionName  = errors.New("session_name must not be empty")
	errEmptyWorktreePath = errors.New("worktree_path must not be empty")
)

// NativeGitRolloutService handles the GetNativeGitRolloutStatus/
// SetNativeWorktreeGlobalOverride/SetNativeWorktreeSessionOverride/
// SetNativeMergeGlobalOverride/SetNativeMergeWorktreeOverride RPCs — the
// operator-facing controls for the go-git-worktree-and-merge rollout,
// mirroring TymuxRolloutService's shape exactly: a config-backed handler
// with no second implementation, so a concrete type per the
// `interface-pollution-checklist` skill.
//
// Unlike TymuxRolloutService, this covers two independent feature flags
// (native worktree management and native merge) in one service, since
// neither has a legacy env var to deprecate or a rollback-rehearsal gate
// (ADR-002).
type NativeGitRolloutService struct{}

// NewNativeGitRolloutService creates a NativeGitRolloutService.
func NewNativeGitRolloutService() *NativeGitRolloutService {
	return &NativeGitRolloutService{}
}

// status builds the current NativeGitRolloutStatus from live config — shared
// by all five RPCs since each returns the post-mutation status.
func (s *NativeGitRolloutService) status() *sessionv1.NativeGitRolloutStatus {
	cfg := config.LoadConfig()

	worktreeOverrides := make([]*sessionv1.NativeWorktreeSessionOverrideEntry, 0, len(cfg.NativeWorktreeSessionOverrides))
	for name, forceNative := range cfg.NativeWorktreeSessionOverrides {
		worktreeOverrides = append(worktreeOverrides, &sessionv1.NativeWorktreeSessionOverrideEntry{
			SessionName: name,
			ForceNative: forceNative,
		})
	}

	mergeOverrides := make([]*sessionv1.NativeMergeWorktreeOverrideEntry, 0, len(cfg.NativeMergeWorktreeOverrides))
	for path, forceNative := range cfg.NativeMergeWorktreeOverrides {
		mergeOverrides = append(mergeOverrides, &sessionv1.NativeMergeWorktreeOverrideEntry{
			WorktreePath: path,
			ForceNative:  forceNative,
		})
	}

	var worktreeGlobalOverride *bool
	if v, ok := cfg.GetNativeWorktreeGlobalOverride(); ok {
		worktreeGlobalOverride = &v
	}

	var mergeGlobalOverride *bool
	if v, ok := cfg.GetNativeMergeGlobalOverride(); ok {
		mergeGlobalOverride = &v
	}

	return &sessionv1.NativeGitRolloutStatus{
		WorktreeGlobalOverride:   worktreeGlobalOverride,
		WorktreeSessionOverrides: worktreeOverrides,
		MergeGlobalOverride:      mergeGlobalOverride,
		MergeWorktreeOverrides:   mergeOverrides,
	}
}

// GetNativeGitRolloutStatus returns the current rollout status for both
// flags.
// +api: native-git-rollout:get
func (s *NativeGitRolloutService) GetNativeGitRolloutStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetNativeGitRolloutStatusRequest],
) (*connect.Response[sessionv1.NativeGitRolloutStatus], error) {
	return connect.NewResponse(s.status()), nil
}

// SetNativeWorktreeGlobalOverride sets or clears the "native_git_worktree"
// feature flag. Takes effect immediately for worktrees set up after this
// call — no process restart required.
// +api: native-git-rollout:set-worktree-global-override
func (s *NativeGitRolloutService) SetNativeWorktreeGlobalOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetNativeWorktreeGlobalOverrideRequest],
) (*connect.Response[sessionv1.NativeGitRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.SetNativeWorktreeGlobalOverride(req.Msg.ForceNative); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetNativeWorktreeSessionOverride sets or clears a per-session override for
// the native-worktree flag. req.Msg.ForceNative is presence-tracked (proto3
// optional): unset clears any existing override for the session (falls back
// to the global default), while a present true/false pins the session onto
// native/legacy respectively — see config.SetNativeWorktreeSessionOverride's
// doc comment for the tri-state semantics this passes straight through.
// +api: native-git-rollout:set-worktree-session-override
func (s *NativeGitRolloutService) SetNativeWorktreeSessionOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetNativeWorktreeSessionOverrideRequest],
) (*connect.Response[sessionv1.NativeGitRolloutStatus], error) {
	if req.Msg.GetSessionName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errEmptySessionName)
	}
	cfg := config.LoadConfig()
	if err := cfg.SetNativeWorktreeSessionOverride(req.Msg.GetSessionName(), req.Msg.ForceNative); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetNativeMergeGlobalOverride sets or clears the "native_git_merge" feature
// flag. Takes effect immediately for merges performed after this call — no
// process restart required.
// +api: native-git-rollout:set-merge-global-override
func (s *NativeGitRolloutService) SetNativeMergeGlobalOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetNativeMergeGlobalOverrideRequest],
) (*connect.Response[sessionv1.NativeGitRolloutStatus], error) {
	cfg := config.LoadConfig()
	if err := cfg.SetNativeMergeGlobalOverride(req.Msg.ForceNative); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}

// SetNativeMergeWorktreeOverride sets or clears a per-worktree-path override
// for the native-merge flag. req.Msg.ForceNative is presence-tracked (proto3
// optional): unset clears any existing override for the path (falls back to
// the global default), while a present true/false pins that worktree's
// merges onto native/legacy respectively.
// +api: native-git-rollout:set-merge-worktree-override
func (s *NativeGitRolloutService) SetNativeMergeWorktreeOverride(
	ctx context.Context,
	req *connect.Request[sessionv1.SetNativeMergeWorktreeOverrideRequest],
) (*connect.Response[sessionv1.NativeGitRolloutStatus], error) {
	if req.Msg.GetWorktreePath() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errEmptyWorktreePath)
	}
	cfg := config.LoadConfig()
	if err := cfg.SetNativeMergeWorktreeOverride(req.Msg.GetWorktreePath(), req.Msg.ForceNative); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(s.status()), nil
}
