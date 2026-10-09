package backend

import "context"

// Backend is the single seam through which stapler-squad runs git. One typed method per
// distinct git operation, derived from the Story 0.1.1 call-site audit (every subcommand
// family seen in session/git, session/vc, session/vcs, server/services, pkg/classifier).
// Callers never build argv or parse git text.
//
// Rules every method follows:
//   - the first parameter after ctx is the RepoLocation;
//   - no two adjacent parameters share a type (requests are structs), enforced by
//     TestNoAdjacentSameTypedParams;
//   - failures map onto the sentinel errors in errors.go where one applies, otherwise a
//     *CommandError.
type Backend interface {
	// --- refs and identity (rev-parse, symbolic-ref, for-each-ref, merge-base, rev-list, log) ---

	// CurrentBranch returns the checked-out branch. ErrDetachedHead on a detached HEAD,
	// ErrUnborn on a repository with no commits yet.
	CurrentBranch(ctx context.Context, loc RepoLocation) (BranchName, error)
	// HeadRef returns the ref HEAD points at (symbolic-ref), e.g. refs/heads/main, which is
	// valid on an unborn branch. ErrDetachedHead when HEAD is not symbolic.
	HeadRef(ctx context.Context, loc RepoLocation) (RefName, error)
	// ResolveRef resolves a revision to a commit. ErrUnborn when ref is HEAD of an unborn
	// branch, ErrRefNotFound otherwise.
	ResolveRef(ctx context.Context, loc RepoLocation, ref RefName) (CommitSHA, error)
	RefExists(ctx context.Context, loc RepoLocation, ref RefName) (bool, error)
	RepoRoot(ctx context.Context, loc RepoLocation) (RepoRoot, error)
	GitDir(ctx context.Context, loc RepoLocation) (GitDir, error)
	CommonDir(ctx context.Context, loc RepoLocation) (CommonDir, error)
	ListRefs(ctx context.Context, loc RepoLocation, req ListRefsRequest) ([]RefName, error)
	// MergeBase returns the best common ancestor. ErrRefNotFound when there is none.
	MergeBase(ctx context.Context, loc RepoLocation, req MergeBaseRequest) (CommitSHA, error)
	CountCommits(ctx context.Context, loc RepoLocation, rng RangeSpec) (int, error)
	Log(ctx context.Context, loc RepoLocation, req LogRequest) ([]LogEntry, error)

	// --- config and remotes (config, remote) ---

	// GetConfig returns the effective value. ErrConfigUnset when the key has none.
	GetConfig(ctx context.Context, loc RepoLocation, key ConfigKey) (string, error)
	SetConfig(ctx context.Context, loc RepoLocation, req SetConfigRequest) error
	SetRemoteURL(ctx context.Context, loc RepoLocation, req SetRemoteURLRequest) error

	// --- working tree state (status, ls-files, diff) ---

	IsDirty(ctx context.Context, loc RepoLocation, intent Intent) (bool, error)
	Status(ctx context.Context, loc RepoLocation, intent Intent) (StatusResult, error)
	ListUntracked(ctx context.Context, loc RepoLocation) ([]RepoPath, error)
	Diff(ctx context.Context, loc RepoLocation, spec DiffSpec) (string, error)
	DiffNumstat(ctx context.Context, loc RepoLocation, spec DiffSpec) ([]NumstatRow, error)

	// --- branches and checkout (branch, checkout, stash, reset, clean) ---

	ListBranches(ctx context.Context, loc RepoLocation, req ListBranchesRequest) ([]BranchInfo, error)
	CreateBranch(ctx context.Context, loc RepoLocation, req CreateBranchRequest) error
	RenameCurrentBranch(ctx context.Context, loc RepoLocation, to BranchName) error
	SwitchBranch(ctx context.Context, loc RepoLocation, req SwitchRequest) error
	// DiscardChanges drops every uncommitted change (reset HEAD, checkout ., clean -fd).
	// Irreversible; the caller owns the decision.
	DiscardChanges(ctx context.Context, loc RepoLocation) error
	StashPush(ctx context.Context, loc RepoLocation, req StashRequest) error
	StashPop(ctx context.Context, loc RepoLocation) error

	// --- index and commits (add, restore, reset, commit) ---

	Add(ctx context.Context, loc RepoLocation, req AddRequest) error
	Restore(ctx context.Context, loc RepoLocation, req RestoreRequest) error
	// ResetIndex unstages everything (reset HEAD) without touching the working tree.
	ResetIndex(ctx context.Context, loc RepoLocation) error
	Commit(ctx context.Context, loc RepoLocation, req CommitRequest) error

	// --- network (fetch, pull, push, clone) ---

	Fetch(ctx context.Context, loc RepoLocation, req FetchRequest) error
	Pull(ctx context.Context, loc RepoLocation, req PullRequest) error
	Push(ctx context.Context, loc RepoLocation, req PushRequest) error
	// Clone clones req.URL into loc (the destination directory, which must not exist yet).
	Clone(ctx context.Context, loc RepoLocation, req CloneRequest) error

	// --- linked worktrees (worktree) ---

	ListWorktrees(ctx context.Context, loc RepoLocation) ([]WorktreeInfo, error)
	// AddWorktree creates a worktree on a new branch.
	AddWorktree(ctx context.Context, loc RepoLocation, req AddWorktreeRequest) error
	AddWorktreeForExistingBranch(ctx context.Context, loc RepoLocation, path WorktreePath, br BranchName) error
	RemoveWorktree(ctx context.Context, loc RepoLocation, req RemoveWorktreeRequest) error
	PruneWorktrees(ctx context.Context, loc RepoLocation) error
}
