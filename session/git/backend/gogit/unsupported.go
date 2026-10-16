package gogit

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// notImplemented is the fallback-eligible answer for an operation this backend does not serve.
// Wrote is false because nothing was attempted, so the Router may replay it on the CLI even for
// a mutating operation.
func notImplemented(op backend.OperationName) error {
	return fallbackEligible(fmt.Errorf("%w: %s", backend.ErrNotImplemented, op))
}

func fallbackEligible(err error) error {
	return backend.ErrFallbackEligible{Wrote: false, Err: err}
}

// servedOperations are the operations this package answers in-process. Every other Backend
// method returns notImplemented; TestServedOperationsAreExactlyTheImplementedOnes keeps this
// list and the method bodies in step.
func servedOperations() []backend.OperationName {
	return []backend.OperationName{
		backend.OpCurrentBranch, backend.OpHeadRef, backend.OpResolveRef, backend.OpRefExists,
		backend.OpRepoRoot, backend.OpGitDir, backend.OpCommonDir, backend.OpListRefs,
		backend.OpMergeBase, backend.OpGetConfig,
	}
}

func (*Backend) CountCommits(context.Context, backend.RepoLocation, backend.RangeSpec) (int, error) {
	return 0, notImplemented(backend.OpCountCommits)
}

func (*Backend) Log(context.Context, backend.RepoLocation, backend.LogRequest) ([]backend.LogEntry, error) {
	return nil, notImplemented(backend.OpLog)
}

func (*Backend) SetConfig(context.Context, backend.RepoLocation, backend.SetConfigRequest) error {
	return notImplemented(backend.OpSetConfig)
}

func (*Backend) SetRemoteURL(context.Context, backend.RepoLocation, backend.SetRemoteURLRequest) error {
	return notImplemented(backend.OpSetRemoteURL)
}

func (*Backend) IsDirty(context.Context, backend.RepoLocation, backend.Intent) (bool, error) {
	return false, notImplemented(backend.OpIsDirty)
}

func (*Backend) Status(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
	return backend.StatusResult{}, notImplemented(backend.OpStatus)
}

func (*Backend) ListUntracked(context.Context, backend.RepoLocation) ([]backend.RepoPath, error) {
	return nil, notImplemented(backend.OpListUntracked)
}

func (*Backend) Diff(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) {
	return "", notImplemented(backend.OpDiff)
}

func (*Backend) DiffNumstat(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
	return nil, notImplemented(backend.OpDiffNumstat)
}

// ListBranches is not served: BranchInfo.ShortSHA is git's auto-scaled abbreviation, which
// depends on the repository's object count and on uniqueness among all objects.
func (*Backend) ListBranches(context.Context, backend.RepoLocation, backend.ListBranchesRequest) ([]backend.BranchInfo, error) {
	return nil, notImplemented(backend.OpListBranches)
}

func (*Backend) CreateBranch(context.Context, backend.RepoLocation, backend.CreateBranchRequest) error {
	return notImplemented(backend.OpCreateBranch)
}

func (*Backend) RenameCurrentBranch(context.Context, backend.RepoLocation, backend.BranchName) error {
	return notImplemented(backend.OpRenameCurrentBranch)
}

func (*Backend) DeleteBranch(context.Context, backend.RepoLocation, backend.DeleteBranchRequest) error {
	return notImplemented(backend.OpDeleteBranch)
}

func (*Backend) SetUpstream(context.Context, backend.RepoLocation, backend.SetUpstreamRequest) error {
	return notImplemented(backend.OpSetUpstream)
}

func (*Backend) CheckoutCommit(context.Context, backend.RepoLocation, backend.CommitSHA) error {
	return notImplemented(backend.OpCheckoutCommit)
}

func (*Backend) SwitchBranch(context.Context, backend.RepoLocation, backend.SwitchRequest) error {
	return notImplemented(backend.OpSwitchBranch)
}

func (*Backend) DiscardChanges(context.Context, backend.RepoLocation) error {
	return notImplemented(backend.OpDiscardChanges)
}

func (*Backend) StashPush(context.Context, backend.RepoLocation, backend.StashRequest) error {
	return notImplemented(backend.OpStashPush)
}

func (*Backend) StashPop(context.Context, backend.RepoLocation) error {
	return notImplemented(backend.OpStashPop)
}

func (*Backend) Add(context.Context, backend.RepoLocation, backend.AddRequest) error {
	return notImplemented(backend.OpAdd)
}

func (*Backend) Restore(context.Context, backend.RepoLocation, backend.RestoreRequest) error {
	return notImplemented(backend.OpRestore)
}

func (*Backend) Reset(context.Context, backend.RepoLocation, backend.ResetRequest) error {
	return notImplemented(backend.OpReset)
}

func (*Backend) RemoveFiles(context.Context, backend.RepoLocation, backend.RemoveFilesRequest) error {
	return notImplemented(backend.OpRemoveFiles)
}

func (*Backend) MoveFile(context.Context, backend.RepoLocation, backend.MoveFileRequest) error {
	return notImplemented(backend.OpMoveFile)
}

func (*Backend) Commit(context.Context, backend.RepoLocation, backend.CommitRequest) error {
	return notImplemented(backend.OpCommit)
}

func (*Backend) Fetch(context.Context, backend.RepoLocation, backend.FetchRequest) error {
	return notImplemented(backend.OpFetch)
}

func (*Backend) Pull(context.Context, backend.RepoLocation, backend.PullRequest) error {
	return notImplemented(backend.OpPull)
}

func (*Backend) Push(context.Context, backend.RepoLocation, backend.PushRequest) error {
	return notImplemented(backend.OpPush)
}

func (*Backend) ListRemote(context.Context, backend.RepoLocation, backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
	return nil, notImplemented(backend.OpListRemote)
}

func (*Backend) Clone(context.Context, backend.RepoLocation, backend.CloneRequest) error {
	return notImplemented(backend.OpClone)
}

func (*Backend) ListWorktrees(context.Context, backend.RepoLocation) ([]backend.WorktreeInfo, error) {
	return nil, notImplemented(backend.OpListWorktrees)
}

func (*Backend) AddWorktree(context.Context, backend.RepoLocation, backend.AddWorktreeRequest) error {
	return notImplemented(backend.OpAddWorktree)
}

func (*Backend) AddWorktreeForExistingBranch(context.Context, backend.RepoLocation, backend.WorktreePath, backend.BranchName) error {
	return notImplemented(backend.OpAddWorktreeForExistingBranch)
}

func (*Backend) RemoveWorktree(context.Context, backend.RepoLocation, backend.RemoveWorktreeRequest) error {
	return notImplemented(backend.OpRemoveWorktree)
}

func (*Backend) PruneWorktrees(context.Context, backend.RepoLocation) error {
	return notImplemented(backend.OpPruneWorktrees)
}
