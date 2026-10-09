package backend

// Distinct string newtypes so a branch cannot be passed where a SHA is expected and a
// repository path cannot be confused with a git directory.
type (
	RepoRoot     string // worktree top (rev-parse --show-toplevel)
	GitDir       string // per-worktree admin dir; differs from CommonDir in a linked worktree
	CommonDir    string // shared .git
	BranchName   string // short branch name, e.g. "main"
	RefName      string // any ref or revision expression git accepts, e.g. "origin/main", "HEAD"
	CommitSHA    string
	RemoteName   string // configured remote, e.g. "origin"
	RemoteURL    string // clone/fetch URL; may carry credentials, always redact before logging
	RemoteHost   string // SSH host a Remote location runs on (identity for logs only)
	RemotePath   string // repository path on the remote host
	WorktreePath string // filesystem path of a linked worktree
	RepoPath     string // path relative to the repository root
	ConfigKey    string // dotted git config key, e.g. "remote.origin.url"
	RefPattern   string // for-each-ref pattern, e.g. "refs/heads/"
)

// Intent states what a read's answer will be used for. The zero value is Destructive so a
// caller that forgets to set it gets the conservative behaviour (the CLI confirms a "clean").
type Intent uint8

const (
	// IntentDestructive: the answer gates an irreversible step (worktree removal, cleanup,
	// pause, a review-gate pass). A backend that is not the CLI must have a "clean" confirmed.
	IntentDestructive Intent = iota
	// IntentDisplay: the answer only feeds UI or prompts; a stale answer is harmless.
	IntentDisplay
)

func (i Intent) String() string {
	if i == IntentDisplay {
		return "display"
	}
	return "destructive"
}

// FileStatus is one entry of a StatusResult.
type FileStatus struct {
	Path         RepoPath
	OrigPath     RepoPath // set for renames and copies
	Index        StatusCode
	Worktree     StatusCode
	Untracked    bool
	Ignored      bool
	Unmerged     bool
	SubmoduleDir bool
}

// StatusCode is the single-letter porcelain status of one side of a file ('M', 'A', 'D', 'R',
// 'C', 'T', 'U' or '.' for unchanged).
type StatusCode byte

// StatusResult is the parsed `git status --porcelain=v2 --branch`.
type StatusResult struct {
	HeadOID  CommitSHA // zero when unborn
	Branch   BranchName
	Detached bool
	Unborn   bool
	Upstream RefName
	Ahead    int
	Behind   int
	Files    []FileStatus
}

// Dirty reports whether any tracked change, unmerged path or untracked file exists.
func (s StatusResult) Dirty() bool {
	for _, f := range s.Files {
		if !f.Ignored {
			return true
		}
	}
	return false
}

// DiffSpec selects what a diff compares. Zero Base and Head with Staged unset means working
// tree against the index; Staged compares index against HEAD; Base alone means Base against
// the working tree; Base and Head together mean Base..Head. FromMergeBase switches to
// Base...Head (Head, default HEAD, against its merge-base with Base).
type DiffSpec struct {
	Base          RefName
	Head          RefName
	Staged        bool
	FromMergeBase bool
	Paths         []RepoPath
	Intent        Intent
}

// NumstatRow is one line of `git diff --numstat`.
type NumstatRow struct {
	Path     RepoPath
	OrigPath RepoPath // set for renames
	Added    int
	Deleted  int
	Binary   bool // git reports "-" for both counts
}

// RangeSpec is the revision range Include minus Exclude (`git rev-list Exclude..Include`).
// Named fields prevent the swap a positional pair would allow.
type RangeSpec struct {
	Exclude RefName
	Include RefName
}

// MergeBaseRequest names the two revisions whose best common ancestor is wanted.
type MergeBaseRequest struct {
	Left  RefName
	Right RefName
}

// LogRequest selects commits. Limit 0 means no limit. Range's zero value means "from HEAD".
type LogRequest struct {
	Range RangeSpec
	Limit int
}

// LogEntry is one commit from Log.
type LogEntry struct {
	SHA     CommitSHA
	Subject string
}

// ListRefsRequest limits a for-each-ref listing. Limit 0 means no limit.
type ListRefsRequest struct {
	Pattern RefPattern
	Limit   int
}

// ListBranchesRequest selects branches. Contains, when set, keeps only branches containing it.
type ListBranchesRequest struct {
	IncludeRemote bool
	Contains      CommitSHA
}

// BranchInfo is one branch from ListBranches.
type BranchInfo struct {
	Name     BranchName
	ShortSHA string
	Upstream RefName
}

// CreateBranchRequest creates Name at Base (HEAD when Base is empty) without switching to it.
type CreateBranchRequest struct {
	Name BranchName
	Base RefName
}

// SwitchRequest is `git checkout`: switch to Branch, creating it at Base first when Create.
type SwitchRequest struct {
	Branch BranchName
	Create bool
	Base   RefName
}

// AddRequest stages Paths, or everything when All is set.
type AddRequest struct {
	Paths []RepoPath
	All   bool
}

// RestoreRequest unstages (Staged) and/or discards (Worktree) changes to Paths.
type RestoreRequest struct {
	Paths    []RepoPath
	Staged   bool
	Worktree bool
}

// CommitRequest commits the index. Amend rewrites HEAD; an empty Message with Amend keeps the
// previous message.
type CommitRequest struct {
	Message string
	Amend   bool
}

// StashRequest pushes the working tree onto the stash.
type StashRequest struct {
	Message string
}

// FetchRequest. Branch limits the fetch to one branch of Remote; All fetches every remote.
// Remote's zero value means git's default remote.
type FetchRequest struct {
	Remote RemoteName
	Branch BranchName
	All    bool
	Prune  bool
}

// PushRequest. Zero Remote/Branch push git's configured upstream.
type PushRequest struct {
	Remote      RemoteName
	Branch      BranchName
	SetUpstream bool
	Force       bool
}

// PullRequest. Zero Remote/Branch pull git's configured upstream.
type PullRequest struct {
	Remote RemoteName
	Branch BranchName
}

// CloneRequest clones URL into the destination location passed to Backend.Clone.
type CloneRequest struct {
	URL RemoteURL
}

// SetConfigRequest writes one repository-local config value.
type SetConfigRequest struct {
	Key   ConfigKey
	Value string
}

// SetRemoteURLRequest is `git remote set-url`.
type SetRemoteURLRequest struct {
	Remote RemoteName
	URL    RemoteURL
}

// AddWorktreeRequest creates a linked worktree at Path on a new Branch cut from Base (HEAD
// when Base is empty).
type AddWorktreeRequest struct {
	Path   WorktreePath
	Branch BranchName
	Base   RefName
}

// RemoveWorktreeRequest removes the linked worktree at Path.
type RemoveWorktreeRequest struct {
	Path  WorktreePath
	Force bool
}

// WorktreeInfo is one entry of `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path     WorktreePath
	HeadSHA  CommitSHA
	Branch   RefName // empty when detached or bare
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool
}
