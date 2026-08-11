package unfinished

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// JJVCSReader implements VCSReader for Jujutsu (jj) repositories.
// jj uses a different model than git: there are no traditional "worktrees"
// (each checkout is a separate repo), and change tracking works differently.
// This implementation maps jj concepts onto the VCSReader interface as closely
// as possible so the scanner can surface unfinished work in jj repos.
type JJVCSReader struct{}

var _ VCSReader = (*JJVCSReader)(nil)

// ListWorktrees returns a single WorktreeInfo for a jj repo.
// jj does not have git-style linked worktrees; the repo path is the only tree.
func (j *JJVCSReader) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	out, err := j.run(repoPath, "log", "-r", "@", "--no-graph", "-T",
		`change_id.short() ++ " " ++ bookmarks`)
	if err != nil {
		return nil, fmt.Errorf("jj log: %w", err)
	}

	wt := WorktreeInfo{Path: repoPath}
	parts := strings.SplitN(strings.TrimSpace(out), " ", 2)
	if len(parts) >= 1 {
		wt.HEAD = parts[0]
	}
	if len(parts) == 2 && parts[1] != "" {
		// Use the first bookmark as the branch name.
		wt.Branch = strings.Fields(parts[1])[0]
	}
	if wt.Branch == "" {
		// jj changes without a bookmark are analogous to detached HEAD.
		wt.IsDetached = true
	}
	return []WorktreeInfo{wt}, nil
}

// ResolveDefaultBranch returns the trunk bookmark for the jj repo.
// Tries "trunk()" revset first, then falls back to common bookmark names.
func (j *JJVCSReader) ResolveDefaultBranch(repoPath string) string {
	// jj trunk() returns the configured trunk bookmark (usually main/master).
	out, err := j.run(repoPath, "log", "-r", "trunk()", "--no-graph", "-T",
		`bookmarks.map(|b| b.name()).join(" ")`)
	if err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return strings.Fields(name)[0]
		}
	}

	// Fall back to checking well-known bookmark names.
	for _, candidate := range []string{"main", "master", "develop", "trunk"} {
		_, err := j.run(repoPath, "log", "-r", candidate, "--no-graph", "-T", `""`)
		if err == nil {
			return candidate
		}
	}
	return ""
}

// HasUncommitted reports whether the working-copy change (@) has any modified files.
func (j *JJVCSReader) HasUncommitted(worktreePath string) (bool, error) {
	// Use the `empty` template property — more reliable than parsing diff output,
	// which prints "0 files changed…" rather than empty string for an empty change.
	out, err := j.run(worktreePath, "log", "-r", "@", "--no-graph", "-T", `if(empty,"e","d")`)
	if err != nil {
		return false, fmt.Errorf("jj log: %w", err)
	}
	return strings.TrimSpace(out) == "d", nil
}

// AheadBehind counts revisions between the working copy and base.
// "ahead" = commits in @:: that are not ancestors of base (exclusive of base).
// "behind" = commits in base:: that are not ancestors of @ (exclusive of @).
func (j *JJVCSReader) AheadBehind(worktreePath, base string) (int, int, error) {
	aheadRevset := fmt.Sprintf("@ :: ~::%s", base)
	behindRevset := fmt.Sprintf("%s :: ~::@", base)

	ahead, err := j.countRevisions(worktreePath, aheadRevset)
	if err != nil {
		return 0, 0, err
	}
	behind, err := j.countRevisions(worktreePath, behindRevset)
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// CommitMessages returns up to max commit descriptions from @ back to base.
func (j *JJVCSReader) CommitMessages(worktreePath, base string, max int) ([]string, error) {
	revset := fmt.Sprintf("@ :: ~::%s", base)
	out, err := j.run(worktreePath, "log", "-r", revset,
		"--no-graph", "-T", `change_id.short() ++ " " ++ description.first_line() ++ "\n"`,
		"--limit", strconv.Itoa(max))
	if err != nil {
		return nil, fmt.Errorf("jj log: %w", err)
	}
	var msgs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			msgs = append(msgs, line)
		}
	}
	return msgs, nil
}

// DiffShortstat returns file-change counts for the working-copy change (@).
func (j *JJVCSReader) DiffShortstat(worktreePath string) (DiffStat, error) {
	out, err := j.run(worktreePath, "diff", "--stat", "-r", "@")
	if err != nil {
		return DiffStat{}, fmt.Errorf("jj diff --stat: %w", err)
	}
	return parseJJDiffStat(out), nil
}

// run executes a jj command rooted at repoPath and returns trimmed stdout.
func (j *JJVCSReader) run(repoPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	all := append([]string{"--no-pager", "-R", repoPath}, args...)
	cmd := safeexec.CommandContext(ctx, "jj", all...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (j *JJVCSReader) countRevisions(repoPath, revset string) (int, error) {
	out, err := j.run(repoPath, "log", "-r", revset, "--no-graph", "-T", `"x\n"`)
	if err != nil {
		return 0, err
	}
	return strings.Count(out, "x"), nil
}

// parseJJDiffStat parses jj diff --stat output.
// Example: "src/foo.rs | 3 +-\n1 file changed, 2 insertions(+), 1 deletion(-)"
func parseJJDiffStat(out string) DiffStat {
	// jj's stat summary line looks like git's shortstat.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "file") {
			return parseDiffShortstat(strings.TrimSpace(line))
		}
	}
	return DiffStat{}
}
