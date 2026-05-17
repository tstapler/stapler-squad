package unfinished

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// GoGitVCSReader implements VCSReader using the go-git library.
// No subprocesses are spawned; all operations run in-process.
// Prefer this in environments where spawning git subprocesses is undesirable
// or where index.lock contention is a concern.
type GoGitVCSReader struct{}

var _ VCSReader = (*GoGitVCSReader)(nil)

func (g *GoGitVCSReader) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	repo, err := openWorktree(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo %s: %w", repoPath, err)
	}

	// Main worktree.
	main := WorktreeInfo{Path: repoPath}
	if head, err := repo.Head(); err == nil {
		main.HEAD = head.Hash().String()
		if head.Name().IsBranch() {
			main.Branch = head.Name().Short()
		} else {
			main.IsDetached = true
		}
	}
	worktrees := []WorktreeInfo{main}

	// Linked worktrees live in .git/worktrees/<name>/.
	worktreesDir := filepath.Join(gitCommonDir(repoPath), "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return worktrees, nil // no linked worktrees — not an error
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(worktreesDir, entry.Name())

		// gitdir file contains the absolute path to the worktree's .git file.
		gitdirData, err := os.ReadFile(filepath.Join(base, "gitdir"))
		if err != nil {
			continue
		}
		// Strip the trailing "/.git" to get the worktree path.
		wtPath := filepath.Dir(strings.TrimSpace(string(gitdirData)))

		wt := WorktreeInfo{Path: wtPath}

		// Read HEAD: either "ref: refs/heads/<branch>" or a bare SHA.
		headData, err := os.ReadFile(filepath.Join(base, "HEAD"))
		if err == nil {
			headStr := strings.TrimSpace(string(headData))
			const refPrefix = "ref: refs/heads/"
			if strings.HasPrefix(headStr, refPrefix) {
				wt.Branch = strings.TrimPrefix(headStr, refPrefix)
			} else {
				wt.IsDetached = true
				wt.HEAD = headStr
			}
		}

		if _, err := os.Stat(filepath.Join(base, "locked")); err == nil {
			wt.IsLocked = true
		}
		if _, err := os.Stat(filepath.Join(base, "gitdir")); err == nil {
			// Check prune flag.
			if _, err := os.Stat(wtPath); os.IsNotExist(err) {
				wt.IsPrunable = true
			}
		}

		worktrees = append(worktrees, wt)
	}
	return worktrees, nil
}

func (g *GoGitVCSReader) ResolveDefaultBranch(repoPath string) string {
	repo, err := openWorktree(repoPath)
	if err != nil {
		return ""
	}

	// Try refs/remotes/origin/HEAD first.
	if ref, err := repo.Reference("refs/remotes/origin/HEAD", true); err == nil {
		name := ref.Name().Short() // e.g. "origin/main"
		if name != "" {
			return name
		}
	}

	// Fall back to well-known remote tracking refs, then local.
	for _, candidate := range []string{
		"refs/remotes/origin/main", "refs/remotes/origin/master",
		"refs/remotes/origin/develop", "refs/remotes/origin/trunk",
		"refs/heads/main", "refs/heads/master",
		"refs/heads/develop", "refs/heads/trunk",
	} {
		if _, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			// Return the short name callers expect (e.g. "origin/main").
			short := plumbing.ReferenceName(candidate).Short()
			return short
		}
	}
	return ""
}

func (g *GoGitVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	repo, err := openWorktree(worktreePath)
	if err != nil {
		return false, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false, err
	}
	status, err := wt.Status()
	if err != nil {
		return false, err
	}
	return !status.IsClean(), nil
}

func (g *GoGitVCSReader) AheadBehind(worktreePath, base string) (int, int, error) {
	repo, err := openWorktree(worktreePath)
	if err != nil {
		return 0, 0, err
	}

	headRef, err := repo.Head()
	if err != nil {
		return 0, 0, err
	}

	baseHash, err := resolveRef(repo, base)
	if err != nil {
		return 0, 0, err
	}

	headSet, err := reachableSet(repo, headRef.Hash())
	if err != nil {
		return 0, 0, err
	}
	baseSet, err := reachableSet(repo, baseHash)
	if err != nil {
		return 0, 0, err
	}
	return countNotIn(headSet, baseSet), countNotIn(baseSet, headSet), nil
}

func (g *GoGitVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	repo, err := openWorktree(worktreePath)
	if err != nil {
		return nil, err
	}

	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}

	baseHash, err := resolveRef(repo, base)
	if err != nil {
		return nil, err
	}

	// Collect commits reachable from HEAD but not from base.
	baseReachable, err := reachableSet(repo, baseHash)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var msgs []string
	err = iter.ForEach(func(c *object.Commit) error {
		if baseReachable[c.Hash] {
			return storer.ErrStop
		}
		if len(msgs) < max {
			// Mimic `git log --oneline`: short hash + first line of message.
			msgs = append(msgs, c.Hash.String()[:7]+" "+firstLine(c.Message))
		}
		return nil
	})
	return msgs, err
}

func (g *GoGitVCSReader) DiffShortstat(worktreePath string) (DiffStat, error) {
	repo, err := openWorktree(worktreePath)
	if err != nil {
		return DiffStat{}, err
	}

	head, err := repo.Head()
	if err != nil {
		return DiffStat{}, err
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return DiffStat{}, err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return DiffStat{}, err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return DiffStat{}, err
	}
	status, err := wt.Status()
	if err != nil {
		return DiffStat{}, err
	}

	var d DiffStat
	for filePath, fs := range status {
		if fs.Worktree == git.Unmodified && fs.Staging == git.Unmodified {
			continue
		}
		d.Files++

		// HEAD content — empty string for new (untracked/added) files.
		var headContent string
		if f, ferr := headTree.File(filePath); ferr == nil {
			headContent, _ = f.Contents()
		}

		// Working-tree content — empty string for deleted files.
		var currentContent string
		if data, rerr := os.ReadFile(filepath.Join(worktreePath, filePath)); rerr == nil {
			currentContent = string(data)
		}

		ins, del := LinesDiff(headContent, currentContent)
		d.Insertions += ins
		d.Deletions += del
	}
	return d, nil
}

// LinesDiff returns inserted and deleted line counts between old and new using LCS.
// Exported so tests can exercise the algorithm directly.
func LinesDiff(old, newContent string) (insertions, deletions int) {
	oldLines := splitLines(old)
	newLines := splitLines(newContent)

	// Trim common prefix and suffix — reduces the LCS problem size significantly
	// for typical edits that touch a small region of a large file.
	start := 0
	for start < len(oldLines) && start < len(newLines) && oldLines[start] == newLines[start] {
		start++
	}
	end := 0
	for end < len(oldLines)-start && end < len(newLines)-start &&
		oldLines[len(oldLines)-1-end] == newLines[len(newLines)-1-end] {
		end++
	}
	trimOld := oldLines[start : len(oldLines)-end]
	trimNew := newLines[start : len(newLines)-end]

	// Avoid O(n*m) DP on very large diffs — treat all remaining lines as changed.
	const lcsMaxCells = 5_000_000
	if len(trimOld)*len(trimNew) > lcsMaxCells {
		return len(trimNew), len(trimOld)
	}
	lcs := lcsLength(trimOld, trimNew)
	return len(trimNew) - lcs, len(trimOld) - lcs
}

// lcsLength computes the length of the longest common subsequence of two line slices.
// Uses O(n*m) DP — acceptable for typical source files.
func lcsLength(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Use two rows to keep memory O(min(n,m)).
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return prev[len(b)]
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Drop the empty string that results from a trailing newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// gitCommonDir returns the path to the common git directory (the real .git dir)
// for both regular repos (where .git is a directory) and linked worktrees (where
// .git is a file pointing at the per-worktree gitdir, which contains a commondir file).
func gitCommonDir(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	data, err := os.ReadFile(gitPath)
	if err != nil {
		// .git is a directory (or missing).
		return gitPath
	}
	// .git is a file: "gitdir: /abs/path/to/.git/worktrees/<name>\n"
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return gitPath
	}
	wtGitDir := strings.TrimPrefix(line, prefix)
	// Each per-worktree gitdir contains a "commondir" file pointing to the main .git.
	if cdData, err := os.ReadFile(filepath.Join(wtGitDir, "commondir")); err == nil {
		commondir := strings.TrimSpace(string(cdData))
		if !filepath.IsAbs(commondir) {
			commondir = filepath.Join(wtGitDir, commondir)
		}
		return commondir
	}
	// Fallback: parent of the per-worktree gitdir is typically the main .git.
	return filepath.Dir(wtGitDir)
}

// openWorktree opens a git repo that may be a linked worktree (has a .git file
// rather than a .git directory).
func openWorktree(path string) (*git.Repository, error) {
	return git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
}

// resolveRef resolves a short ref name (e.g. "origin/main") to a commit hash.
func resolveRef(repo *git.Repository, name string) (plumbing.Hash, error) {
	// Try as a full or short reference name.
	for _, candidate := range []string{
		name,
		"refs/remotes/" + name,
		"refs/heads/" + name,
	} {
		if ref, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			return ref.Hash(), nil
		}
	}
	// Try as a literal hash.
	h := plumbing.NewHash(name)
	if !h.IsZero() {
		return h, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("cannot resolve ref %q", name)
}

// reachableSet returns the set of all commits reachable from start.
func reachableSet(repo *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := map[plumbing.Hash]bool{}
	iter, err := repo.Log(&git.LogOptions{From: start})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if err := iter.ForEach(func(c *object.Commit) error {
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return nil, err
	}
	return seen, nil
}

// countNotIn counts entries in a that are absent from b.
func countNotIn(a, b map[plumbing.Hash]bool) int {
	n := 0
	for h := range a {
		if !b[h] {
			n++
		}
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return strings.TrimSpace(s)
}
