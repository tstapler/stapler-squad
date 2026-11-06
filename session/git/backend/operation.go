package backend

// OperationName identifies one Backend method. It is a closed set: every Backend method has
// exactly one constant here, enforced by TestOperationsMatchBackendMethods.
type OperationName string

// One constant per Backend method, grouped by the git subcommand family it wraps.
const (
	OpCurrentBranch                OperationName = "CurrentBranch"                // rev-parse --abbrev-ref
	OpHeadRef                      OperationName = "HeadRef"                      // symbolic-ref
	OpResolveRef                   OperationName = "ResolveRef"                   // rev-parse --verify
	OpRefExists                    OperationName = "RefExists"                    // rev-parse --verify --quiet
	OpRepoRoot                     OperationName = "RepoRoot"                     // rev-parse --show-toplevel
	OpGitDir                       OperationName = "GitDir"                       // rev-parse --git-dir
	OpCommonDir                    OperationName = "CommonDir"                    // rev-parse --git-common-dir
	OpListRefs                     OperationName = "ListRefs"                     // for-each-ref
	OpMergeBase                    OperationName = "MergeBase"                    // merge-base
	OpCountCommits                 OperationName = "CountCommits"                 // rev-list --count
	OpLog                          OperationName = "Log"                          // log
	OpGetConfig                    OperationName = "GetConfig"                    // config --get
	OpSetConfig                    OperationName = "SetConfig"                    // config
	OpSetRemoteURL                 OperationName = "SetRemoteURL"                 // remote set-url
	OpIsDirty                      OperationName = "IsDirty"                      // status --porcelain
	OpStatus                       OperationName = "Status"                       // status --porcelain=v2
	OpListUntracked                OperationName = "ListUntracked"                // ls-files --others
	OpDiff                         OperationName = "Diff"                         // diff
	OpDiffNumstat                  OperationName = "DiffNumstat"                  // diff --numstat
	OpListBranches                 OperationName = "ListBranches"                 // branch --list / --contains
	OpCreateBranch                 OperationName = "CreateBranch"                 // branch <name> <base>
	OpRenameCurrentBranch          OperationName = "RenameCurrentBranch"          // branch -m
	OpSwitchBranch                 OperationName = "SwitchBranch"                 // checkout [-b]
	OpDiscardChanges               OperationName = "DiscardChanges"               // reset HEAD, checkout ., clean -fd
	OpStashPush                    OperationName = "StashPush"                    // stash push
	OpStashPop                     OperationName = "StashPop"                     // stash pop
	OpAdd                          OperationName = "Add"                          // add
	OpRestore                      OperationName = "Restore"                      // restore
	OpReset                        OperationName = "Reset"                        // reset --mixed|--soft|--hard
	OpDeleteBranch                 OperationName = "DeleteBranch"                 // branch -d / -D
	OpSetUpstream                  OperationName = "SetUpstream"                  // branch --set-upstream-to
	OpCheckoutCommit               OperationName = "CheckoutCommit"               // switch --detach
	OpListRemote                   OperationName = "ListRemote"                   // ls-remote
	OpRemoveFiles                  OperationName = "RemoveFiles"                  // rm
	OpMoveFile                     OperationName = "MoveFile"                     // mv
	OpCommit                       OperationName = "Commit"                       // commit
	OpFetch                        OperationName = "Fetch"                        // fetch
	OpPull                         OperationName = "Pull"                         // pull
	OpPush                         OperationName = "Push"                         // push
	OpClone                        OperationName = "Clone"                        // clone
	OpListWorktrees                OperationName = "ListWorktrees"                // worktree list --porcelain
	OpAddWorktree                  OperationName = "AddWorktree"                  // worktree add -b
	OpAddWorktreeForExistingBranch OperationName = "AddWorktreeForExistingBranch" // worktree add
	OpRemoveWorktree               OperationName = "RemoveWorktree"               // worktree remove
	OpPruneWorktrees               OperationName = "PruneWorktrees"               // worktree prune
)

var allOperations = []OperationName{
	OpCurrentBranch, OpHeadRef, OpResolveRef, OpRefExists, OpRepoRoot, OpGitDir, OpCommonDir,
	OpListRefs, OpMergeBase, OpCountCommits, OpLog, OpGetConfig, OpSetConfig, OpSetRemoteURL,
	OpIsDirty, OpStatus, OpListUntracked, OpDiff, OpDiffNumstat, OpListBranches, OpCreateBranch,
	OpRenameCurrentBranch, OpSwitchBranch, OpDiscardChanges, OpStashPush, OpStashPop, OpAdd,
	OpRestore, OpReset, OpDeleteBranch, OpSetUpstream, OpCheckoutCommit, OpListRemote, OpRemoveFiles, OpMoveFile, OpCommit, OpFetch, OpPull, OpPush, OpClone, OpListWorktrees,
	OpAddWorktree, OpAddWorktreeForExistingBranch, OpRemoveWorktree, OpPruneWorktrees,
}

var mutating = map[OperationName]bool{
	OpSetConfig: true, OpSetRemoteURL: true, OpCreateBranch: true, OpRenameCurrentBranch: true,
	OpSwitchBranch: true, OpDiscardChanges: true, OpStashPush: true, OpStashPop: true,
	OpAdd: true, OpRestore: true, OpReset: true, OpDeleteBranch: true, OpSetUpstream: true, OpCheckoutCommit: true, OpRemoveFiles: true, OpMoveFile: true, OpCommit: true, OpFetch: true,
	OpPull: true, OpPush: true, OpClone: true, OpAddWorktree: true,
	OpAddWorktreeForExistingBranch: true, OpRemoveWorktree: true, OpPruneWorktrees: true,
}

// AllOperations returns every operation, in declaration order.
func AllOperations() []OperationName {
	return append([]OperationName(nil), allOperations...)
}

// Mutating reports whether the operation changes repository, index, config or network-visible
// state. The router never shadows a mutating operation and runs capability preflight before it.
func (o OperationName) Mutating() bool { return mutating[o] }

// Known reports whether o is one of the closed set.
func (o OperationName) Known() bool {
	for _, k := range allOperations {
		if k == o {
			return true
		}
	}
	return false
}
