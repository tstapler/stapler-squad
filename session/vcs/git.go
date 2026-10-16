package vcs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

// GitClient implements the VCS interface for Git
type GitClient struct {
	repoPath string
	// stashRef holds the stash reference for BringAlong strategy
	stashRef string
}

// NewGitClient creates a new Git client for the given repository path
func NewGitClient(repoPath string) *GitClient {
	return &GitClient{
		repoPath: repoPath,
	}
}

// Type returns the VCS type
func (g *GitClient) Type() VCSType {
	return VCSTypeGit
}

// RepoPath returns the repository path
func (g *GitClient) RepoPath() string {
	return g.repoPath
}

const gitCallTimeout = 30 * time.Second

func (g *GitClient) loc() backend.RepoLocation {
	return backend.Local{Root: backend.RepoRoot(g.repoPath)}
}

// gitCtx bounds one backend call.
func gitCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), gitCallTimeout)
}

// GetStatus returns the working copy status. Untracked files are listed
// individually (`--untracked-files=all`), so an untracked directory counts
// once per file rather than once per directory.
func (g *GitClient) GetStatus() (*WorkingCopyStatus, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	res, err := gitprovider.Backend().Status(ctx, g.loc(), backend.IntentDisplay)
	if err != nil {
		return nil, err
	}

	status := &WorkingCopyStatus{}
	for _, f := range res.Files {
		if f.Ignored {
			continue
		}
		if f.Untracked {
			status.AddedFiles++ // Untracked files count as added
			continue
		}
		index, workTree := byte(f.Index), byte(f.Worktree)
		switch {
		case index == 'A' || workTree == 'A':
			status.AddedFiles++
		case index == 'D' || workTree == 'D':
			status.DeletedFiles++
		case index == 'M' || workTree == 'M':
			status.ModifiedFiles++
		case index == 'U' || workTree == 'U':
			status.ConflictedFiles++
		}
	}

	status.HasChanges = status.ModifiedFiles > 0 || status.AddedFiles > 0 ||
		status.DeletedFiles > 0 || status.ConflictedFiles > 0

	return status, nil
}

// HasUncommittedChanges checks for staged, unstaged or untracked changes.
func (g *GitClient) HasUncommittedChanges() (bool, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	return gitprovider.Backend().IsDirty(ctx, g.loc(), backend.IntentDestructive)
}

// GetCurrentRevision returns the current revision
func (g *GitClient) GetCurrentRevision() (*Revision, error) {
	revs, err := g.recentRevisions(1)
	if err != nil {
		return nil, err
	}
	if len(revs) == 0 {
		return nil, errors.New("git log returned no commits")
	}
	rev := revs[0]

	// Get branches pointing to this commit
	branches, _ := g.getBranchesForCommit(rev.ID)
	rev.Bookmarks = branches

	return &rev, nil
}

// recentRevisions returns up to limit commits from HEAD, newest first, the
// first marked current.
func (g *GitClient) recentRevisions(limit int) ([]Revision, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	entries, err := gitprovider.Backend().Log(ctx, g.loc(), backend.LogRequest{Limit: limit, Detail: true})
	if err != nil {
		return nil, err
	}
	revisions := make([]Revision, 0, len(entries))
	for i, e := range entries {
		revisions = append(revisions, Revision{
			ID:          string(e.SHA),
			ShortID:     e.ShortSHA,
			Description: e.Subject,
			Author:      e.Author,
			Timestamp:   e.AuthorTime,
			IsCurrent:   i == 0,
		})
	}
	return revisions, nil
}

// GetCurrentBookmark returns the current branch name
func (g *GitClient) GetCurrentBookmark() (string, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	branch, err := gitprovider.Backend().CurrentBranch(ctx, g.loc())
	if errors.Is(err, backend.ErrDetachedHead) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(branch), nil
}

// getBranchesForCommit gets local branches containing a commit
func (g *GitClient) getBranchesForCommit(commitID string) ([]string, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	infos, err := gitprovider.Backend().ListBranches(ctx, g.loc(), backend.ListBranchesRequest{Contains: backend.CommitSHA(commitID)})
	if err != nil {
		return nil, err
	}
	branches := make([]string, 0, len(infos))
	for _, b := range infos {
		branches = append(branches, string(b.Name))
	}
	return branches, nil
}

// SwitchTo switches to the target branch or commit
func (g *GitClient) SwitchTo(target string, opts SwitchOptions) error {
	// 1. Handle uncommitted changes
	hasChanges, err := g.HasUncommittedChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if hasChanges {
		switch opts.ChangeStrategy {
		case KeepAsWIP:
			// Stash changes
			if err := g.stashPush("stapler-squad: WIP before workspace switch"); err != nil {
				return fmt.Errorf("failed to stash changes: %w", err)
			}
			log.Info("stashed uncommitted changes")

		case BringAlong:
			// Stash changes to restore after switch
			if err := g.stashPush("stapler-squad: bringing changes along"); err != nil {
				return fmt.Errorf("failed to stash changes: %w", err)
			}
			g.stashRef = "stash@{0}" // Remember we stashed
			log.Info("stashed changes to bring along")

		case Abandon:
			if err := g.AbandonChanges(); err != nil {
				return fmt.Errorf("failed to abandon changes: %w", err)
			}
		}
	}

	// 2. Check if target exists
	targetExists := g.branchExists(target)

	if !targetExists && opts.CreateIfMissing {
		// Create new branch
		base := opts.BaseRevision
		if base == "" {
			base = "HEAD"
		}
		if err := g.switchBranch(backend.SwitchRequest{Branch: backend.BranchName(target), Create: true, Base: backend.RefName(base)}); err != nil {
			return fmt.Errorf("failed to create branch %s: %w", target, err)
		}
		log.Info("created and switched to branch", "branch", target)
	} else if targetExists {
		// Switch to existing branch
		if err := g.checkout(target); err != nil {
			return fmt.Errorf("failed to switch to %s: %w", target, err)
		}
		log.Info("switched to branch", "branch", target)
	} else {
		return fmt.Errorf("branch or commit not found: %s", target)
	}

	// 3. Restore stashed changes if BringAlong
	if g.stashRef != "" {
		if err := g.stashPop(); err != nil {
			log.Warn("failed to pop stash", "err", err)
			// Don't fail the switch - changes are safe in stash
		} else {
			log.Info("restored stashed changes")
		}
		g.stashRef = ""
	}

	return nil
}

// branchExists checks if a branch exists
func (g *GitClient) branchExists(name string) bool {
	ctx, cancel := gitCtx()
	defer cancel()
	exists, err := gitprovider.Backend().RefExists(ctx, g.loc(), backend.RefName(name))
	return err == nil && exists
}

// CreateBookmark creates a new branch at the specified base
func (g *GitClient) CreateBookmark(name string, base string) error {
	if base == "" {
		base = "HEAD"
	}

	ctx, cancel := gitCtx()
	defer cancel()
	err := gitprovider.Backend().CreateBranch(ctx, g.loc(), backend.CreateBranchRequest{Name: backend.BranchName(name), Base: backend.RefName(base)})
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", name, err)
	}

	log.Info("created branch", "branch", name, "base", base)
	return nil
}

// DescribeWIP is a no-op for Git (changes are always in working tree)
func (g *GitClient) DescribeWIP(message string) error {
	// Git doesn't have the concept of describing uncommitted changes
	// We could create a commit, but that changes the history
	// For now, this is a no-op
	log.Debug("DescribeWIP called (no-op)", "message", message)
	return nil
}

// AbandonChanges discards all uncommitted changes
func (g *GitClient) AbandonChanges() error {
	ctx, cancel := gitCtx()
	defer cancel()
	if err := gitprovider.Backend().DiscardChanges(ctx, g.loc()); err != nil {
		return fmt.Errorf("failed to discard changes: %w", err)
	}

	log.Info("abandoned all uncommitted changes")
	return nil
}

func (g *GitClient) stashPush(message string) error {
	ctx, cancel := gitCtx()
	defer cancel()
	return gitprovider.Backend().StashPush(ctx, g.loc(), backend.StashRequest{Message: message})
}

func (g *GitClient) stashPop() error {
	ctx, cancel := gitCtx()
	defer cancel()
	return gitprovider.Backend().StashPop(ctx, g.loc())
}

func (g *GitClient) switchBranch(req backend.SwitchRequest) error {
	ctx, cancel := gitCtx()
	defer cancel()
	return gitprovider.Backend().SwitchBranch(ctx, g.loc(), req)
}

// checkout moves to an existing branch, or detaches onto any other commit-ish
// (tag, SHA) the way `git checkout <target>` does; SwitchBranch alone refuses
// non-branches. A local branch that cannot be switched to (e.g. checked out in
// another worktree) must fail rather than silently detach.
func (g *GitClient) checkout(target string) error {
	switchErr := g.switchBranch(backend.SwitchRequest{Branch: backend.BranchName(target)})
	if switchErr == nil {
		return nil
	}
	ctx, cancel := gitCtx()
	defer cancel()
	isLocalBranch, err := gitprovider.Backend().RefExists(ctx, g.loc(), backend.RefName("refs/heads/"+target))
	if err != nil || isLocalBranch {
		return switchErr
	}
	if err := gitprovider.Backend().CheckoutCommit(ctx, g.loc(), backend.CommitSHA(target)); err != nil {
		return switchErr
	}
	return nil
}

// ListBookmarks returns all branches
func (g *GitClient) ListBookmarks() ([]Bookmark, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	infos, err := gitprovider.Backend().ListBranches(ctx, g.loc(), backend.ListBranchesRequest{IncludeRemote: true})
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	for _, b := range infos {
		name := string(b.Name)
		isRemote := strings.HasPrefix(name, "remotes/") || strings.HasPrefix(name, "origin/")

		bookmarks = append(bookmarks, Bookmark{
			Name:       name,
			RevisionID: b.ShortSHA,
			IsRemote:   isRemote,
			Upstream:   string(b.Upstream),
		})
	}

	return bookmarks, nil
}

// ListRecentRevisions returns recent commits
func (g *GitClient) ListRecentRevisions(limit int) ([]Revision, error) {
	if limit <= 0 {
		return nil, nil // `git log -0` prints nothing; Backend.Log treats 0 as unlimited
	}
	return g.recentRevisions(limit)
}

// CreateWorktree creates a new Git worktree
func (g *GitClient) CreateWorktree(path string, branch string) error {
	ctx, cancel := gitCtx()
	defer cancel()
	err := gitprovider.Backend().AddWorktree(ctx, g.loc(), backend.AddWorktreeRequest{Path: backend.WorktreePath(path), Branch: backend.BranchName(branch)})
	if err != nil {
		return fmt.Errorf("failed to create worktree at %s: %w", path, err)
	}

	log.Info("created worktree", "path", path, "branch", branch)
	return nil
}

// ListWorktrees returns all Git worktrees
func (g *GitClient) ListWorktrees() ([]Worktree, error) {
	ctx, cancel := gitCtx()
	defer cancel()
	infos, err := gitprovider.Backend().ListWorktrees(ctx, g.loc())
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	for _, info := range infos {
		wt := Worktree{Path: string(info.Path), RevisionID: string(info.HeadSHA)}
		if info.Branch != "" {
			branch := strings.TrimPrefix(string(info.Branch), "refs/heads/")
			wt.Bookmark = branch
			wt.Name = branch // Use branch as name
		}
		worktrees = append(worktrees, wt)
	}

	// Mark current worktree
	if len(worktrees) > 0 && g.repoPath != "" {
		for i := range worktrees {
			if worktrees[i].Path == g.repoPath {
				worktrees[i].IsCurrent = true
				break
			}
		}
	}

	return worktrees, nil
}
