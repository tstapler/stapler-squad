package svctest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// runGitOutput runs a git command in dir and returns its combined output, failing the
// test on error. Used where the test needs to inspect output (runGit, defined in
// path_completion_service_test.go, discards it).
func RunGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}

// NewDraftPRTestRepo creates a temp git repo with "main" as the initial branch and one
// commit, so GoGitVCSReader.ResolveDefaultBranch resolves it deterministically via the
// "refs/heads/main" fallback (there is no origin remote in these tests).
func NewDraftPRTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	RunGit(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test repo\n"), 0o644))
	RunGit(t, dir, "add", ".")
	RunGit(t, dir, "commit", "-q", "-m", "initial commit")
	return dir
}
