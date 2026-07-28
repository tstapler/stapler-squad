package vc

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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

// runGit executes a git command and returns the output
func (g *GitProvider) runGit(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", append([]string{"-C", g.repoRoot}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *GitProvider) GetStatus() (*VCSStatus, error) {
	status := &VCSStatus{
		Type: VCSGit,
	}

	// Get branch name
	branch, err := g.GetBranch()
	if err == nil {
		status.Branch = branch
	}

	// Get HEAD commit (short)
	if head, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
		status.HeadCommit = head
	}

	// Get last commit message
	if desc, err := g.runGit("log", "-1", "--format=%s"); err == nil {
		status.Description = desc
	}

	// Get ahead/behind info
	if branch != "" {
		if output, err := g.runGit("rev-list", "--left-right", "--count", branch+"...@{upstream}"); err == nil {
			parts := strings.Fields(output)
			if len(parts) == 2 {
				status.AheadBy, _ = strconv.Atoi(parts[0])
				status.BehindBy, _ = strconv.Atoi(parts[1])
			}
		}
		// Get upstream name
		if upstream, err := g.runGit("rev-parse", "--abbrev-ref", branch+"@{upstream}"); err == nil {
			status.Upstream = upstream
		}
	}

	// Get changed files using porcelain v2 format
	files, err := g.GetChangedFiles()
	if err != nil {
		return status, err
	}

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
	output, err := g.runGit("branch", "--show-current")
	if err != nil {
		// Might be in detached HEAD state
		if output, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
			return "(detached: " + output + ")", nil
		}
		return "", err
	}
	if output == "" {
		// Detached HEAD
		if output, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
			output = "(detached: " + output + ")"
			g.branchCache.Store(&branchCacheEntry{output, time.Now().Add(branchCacheTTL).UnixNano()})
			return output, nil
		}
	}
	g.branchCache.Store(&branchCacheEntry{output, time.Now().Add(branchCacheTTL).UnixNano()})
	return output, nil
}

// GetChangedFiles returns the current working-tree/index status as a flat
// list of FileChange entries. It orchestrates three pure steps: run the
// porcelain status + numstat git commands, parse the porcelain output
// (parsePorcelainV2Z), then merge in per-file insertion/deletion counts
// (mergeNumstat).
func (g *GitProvider) GetChangedFiles() ([]FileChange, error) {
	// Use porcelain v2 with NUL-delimited (-z) records. Without -z, git only
	// quotes paths containing "unusual" characters — a plain-ASCII filename
	// with a literal space is emitted unquoted, which makes any
	// space-splitting parse ambiguous. With -z, records are NUL-separated and
	// the path is never itself split, so paths (and rename origPaths)
	// containing spaces parse correctly. See parsePorcelainV2Z for the
	// record layout.
	output, err := g.runGit("status", "--porcelain=v2", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	// Best-effort per-file insertion/deletion counts. A numstat failure (e.g. no
	// commits yet) should not prevent the file list itself from being returned.
	var unstagedStats, stagedStats map[string]numstatEntry
	if stats, statErr := g.getNumstat(false); statErr == nil {
		unstagedStats = stats
	}
	if stats, statErr := g.getNumstat(true); statErr == nil {
		stagedStats = stats
	}

	files := parsePorcelainV2Z(output)
	return mergeNumstat(files, unstagedStats, stagedStats), nil
}

// parsePorcelainV2Z parses the NUL-delimited output of
// `git status --porcelain=v2 -z --untracked-files=all` into a list of
// FileChange entries. Additions/Deletions are left zero-valued — merging
// numstat counts in is a separate step, see mergeNumstat.
//
// Record formats (fields within a record are space-separated up to a fixed
// count; everything after that is the literal, unsplit path):
//
//	1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
//	2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>
//	u <xy> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
//	? <path>
//	! <path>
//
// With -z, records are NUL-separated instead of newline-separated. Rename
// and copy ("2 ") records are followed by one extra NUL-delimited token
// holding <origPath> — that token has no recognized record prefix of its
// own, so the loop must consume it explicitly as part of the preceding "2 "
// record rather than mistaking it for the next record.
func parsePorcelainV2Z(output string) []FileChange {
	tokens := strings.Split(output, "\x00")

	var files []FileChange
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "" {
			continue
		}

		switch {
		case strings.HasPrefix(token, "? "):
			files = append(files, FileChange{
				Path:     token[2:],
				Status:   FileUntracked,
				IsStaged: false,
			})

		case strings.HasPrefix(token, "! "):
			// Ignored files - skip

		case strings.HasPrefix(token, "1 "), strings.HasPrefix(token, "2 "):
			isRenameOrCopy := strings.HasPrefix(token, "2 ")

			// Non-path fields before the path: "1"/"2", XY, sub, mH, mI, mW,
			// hH, hI — 8 fields. Rename/copy records have one extra
			// "<X><score>" field before the path.
			fieldCount := 8
			if isRenameOrCopy {
				fieldCount = 9
			}

			// SplitN caps the number of pieces, so the final piece is
			// whatever remains after the fixed fields — the literal path,
			// spaces and all, with no further splitting.
			parts := strings.SplitN(token, " ", fieldCount+1)
			if len(parts) != fieldCount+1 {
				continue
			}

			xy := parts[1] // XY status codes
			path := parts[fieldCount]

			var oldPath string
			if isRenameOrCopy && i+1 < len(tokens) {
				oldPath = tokens[i+1]
				i++ // consume the origPath token — it is not its own record
			}

			// X = status in index (staged), Y = status in worktree (unstaged)
			indexStatus := xy[0]
			worktreeStatus := xy[1]

			// Check for conflicts (unmerged entries)
			if indexStatus == 'U' || worktreeStatus == 'U' ||
				(indexStatus == 'A' && worktreeStatus == 'A') ||
				(indexStatus == 'D' && worktreeStatus == 'D') {
				files = append(files, FileChange{
					Path:     path,
					Status:   FileConflict,
					IsStaged: false,
					OldPath:  oldPath,
				})
				continue
			}

			// Add staged change if present
			if indexStatus != '.' && indexStatus != ' ' {
				files = append(files, FileChange{
					Path:     path,
					Status:   parseGitStatusChar(indexStatus),
					IsStaged: true,
					OldPath:  oldPath,
				})
			}

			// Add unstaged change if present
			if worktreeStatus != '.' && worktreeStatus != ' ' {
				files = append(files, FileChange{
					Path:     path,
					Status:   parseGitStatusChar(worktreeStatus),
					IsStaged: false,
					OldPath:  oldPath,
				})
			}

		case strings.HasPrefix(token, "u "):
			// "u" <xy> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path> — 11
			// fields total; SplitN keeps the path unsplit even with spaces.
			parts := strings.SplitN(token, " ", 11)
			if len(parts) != 11 {
				continue
			}
			files = append(files, FileChange{
				Path:     parts[10],
				Status:   FileConflict,
				IsStaged: false,
			})
		}
	}

	return files
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
// current (post-change) path. Pass cached=true for staged changes
// (`git diff --numstat --cached`), false for unstaged (`git diff --numstat`).
// Binary files report "-" for both counts, which are treated as 0.
func (g *GitProvider) getNumstat(cached bool) (map[string]numstatEntry, error) {
	args := []string{"diff", "--numstat"}
	if cached {
		args = append(args, "--cached")
	}
	output, err := g.runGit(args...)
	if err != nil {
		return nil, err
	}

	result := make(map[string]numstatEntry)
	if output == "" {
		return result, nil
	}

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, err := strconv.Atoi(parts[0])
		if err != nil {
			added = 0 // binary files report "-"
		}
		removed, err := strconv.Atoi(parts[1])
		if err != nil {
			removed = 0
		}
		result[numstatPath(parts[2])] = numstatEntry{additions: added, deletions: removed}
	}

	return result, nil
}

// numstatPath extracts the current (post-change) path from a numstat path
// field, which may be a plain path or a rename, reported either as
// "old => new" or with a shared prefix/suffix factored out:
// "common/{old => new}/tail".
func numstatPath(raw string) string {
	idx := strings.Index(raw, "=>")
	if idx == -1 {
		return strings.TrimSpace(raw)
	}

	before, after := raw[:idx], raw[idx+2:]
	if braceStart := strings.LastIndex(before, "{"); braceStart != -1 {
		if braceEnd := strings.Index(after, "}"); braceEnd != -1 {
			prefix := before[:braceStart]
			newSegment := strings.TrimSpace(after[:braceEnd])
			suffix := after[braceEnd+1:]
			return strings.TrimSpace(prefix + newSegment + suffix)
		}
	}

	return strings.TrimSpace(after)
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

func (g *GitProvider) Stage(path string) error {
	_, err := g.runGit("add", "--", path)
	return err
}

func (g *GitProvider) StageAll() error {
	_, err := g.runGit("add", "-A")
	return err
}

func (g *GitProvider) Unstage(path string) error {
	_, err := g.runGit("restore", "--staged", "--", path)
	return err
}

func (g *GitProvider) UnstageAll() error {
	_, err := g.runGit("reset", "HEAD")
	return err
}

func (g *GitProvider) Commit(message string) error {
	_, err := g.runGit("commit", "-m", message)
	return err
}

func (g *GitProvider) AmendCommit(message string) error {
	if message == "" {
		_, err := g.runGit("commit", "--amend", "--no-edit")
		return err
	}
	_, err := g.runGit("commit", "--amend", "-m", message)
	return err
}

func (g *GitProvider) Push() error {
	_, err := g.runGit("push")
	return err
}

func (g *GitProvider) PushWithOptions(opts PushOptions) error {
	args := []string{"push"}

	if opts.Force {
		args = append(args, "--force")
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.Remote != "" {
		args = append(args, opts.Remote)
	}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	_, err := g.runGit(args...)
	return err
}

func (g *GitProvider) Pull() error {
	_, err := g.runGit("pull")
	return err
}

func (g *GitProvider) Fetch() error {
	_, err := g.runGit("fetch")
	return err
}

func (g *GitProvider) GetFileDiff(path string) (string, error) {
	// Try staged first, then unstaged
	output, err := g.runGit("diff", "--cached", "--", path)
	if err == nil && output != "" {
		return output, nil
	}
	return g.runGit("diff", "--", path)
}

func (g *GitProvider) GetDiff() (string, error) {
	// Get both staged and unstaged changes
	staged, _ := g.runGit("diff", "--cached")
	unstaged, _ := g.runGit("diff")

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
