package vc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

type branchCacheEntry struct {
	branch string
	expiry int64 // unix nanoseconds
}

// GitProvider implements VCSProvider for Git repositories
type GitProvider struct {
	workDir     string
	repoRoot    string
	branchCache atomic.Pointer[branchCacheEntry]
}

// NewGitProvider creates a new Git provider for the given directory
func NewGitProvider(path string) (*GitProvider, error) {
	root, err := FindVCSRoot(path, VCSGit)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	return &GitProvider{
		workDir:  path,
		repoRoot: root,
	}, nil
}

func (g *GitProvider) Type() VCSType {
	return VCSGit
}

func (g *GitProvider) Name() string {
	return "Git"
}

func (g *GitProvider) WorkDir() string {
	return g.workDir
}

const gitCallTimeout = 30 * time.Second

// loc is the repository this provider operates on.
func (g *GitProvider) loc() backend.RepoLocation {
	return backend.Local{Root: backend.RepoRoot(g.repoRoot)}
}

// gitCtx bounds one backend call.
func gitCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), gitCallTimeout)
}

// shortHashLen is how many leading hex characters of a full commit OID are
// shown as the "short" SHA in status/display output -- a fixed prefix rather
// than git's own core.abbrev=auto disambiguation, acceptable since
// HeadCommit is display-only, never used to look up objects.
const shortHashLen = 7

// truncateHash returns oid's leading shortHashLen characters, or oid
// unchanged if it's already that short or shorter.
func truncateHash(oid string) string {
	if len(oid) > shortHashLen {
		return oid[:shortHashLen]
	}
	return oid
}

// GetStatus reports the working tree's full status in 3 git spawns: one
// `git status --branch`, one `git log` for the commit description, and the two
// `git diff --numstat` calls GetChangedFiles already needs. Branch/HEAD/
// upstream/ahead-behind come from the "# branch.*" header --branch adds to
// porcelain v2 output (parsed by the backend into StatusResult) instead of
// four separate branch/rev-parse/rev-list spawns -- each spawn takes the
// runtime's global syscall.ForkLock, so this count directly drove measured
// mutex contention under concurrent session polling.
func (g *GitProvider) GetStatus() (*VCSStatus, error) {
	status := &VCSStatus{
		Type: VCSGit,
	}

	ctx, cancel := gitCtx()
	defer cancel()
	res, err := gitprovider.Backend().Status(ctx, g.loc(), backend.IntentDisplay)
	if err != nil {
		return status, err
	}

	if res.Branch == "" && !res.Detached && res.HeadOID == "" && !res.Unborn {
		// --branch header missing or unparseable (unexpected git version/
		// locale) -- fall back to GetBranch's own dedicated call rather than
		// silently leaving status.Branch empty.
		if branch, err := g.GetBranch(); err == nil {
			status.Branch = branch
		}
	} else {
		g.applyBranchHeader(status, res)
	}

	if entries, logErr := gitprovider.Backend().Log(ctx, g.loc(), backend.LogRequest{Limit: 1}); logErr == nil && len(entries) > 0 {
		status.Description = entries[0].Subject
	}

	// Changed files come from the status result already fetched above --
	// avoids a second `git status` spawn.
	files := g.changedFilesFromStatus(ctx, res)

	// Categorize files
	for _, f := range files {
		switch {
		case f.Status == FileConflict:
			status.ConflictFiles = append(status.ConflictFiles, f)
			status.HasConflicts = true
		case f.Status == FileUntracked:
			status.UntrackedFiles = append(status.UntrackedFiles, f)
			status.HasUntracked = true
		case f.IsStaged:
			status.StagedFiles = append(status.StagedFiles, f)
			status.HasStaged = true
		default:
			status.UnstagedFiles = append(status.UnstagedFiles, f)
			status.HasUnstaged = true
		}
	}

	status.IsClean = !status.HasStaged && !status.HasUnstaged && !status.HasUntracked && !status.HasConflicts

	return status, nil
}

const branchCacheTTL = 30 * time.Second

func (g *GitProvider) GetBranch() (string, error) {
	// ponytail: cache hit is lock-free; branch changes are rare relative to RPC call rate
	if e := g.branchCache.Load(); e != nil && time.Now().UnixNano() < e.expiry {
		return e.branch, nil
	}
	ctx, cancel := gitCtx()
	defer cancel()
	name, err := gitprovider.Backend().CurrentBranch(ctx, g.loc())
	if err != nil && !errors.Is(err, backend.ErrDetachedHead) {
		return "", err
	}
	branch := string(name)
	if errors.Is(err, backend.ErrDetachedHead) {
		// Full OID + truncateHash (not `rev-parse --short`) so this matches
		// applyBranchHeader's fixed-length format exactly -- both write into
		// the same branchCache, and a mismatched hash length between them
		// would make the cached detached-HEAD label's length change depending
		// on which call happened to populate the cache last.
		sha, headErr := g.headSHA()
		if headErr != nil {
			return "", err
		}
		branch = "(detached: " + truncateHash(sha) + ")"
	}
	g.branchCache.Store(&branchCacheEntry{branch, time.Now().Add(branchCacheTTL).UnixNano()})
	return branch, nil
}

// headSHA resolves HEAD to a full commit OID through the git backend.
func (g *GitProvider) headSHA() (string, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	sha, err := gitprovider.Backend().ResolveRef(ctx, g.loc(), "HEAD")
	return string(sha), err
}

// GetChangedFiles returns the current working-tree/index status as a flat
// list of FileChange entries: the backend's parsed porcelain status, then
// per-file insertion/deletion counts merged in (mergeNumstat).
func (g *GitProvider) GetChangedFiles() ([]FileChange, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	res, err := gitprovider.Backend().Status(ctx, g.loc(), backend.IntentDisplay)
	if err != nil {
		return nil, err
	}
	return g.changedFilesFromStatus(ctx, res), nil
}

// changedFilesFromStatus converts an already-fetched StatusResult into
// FileChange entries with numstat counts merged in. Split out from
// GetChangedFiles so GetStatus can reuse the single status call it already
// made instead of running `git status` a second time.
//
// Numstat is skipped entirely when there are no changed files: with --branch
// the status output always carries header lines, so only the parsed file
// list says whether the repo is clean -- otherwise both `git diff --numstat`
// calls would run unconditionally on every clean-repo poll.
func (g *GitProvider) changedFilesFromStatus(ctx context.Context, res backend.StatusResult) []FileChange {
	files := fileChangesFromStatus(res)
	if len(files) == 0 {
		return nil
	}

	// Best-effort per-file insertion/deletion counts. A numstat failure (e.g. no
	// commits yet) should not prevent the file list itself from being returned.
	var unstagedStats, stagedStats map[string]numstatEntry
	if stats, statErr := g.getNumstat(ctx, false); statErr == nil {
		unstagedStats = stats
	}
	if stats, statErr := g.getNumstat(ctx, true); statErr == nil {
		stagedStats = stats
	}

	return mergeNumstat(files, unstagedStats, stagedStats)
}

// applyBranchHeader fills status's branch/HEAD/upstream/ahead-behind fields
// from the backend's parsed status header, and keeps GetBranch's cache warm
// with the same result so a standalone GetBranch call after GetStatus is a
// cache hit instead of another subprocess spawn.
func (g *GitProvider) applyBranchHeader(status *VCSStatus, res backend.StatusResult) {
	branch := string(res.Branch)
	if res.Detached {
		branch = "(detached: " + truncateHash(string(res.HeadOID)) + ")"
	}
	status.Branch = branch
	if branch != "" {
		g.branchCache.Store(&branchCacheEntry{branch, time.Now().Add(branchCacheTTL).UnixNano()})
	}

	if res.HeadOID != "" {
		status.HeadCommit = truncateHash(string(res.HeadOID))
	}

	status.Upstream = string(res.Upstream)
	status.AheadBy = res.Ahead
	status.BehindBy = res.Behind
}

// fileChangesFromStatus maps the backend's parsed porcelain v2 records onto
// FileChange entries. Additions/Deletions are left zero-valued -- merging
// numstat counts in is a separate step, see mergeNumstat. A path with both an
// index and a worktree change yields two entries (one staged, one not).
func fileChangesFromStatus(res backend.StatusResult) []FileChange {
	var files []FileChange
	for _, f := range res.Files {
		path := string(f.Path)
		switch {
		case f.Ignored:
			// Ignored files - skip
		case f.Untracked:
			files = append(files, FileChange{Path: path, Status: FileUntracked, IsStaged: false})
		case f.Unmerged || isConflictCode(f.Index, f.Worktree):
			oldPath := string(f.OrigPath)
			if f.Unmerged {
				oldPath = "" // "u" records carry no original path
			}
			files = append(files, FileChange{Path: path, Status: FileConflict, IsStaged: false, OldPath: oldPath})
		default:
			oldPath := string(f.OrigPath)
			// X = status in index (staged), Y = status in worktree (unstaged)
			if f.Index != '.' && f.Index != ' ' {
				files = append(files, FileChange{Path: path, Status: parseGitStatusChar(byte(f.Index)), IsStaged: true, OldPath: oldPath})
			}
			if f.Worktree != '.' && f.Worktree != ' ' {
				files = append(files, FileChange{Path: path, Status: parseGitStatusChar(byte(f.Worktree)), IsStaged: false, OldPath: oldPath})
			}
		}
	}
	return files
}

// isConflictCode reports the unmerged XY pairs git can emit on a "1"/"2" record.
func isConflictCode(index, worktree backend.StatusCode) bool {
	return index == 'U' || worktree == 'U' ||
		(index == 'A' && worktree == 'A') ||
		(index == 'D' && worktree == 'D')
}

// mergeNumstat applies per-file insertion/deletion counts onto files,
// selecting stagedStats or unstagedStats per file based on its IsStaged
// flag. Files with no matching numstat entry (e.g. untracked files, or a
// numstat lookup that failed) are left with zero Additions/Deletions.
func mergeNumstat(files []FileChange, unstagedStats, stagedStats map[string]numstatEntry) []FileChange {
	for i := range files {
		stats := unstagedStats
		if files[i].IsStaged {
			stats = stagedStats
		}
		if entry, ok := stats[files[i].Path]; ok {
			files[i].Additions = entry.additions
			files[i].Deletions = entry.deletions
		}
	}
	return files
}

// numstatEntry holds the per-file insertion/deletion counts reported by
// `git diff --numstat`.
type numstatEntry struct {
	additions int
	deletions int
}

// getNumstat returns per-file insertion/deletion counts keyed by the file's
// current (post-change) path. Pass staged=true for staged changes, false for
// unstaged. Binary files report no counts, which are treated as 0.
func (g *GitProvider) getNumstat(ctx context.Context, staged bool) (map[string]numstatEntry, error) {
	rows, err := gitprovider.Backend().DiffNumstat(ctx, g.loc(), backend.DiffSpec{Staged: staged, Intent: backend.IntentDisplay})
	if err != nil {
		return nil, err
	}
	result := make(map[string]numstatEntry, len(rows))
	for _, r := range rows {
		result[string(r.Path)] = numstatEntry{additions: r.Added, deletions: r.Deleted}
	}
	return result, nil
}

// parseGitStatusChar converts a git status character to FileStatus
func parseGitStatusChar(c byte) FileStatus {
	switch c {
	case 'M':
		return FileModified
	case 'A':
		return FileAdded
	case 'D':
		return FileDeleted
	case 'R':
		return FileRenamed
	case 'C':
		return FileCopied
	case '?':
		return FileUntracked
	case '!':
		return FileIgnored
	case 'U':
		return FileConflict
	default:
		return FileModified
	}
}

// do runs one backend mutation under the provider's call timeout.
func (g *GitProvider) do(op func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error) error {
	ctx, cancel := gitCtx()
	defer cancel()
	return op(ctx, gitprovider.Backend(), g.loc())
}

func (g *GitProvider) Stage(path string) error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Add(ctx, loc, backend.AddRequest{Paths: []backend.RepoPath{backend.RepoPath(path)}})
	})
}

func (g *GitProvider) StageAll() error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Add(ctx, loc, backend.AddRequest{All: true})
	})
}

func (g *GitProvider) Unstage(path string) error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Restore(ctx, loc, backend.RestoreRequest{Paths: []backend.RepoPath{backend.RepoPath(path)}, Staged: true})
	})
}

func (g *GitProvider) UnstageAll() error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Reset(ctx, loc, backend.ResetRequest{Mode: backend.ResetMixed})
	})
}

func (g *GitProvider) Commit(message string) error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Commit(ctx, loc, backend.CommitRequest{Message: message})
	})
}

func (g *GitProvider) AmendCommit(message string) error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Commit(ctx, loc, backend.CommitRequest{Message: message, Amend: true})
	})
}

func (g *GitProvider) Push() error {
	return g.PushWithOptions(PushOptions{})
}

func (g *GitProvider) PushWithOptions(opts PushOptions) error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Push(ctx, loc, backend.PushRequest{
			Remote:      backend.RemoteName(opts.Remote),
			Branch:      backend.BranchName(opts.Branch),
			SetUpstream: opts.SetUpstream,
			Force:       opts.Force,
		})
	})
}

func (g *GitProvider) Pull() error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Pull(ctx, loc, backend.PullRequest{})
	})
}

func (g *GitProvider) Fetch() error {
	return g.do(func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error {
		return b.Fetch(ctx, loc, backend.FetchRequest{})
	})
}

// diffText returns a diff trimmed of surrounding whitespace, as the raw
// `git diff` runner this replaced did.
func (g *GitProvider) diffText(spec backend.DiffSpec) (string, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	spec.Intent = backend.IntentDisplay
	out, err := gitprovider.Backend().Diff(ctx, g.loc(), spec)
	return strings.TrimSpace(out), err
}

func (g *GitProvider) GetFileDiff(path string) (string, error) {
	paths := []backend.RepoPath{backend.RepoPath(path)}
	// Try staged first, then unstaged
	output, err := g.diffText(backend.DiffSpec{Staged: true, Paths: paths})
	if err == nil && output != "" {
		return output, nil
	}
	return g.diffText(backend.DiffSpec{Paths: paths})
}

func (g *GitProvider) GetDiff() (string, error) {
	// Get both staged and unstaged changes
	staged, _ := g.diffText(backend.DiffSpec{Staged: true})
	unstaged, _ := g.diffText(backend.DiffSpec{})

	if staged != "" && unstaged != "" {
		return staged + "\n" + unstaged, nil
	}
	if staged != "" {
		return staged, nil
	}
	return unstaged, nil
}

func (g *GitProvider) GetInteractiveCommand() string {
	// Check for available interactive tools in order of preference
	if IsToolAvailable("lazygit") {
		return "lazygit"
	}
	if IsToolAvailable("tig") {
		return "tig"
	}
	if IsToolAvailable("gitui") {
		return "gitui"
	}
	// Fallback to git status in a pager
	return "git status"
}

func (g *GitProvider) GetLogCommand() string {
	if IsToolAvailable("tig") {
		return "tig"
	}
	return "git log --oneline --graph -20"
}
