package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiffStats holds statistics about the changes in a diff
type DiffStats struct {
	// Content is the full diff content
	Content string
	// Added is the number of added lines
	Added int
	// Removed is the number of removed lines
	Removed int
	// Error holds any error that occurred during diff computation
	// This allows propagating setup errors (like missing base commit) without breaking the flow
	Error error
}

func (d *DiffStats) IsEmpty() bool {
	return d.Added == 0 && d.Removed == 0 && d.Content == ""
}

// Diff returns the git diff between the worktree and the base branch along with statistics
func (g *GitWorktree) Diff() *DiffStats {
	stats := &DiffStats{}

	// Check if the worktree path exists
	if _, err := os.Stat(g.worktreePath); os.IsNotExist(err) {
		stats.Error = fmt.Errorf("worktree path does not exist: %s", g.worktreePath)
		return stats
	}

	// Check if the directory is actually a git repository
	gitDir := filepath.Join(g.worktreePath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// This isn't a git repository - this is common when sessions are created
		// in non-git directories. Return empty stats without error to avoid spam
		return stats
	}

	// Check if base commit SHA is set (required for diff operations)
	baseCommitSHA := g.GetBaseCommitSHA()
	if baseCommitSHA == "" {
		// Base commit not set yet (common during setup), return empty stats
		return stats
	}

	// -N stages untracked files (intent to add), including them in the diff
	_, err := g.runGitCommand(g.worktreePath, "add", "-N", ".")
	if err != nil {
		// Check if this is a "not a git repository" error and handle gracefully
		if strings.Contains(err.Error(), "not a git repository") {
			// The directory lost its git status or was corrupted, return empty stats
			return stats
		}
		stats.Error = err
		return stats
	}

	content, err := g.runGitCommand(g.worktreePath, "--no-pager", "diff", baseCommitSHA)
	if err != nil {
		// Check if this is a "not a git repository" error and handle gracefully
		if strings.Contains(err.Error(), "not a git repository") {
			// The directory lost its git status or was corrupted, return empty stats
			return stats
		}
		stats.Error = err
		return stats
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Removed++
		}
	}
	stats.Content = content

	return stats
}
