package vcs

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"claude-squad/log"
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

// run executes a git command and returns the output
func (g *GitClient) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.DebugLog.Printf("[Git] Running: git %s", strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetStatus returns the working copy status
func (g *GitClient) GetStatus() (*WorkingCopyStatus, error) {
	output, err := g.run("status", "--porcelain")
	if err != nil {
		return nil, err
	}

	status := &WorkingCopyStatus{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}

		// Git porcelain format: XY filename
		indexStatus := line[0]
		workTreeStatus := line[1]

		switch {
		case indexStatus == 'A' || workTreeStatus == 'A':
			status.AddedFiles++
		case indexStatus == 'D' || workTreeStatus == 'D':
			status.DeletedFiles++
		case indexStatus == 'M' || workTreeStatus == 'M':
			status.ModifiedFiles++
		case indexStatus == 'U' || workTreeStatus == 'U':
			status.ConflictedFiles++
		case indexStatus == '?' && workTreeStatus == '?':
			status.AddedFiles++ // Untracked files count as added
		}
	}

	status.HasChanges = status.ModifiedFiles > 0 || status.AddedFiles > 0 ||
		status.DeletedFiles > 0 || status.ConflictedFiles > 0

	return status, nil
}

// HasUncommittedChanges checks if there are uncommitted changes
func (g *GitClient) HasUncommittedChanges() (bool, error) {
	// Check for staged changes
	staged, err := g.run("diff", "--cached", "--quiet")
	if err != nil {
		// Non-zero exit means there are staged changes
		if strings.Contains(err.Error(), "failed") {
			return true, nil
		}
	}

	// Check for unstaged changes
	unstaged, err := g.run("diff", "--quiet")
	if err != nil {
		// Non-zero exit means there are unstaged changes
		if strings.Contains(err.Error(), "failed") {
			return true, nil
		}
	}

	// Check for untracked files
	untracked, err := g.run("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false, err
	}

	hasChanges := staged != "" || unstaged != "" || untracked != ""
	return hasChanges, nil
}

// GetCurrentRevision returns the current revision
func (g *GitClient) GetCurrentRevision() (*Revision, error) {
	// Get commit info
	format := "%H|%h|%s|%an|%at"
	output, err := g.run("log", "-1", "--format="+format)
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(output, "|", 5)
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected git log output: %s", output)
	}

	rev := &Revision{
		ID:          parts[0],
		ShortID:     parts[1],
		Description: parts[2],
		Author:      parts[3],
		IsCurrent:   true,
	}

	// Parse Unix timestamp
	if ts, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
		rev.Timestamp = time.Unix(ts, 0)
	}

	// Get branches pointing to this commit
	branches, _ := g.getBranchesForCommit(rev.ID)
	rev.Bookmarks = branches

	return rev, nil
}

// GetCurrentBookmark returns the current branch name
func (g *GitClient) GetCurrentBookmark() (string, error) {
	output, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}

	branch := strings.TrimSpace(output)
	if branch == "HEAD" {
		// Detached HEAD state
		return "", nil
	}

	return branch, nil
}

// getBranchesForCommit gets branches pointing to a commit
func (g *GitClient) getBranchesForCommit(commitID string) ([]string, error) {
	output, err := g.run("branch", "--contains", commitID, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(output, "\n") {
		if branch := strings.TrimSpace(line); branch != "" {
			branches = append(branches, branch)
		}
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
			_, err := g.run("stash", "push", "-m", "claude-squad: WIP before workspace switch")
			if err != nil {
				return fmt.Errorf("failed to stash changes: %w", err)
			}
			log.InfoLog.Printf("[Git] Stashed uncommitted changes")

		case BringAlong:
			// Stash changes to restore after switch
			_, err := g.run("stash", "push", "-m", "claude-squad: bringing changes along")
			if err != nil {
				return fmt.Errorf("failed to stash changes: %w", err)
			}
			g.stashRef = "stash@{0}" // Remember we stashed
			log.InfoLog.Printf("[Git] Stashed changes to bring along")

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
		_, err := g.run("checkout", "-b", target, base)
		if err != nil {
			return fmt.Errorf("failed to create branch %s: %w", target, err)
		}
		log.InfoLog.Printf("[Git] Created and switched to branch %s", target)
	} else if targetExists {
		// Switch to existing branch
		_, err := g.run("checkout", target)
		if err != nil {
			return fmt.Errorf("failed to switch to %s: %w", target, err)
		}
		log.InfoLog.Printf("[Git] Switched to %s", target)
	} else {
		return fmt.Errorf("branch or commit not found: %s", target)
	}

	// 3. Restore stashed changes if BringAlong
	if g.stashRef != "" {
		_, err := g.run("stash", "pop")
		if err != nil {
			log.WarningLog.Printf("[Git] Failed to pop stash: %v (changes saved in stash)", err)
			// Don't fail the switch - changes are safe in stash
		} else {
			log.InfoLog.Printf("[Git] Restored stashed changes")
		}
		g.stashRef = ""
	}

	return nil
}

// branchExists checks if a branch exists
func (g *GitClient) branchExists(name string) bool {
	_, err := g.run("rev-parse", "--verify", name)
	return err == nil
}

// CreateBookmark creates a new branch at the specified base
func (g *GitClient) CreateBookmark(name string, base string) error {
	if base == "" {
		base = "HEAD"
	}

	_, err := g.run("branch", name, base)
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", name, err)
	}

	log.InfoLog.Printf("[Git] Created branch %s at %s", name, base)
	return nil
}

// DescribeWIP is a no-op for Git (changes are always in working tree)
func (g *GitClient) DescribeWIP(message string) error {
	// Git doesn't have the concept of describing uncommitted changes
	// We could create a commit, but that changes the history
	// For now, this is a no-op
	log.DebugLog.Printf("[Git] DescribeWIP called (no-op): %s", message)
	return nil
}

// AbandonChanges discards all uncommitted changes
func (g *GitClient) AbandonChanges() error {
	// Reset staged changes
	if _, err := g.run("reset", "HEAD"); err != nil {
		// Ignore errors - might be on initial commit
	}

	// Discard working tree changes
	if _, err := g.run("checkout", "."); err != nil {
		return fmt.Errorf("failed to discard changes: %w", err)
	}

	// Remove untracked files
	if _, err := g.run("clean", "-fd"); err != nil {
		log.WarningLog.Printf("[Git] Failed to clean untracked files: %v", err)
	}

	log.InfoLog.Printf("[Git] Abandoned all uncommitted changes")
	return nil
}

// ListBookmarks returns all branches
func (g *GitClient) ListBookmarks() ([]Bookmark, error) {
	output, err := g.run("branch", "-a", "--format=%(refname:short)|%(objectname:short)|%(upstream:short)")
	if err != nil {
		return nil, err
	}

	var bookmarks []Bookmark
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		name := parts[0]
		revID := ""
		upstream := ""

		if len(parts) > 1 {
			revID = parts[1]
		}
		if len(parts) > 2 {
			upstream = parts[2]
		}

		isRemote := strings.HasPrefix(name, "remotes/") || strings.HasPrefix(name, "origin/")

		bookmarks = append(bookmarks, Bookmark{
			Name:       name,
			RevisionID: revID,
			IsRemote:   isRemote,
			Upstream:   upstream,
		})
	}

	return bookmarks, nil
}

// ListRecentRevisions returns recent commits
func (g *GitClient) ListRecentRevisions(limit int) ([]Revision, error) {
	format := "%H|%h|%s|%an|%at"
	output, err := g.run("log", fmt.Sprintf("-%d", limit), "--format="+format)
	if err != nil {
		return nil, err
	}

	var revisions []Revision
	scanner := bufio.NewScanner(strings.NewReader(output))
	isFirst := true
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		rev := Revision{
			ID:          parts[0],
			ShortID:     parts[1],
			Description: parts[2],
			Author:      parts[3],
			IsCurrent:   isFirst,
		}
		isFirst = false

		if ts, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
			rev.Timestamp = time.Unix(ts, 0)
		}

		revisions = append(revisions, rev)
	}

	return revisions, nil
}

// CreateWorktree creates a new Git worktree
func (g *GitClient) CreateWorktree(path string, branch string) error {
	args := []string{"worktree", "add", path}
	if branch != "" {
		args = append(args, "-b", branch)
	}

	_, err := g.run(args...)
	if err != nil {
		return fmt.Errorf("failed to create worktree at %s: %w", path, err)
	}

	log.InfoLog.Printf("[Git] Created worktree at %s with branch %s", path, branch)
	return nil
}

// ListWorktrees returns all Git worktrees
func (g *GitClient) ListWorktrees() ([]Worktree, error) {
	output, err := g.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current *Worktree

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			current = &Worktree{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if current != nil {
			if strings.HasPrefix(line, "HEAD ") {
				current.RevisionID = strings.TrimPrefix(line, "HEAD ")
			} else if strings.HasPrefix(line, "branch ") {
				branch := strings.TrimPrefix(line, "branch ")
				branch = strings.TrimPrefix(branch, "refs/heads/")
				current.Bookmark = branch
				current.Name = branch // Use branch as name
			}
		}
	}

	if current != nil {
		worktrees = append(worktrees, *current)
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

// GetGitVersion returns the installed Git version
func GetGitVersion() (string, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
