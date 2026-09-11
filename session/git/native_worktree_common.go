package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// openWorktreeRepo is the funnel every native worktree/merge call site must use to open
// a worktree or repo path (plan.md's Domain Glossary: "openWorktreeRepo"). It delegates
// to OpenRepo (util.go) rather than duplicating a second
// git.PlainOpenWithOptions(EnableDotGitCommonDir: true) call: tools/lint/norawgitopen
// forbids a second raw call site in this package specifically to prevent the
// EnableDotGitCommonDir omission bug (see getHeadCommitSHA's doc comment) from
// resurfacing at a new call site, so a distinct raw implementation here would either
// trip that lint or need its own nolint carve-out for no benefit over reusing OpenRepo.
func openWorktreeRepo(path string) (*git.Repository, error) {
	return OpenRepo(path)
}

// gitdirFileLinePrefix is the on-disk prefix of a linked worktree's top-level `.git`
// redirect file (WorktreeRedirectFile in plan.md's Domain Glossary): "gitdir: <path>".
const gitdirFileLinePrefix = "gitdir: "

// resolveWorktreeIndexPath returns the on-disk path of the `index` file that applies to
// the worktree (or main working copy) at path: "<path>/.git/index" when path's `.git`
// is a real directory, or "<gitdir>/index" — the linked worktree's own
// WorktreeAdminDir, read from the `.git` redirect file's "gitdir: <path>" line —
// when it's a file. Needed because MergeMainIntoWorktree's native path may run against
// either kind of worktree (plan.md's Domain Glossary).
func resolveWorktreeIndexPath(path string) (string, error) {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("resolveWorktreeIndexPath: failed to stat %q: %w", gitPath, err)
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "index"), nil
	}
	return resolveIndexPathFromRedirectFile(gitPath)
}

// resolveIndexPathFromRedirectFile reads a linked worktree's `.git` redirect file
// (gitdirFilePath) and returns the index path inside the WorktreeAdminDir it points to.
func resolveIndexPathFromRedirectFile(gitdirFilePath string) (string, error) {
	content, err := os.ReadFile(gitdirFilePath)
	if err != nil {
		return "", fmt.Errorf("resolveWorktreeIndexPath: failed to read %q: %w", gitdirFilePath, err)
	}
	line := strings.TrimRight(string(content), "\r\n")
	if !strings.HasPrefix(line, gitdirFileLinePrefix) {
		return "", fmt.Errorf("resolveWorktreeIndexPath: %q does not contain a %q line, got %q", gitdirFilePath, gitdirFileLinePrefix, line)
	}
	adminDir := strings.TrimSpace(strings.TrimPrefix(line, gitdirFileLinePrefix))
	if adminDir == "" {
		return "", fmt.Errorf("resolveWorktreeIndexPath: %q has an empty gitdir target", gitdirFilePath)
	}
	return filepath.Join(adminDir, "index"), nil
}
