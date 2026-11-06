package backend

import "context"

// The Router's Backend methods are mechanical: each names its operation and forwards to
// dispatch. TestRouterMethodsReportTheirOwnOperation guards the operation names.

func (r *Router) CurrentBranch(ctx context.Context, loc RepoLocation) (BranchName, error) {
	return dispatch(r, ctx, loc, OpCurrentBranch, readSpec[BranchName]{}, func(ctx context.Context, b Backend) (BranchName, error) {
		return b.CurrentBranch(ctx, loc)
	})
}

func (r *Router) HeadRef(ctx context.Context, loc RepoLocation) (RefName, error) {
	return dispatch(r, ctx, loc, OpHeadRef, readSpec[RefName]{}, func(ctx context.Context, b Backend) (RefName, error) {
		return b.HeadRef(ctx, loc)
	})
}

func (r *Router) ResolveRef(ctx context.Context, loc RepoLocation, ref RefName) (CommitSHA, error) {
	return dispatch(r, ctx, loc, OpResolveRef, readSpec[CommitSHA]{}, func(ctx context.Context, b Backend) (CommitSHA, error) {
		return b.ResolveRef(ctx, loc, ref)
	})
}

func (r *Router) RefExists(ctx context.Context, loc RepoLocation, ref RefName) (bool, error) {
	return dispatch(r, ctx, loc, OpRefExists, readSpec[bool]{}, func(ctx context.Context, b Backend) (bool, error) {
		return b.RefExists(ctx, loc, ref)
	})
}

func (r *Router) RepoRoot(ctx context.Context, loc RepoLocation) (RepoRoot, error) {
	return dispatch(r, ctx, loc, OpRepoRoot, readSpec[RepoRoot]{}, func(ctx context.Context, b Backend) (RepoRoot, error) {
		return b.RepoRoot(ctx, loc)
	})
}

func (r *Router) GitDir(ctx context.Context, loc RepoLocation) (GitDir, error) {
	return dispatch(r, ctx, loc, OpGitDir, readSpec[GitDir]{}, func(ctx context.Context, b Backend) (GitDir, error) {
		return b.GitDir(ctx, loc)
	})
}

func (r *Router) CommonDir(ctx context.Context, loc RepoLocation) (CommonDir, error) {
	return dispatch(r, ctx, loc, OpCommonDir, readSpec[CommonDir]{}, func(ctx context.Context, b Backend) (CommonDir, error) {
		return b.CommonDir(ctx, loc)
	})
}

func (r *Router) ListRefs(ctx context.Context, loc RepoLocation, req ListRefsRequest) ([]RefName, error) {
	return dispatch(r, ctx, loc, OpListRefs, readSpec[[]RefName]{}, func(ctx context.Context, b Backend) ([]RefName, error) {
		return b.ListRefs(ctx, loc, req)
	})
}

func (r *Router) MergeBase(ctx context.Context, loc RepoLocation, req MergeBaseRequest) (CommitSHA, error) {
	return dispatch(r, ctx, loc, OpMergeBase, readSpec[CommitSHA]{}, func(ctx context.Context, b Backend) (CommitSHA, error) {
		return b.MergeBase(ctx, loc, req)
	})
}

func (r *Router) CountCommits(ctx context.Context, loc RepoLocation, rng RangeSpec) (int, error) {
	return dispatch(r, ctx, loc, OpCountCommits, readSpec[int]{}, func(ctx context.Context, b Backend) (int, error) {
		return b.CountCommits(ctx, loc, rng)
	})
}

func (r *Router) Log(ctx context.Context, loc RepoLocation, req LogRequest) ([]LogEntry, error) {
	return dispatch(r, ctx, loc, OpLog, readSpec[[]LogEntry]{}, func(ctx context.Context, b Backend) ([]LogEntry, error) {
		return b.Log(ctx, loc, req)
	})
}

func (r *Router) GetConfig(ctx context.Context, loc RepoLocation, key ConfigKey) (string, error) {
	return dispatch(r, ctx, loc, OpGetConfig, readSpec[string]{}, func(ctx context.Context, b Backend) (string, error) {
		return b.GetConfig(ctx, loc, key)
	})
}

func (r *Router) SetConfig(ctx context.Context, loc RepoLocation, req SetConfigRequest) error {
	return dispatchErr(r, ctx, loc, OpSetConfig, func(ctx context.Context, b Backend) error {
		return b.SetConfig(ctx, loc, req)
	})
}

func (r *Router) SetRemoteURL(ctx context.Context, loc RepoLocation, req SetRemoteURLRequest) error {
	return dispatchErr(r, ctx, loc, OpSetRemoteURL, func(ctx context.Context, b Backend) error {
		return b.SetRemoteURL(ctx, loc, req)
	})
}

func (r *Router) IsDirty(ctx context.Context, loc RepoLocation, intent Intent) (bool, error) {
	return dispatch(r, ctx, loc, OpIsDirty, readSpec[bool]{clean: func(dirty bool) bool { return !dirty }, destructive: intent == IntentDestructive}, func(ctx context.Context, b Backend) (bool, error) {
		return b.IsDirty(ctx, loc, intent)
	})
}

func (r *Router) Status(ctx context.Context, loc RepoLocation, intent Intent) (StatusResult, error) {
	return dispatch(r, ctx, loc, OpStatus, readSpec[StatusResult]{clean: func(s StatusResult) bool { return !s.Dirty() }, destructive: intent == IntentDestructive}, func(ctx context.Context, b Backend) (StatusResult, error) {
		return b.Status(ctx, loc, intent)
	})
}

func (r *Router) ListUntracked(ctx context.Context, loc RepoLocation) ([]RepoPath, error) {
	return dispatch(r, ctx, loc, OpListUntracked, readSpec[[]RepoPath]{clean: func(p []RepoPath) bool { return len(p) == 0 }}, func(ctx context.Context, b Backend) ([]RepoPath, error) {
		return b.ListUntracked(ctx, loc)
	})
}

func (r *Router) Diff(ctx context.Context, loc RepoLocation, spec DiffSpec) (string, error) {
	return dispatch(r, ctx, loc, OpDiff, readSpec[string]{}, func(ctx context.Context, b Backend) (string, error) {
		return b.Diff(ctx, loc, spec)
	})
}

func (r *Router) DiffNumstat(ctx context.Context, loc RepoLocation, spec DiffSpec) ([]NumstatRow, error) {
	return dispatch(r, ctx, loc, OpDiffNumstat, readSpec[[]NumstatRow]{clean: func(rows []NumstatRow) bool { return len(rows) == 0 }, destructive: spec.Intent == IntentDestructive}, func(ctx context.Context, b Backend) ([]NumstatRow, error) {
		return b.DiffNumstat(ctx, loc, spec)
	})
}

func (r *Router) ListBranches(ctx context.Context, loc RepoLocation, req ListBranchesRequest) ([]BranchInfo, error) {
	return dispatch(r, ctx, loc, OpListBranches, readSpec[[]BranchInfo]{}, func(ctx context.Context, b Backend) ([]BranchInfo, error) {
		return b.ListBranches(ctx, loc, req)
	})
}

func (r *Router) CreateBranch(ctx context.Context, loc RepoLocation, req CreateBranchRequest) error {
	return dispatchErr(r, ctx, loc, OpCreateBranch, func(ctx context.Context, b Backend) error {
		return b.CreateBranch(ctx, loc, req)
	})
}

func (r *Router) RenameCurrentBranch(ctx context.Context, loc RepoLocation, to BranchName) error {
	return dispatchErr(r, ctx, loc, OpRenameCurrentBranch, func(ctx context.Context, b Backend) error {
		return b.RenameCurrentBranch(ctx, loc, to)
	})
}

func (r *Router) DeleteBranch(ctx context.Context, loc RepoLocation, req DeleteBranchRequest) error {
	return dispatchErr(r, ctx, loc, OpDeleteBranch, func(ctx context.Context, b Backend) error {
		return b.DeleteBranch(ctx, loc, req)
	})
}

func (r *Router) SetUpstream(ctx context.Context, loc RepoLocation, req SetUpstreamRequest) error {
	return dispatchErr(r, ctx, loc, OpSetUpstream, func(ctx context.Context, b Backend) error {
		return b.SetUpstream(ctx, loc, req)
	})
}

func (r *Router) CheckoutCommit(ctx context.Context, loc RepoLocation, sha CommitSHA) error {
	return dispatchErr(r, ctx, loc, OpCheckoutCommit, func(ctx context.Context, b Backend) error {
		return b.CheckoutCommit(ctx, loc, sha)
	})
}

func (r *Router) SwitchBranch(ctx context.Context, loc RepoLocation, req SwitchRequest) error {
	return dispatchErr(r, ctx, loc, OpSwitchBranch, func(ctx context.Context, b Backend) error {
		return b.SwitchBranch(ctx, loc, req)
	})
}

func (r *Router) DiscardChanges(ctx context.Context, loc RepoLocation) error {
	return dispatchErr(r, ctx, loc, OpDiscardChanges, func(ctx context.Context, b Backend) error {
		return b.DiscardChanges(ctx, loc)
	})
}

func (r *Router) StashPush(ctx context.Context, loc RepoLocation, req StashRequest) error {
	return dispatchErr(r, ctx, loc, OpStashPush, func(ctx context.Context, b Backend) error {
		return b.StashPush(ctx, loc, req)
	})
}

func (r *Router) StashPop(ctx context.Context, loc RepoLocation) error {
	return dispatchErr(r, ctx, loc, OpStashPop, func(ctx context.Context, b Backend) error {
		return b.StashPop(ctx, loc)
	})
}

func (r *Router) Add(ctx context.Context, loc RepoLocation, req AddRequest) error {
	return dispatchErr(r, ctx, loc, OpAdd, func(ctx context.Context, b Backend) error {
		return b.Add(ctx, loc, req)
	})
}

func (r *Router) Restore(ctx context.Context, loc RepoLocation, req RestoreRequest) error {
	return dispatchErr(r, ctx, loc, OpRestore, func(ctx context.Context, b Backend) error {
		return b.Restore(ctx, loc, req)
	})
}

func (r *Router) Reset(ctx context.Context, loc RepoLocation, req ResetRequest) error {
	return dispatchErr(r, ctx, loc, OpReset, func(ctx context.Context, b Backend) error {
		return b.Reset(ctx, loc, req)
	})
}

func (r *Router) RemoveFiles(ctx context.Context, loc RepoLocation, req RemoveFilesRequest) error {
	return dispatchErr(r, ctx, loc, OpRemoveFiles, func(ctx context.Context, b Backend) error {
		return b.RemoveFiles(ctx, loc, req)
	})
}

func (r *Router) MoveFile(ctx context.Context, loc RepoLocation, req MoveFileRequest) error {
	return dispatchErr(r, ctx, loc, OpMoveFile, func(ctx context.Context, b Backend) error {
		return b.MoveFile(ctx, loc, req)
	})
}

func (r *Router) Commit(ctx context.Context, loc RepoLocation, req CommitRequest) error {
	return dispatchErr(r, ctx, loc, OpCommit, func(ctx context.Context, b Backend) error {
		return b.Commit(ctx, loc, req)
	})
}

func (r *Router) Fetch(ctx context.Context, loc RepoLocation, req FetchRequest) error {
	return dispatchErr(r, ctx, loc, OpFetch, func(ctx context.Context, b Backend) error {
		return b.Fetch(ctx, loc, req)
	})
}

func (r *Router) Pull(ctx context.Context, loc RepoLocation, req PullRequest) error {
	return dispatchErr(r, ctx, loc, OpPull, func(ctx context.Context, b Backend) error {
		return b.Pull(ctx, loc, req)
	})
}

func (r *Router) Push(ctx context.Context, loc RepoLocation, req PushRequest) error {
	return dispatchErr(r, ctx, loc, OpPush, func(ctx context.Context, b Backend) error {
		return b.Push(ctx, loc, req)
	})
}

func (r *Router) ListRemote(ctx context.Context, loc RepoLocation, req ListRemoteRequest) ([]RemoteRef, error) {
	return dispatch(r, ctx, loc, OpListRemote, readSpec[[]RemoteRef]{}, func(ctx context.Context, b Backend) ([]RemoteRef, error) {
		return b.ListRemote(ctx, loc, req)
	})
}

func (r *Router) Clone(ctx context.Context, loc RepoLocation, req CloneRequest) error {
	return dispatchErr(r, ctx, loc, OpClone, func(ctx context.Context, b Backend) error {
		return b.Clone(ctx, loc, req)
	})
}

func (r *Router) ListWorktrees(ctx context.Context, loc RepoLocation) ([]WorktreeInfo, error) {
	return dispatch(r, ctx, loc, OpListWorktrees, readSpec[[]WorktreeInfo]{}, func(ctx context.Context, b Backend) ([]WorktreeInfo, error) {
		return b.ListWorktrees(ctx, loc)
	})
}

func (r *Router) AddWorktree(ctx context.Context, loc RepoLocation, req AddWorktreeRequest) error {
	return dispatchErr(r, ctx, loc, OpAddWorktree, func(ctx context.Context, b Backend) error {
		return b.AddWorktree(ctx, loc, req)
	})
}

func (r *Router) AddWorktreeForExistingBranch(ctx context.Context, loc RepoLocation, path WorktreePath, br BranchName) error {
	return dispatchErr(r, ctx, loc, OpAddWorktreeForExistingBranch, func(ctx context.Context, b Backend) error {
		return b.AddWorktreeForExistingBranch(ctx, loc, path, br)
	})
}

func (r *Router) RemoveWorktree(ctx context.Context, loc RepoLocation, req RemoveWorktreeRequest) error {
	return dispatchErr(r, ctx, loc, OpRemoveWorktree, func(ctx context.Context, b Backend) error {
		return b.RemoveWorktree(ctx, loc, req)
	})
}

func (r *Router) PruneWorktrees(ctx context.Context, loc RepoLocation) error {
	return dispatchErr(r, ctx, loc, OpPruneWorktrees, func(ctx context.Context, b Backend) error {
		return b.PruneWorktrees(ctx, loc)
	})
}
