package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
)

// gitTemplateRepos caches one committed repo per branch name. Copying it replaces
// six git subprocesses per test (init, two configs, add, commit), whose
// fork/exec serializes on the process-wide fork lock when many parallel tests
// set up repos at once.
var gitTemplateRepos = struct {
	mu   sync.Mutex
	dirs map[string]string
}{dirs: map[string]string{}}

// initialCommitRepo makes dir a git repo on branch with one commit containing
// file.txt ("hello\n", message "initial commit"). dir must exist and be empty.
func initialCommitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, os.CopyFS(dir, os.DirFS(gitTemplateRepo(t, branch))))
}

// initConfiguredRepo makes dir an empty git repo on branch with the test
// identity configured (the init + two config calls tests used to fork for).
func initConfiguredRepo(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, os.CopyFS(dir, os.DirFS(gitTemplateRepo(t, "empty:"+branch))))
}

func gitTemplateRepo(t *testing.T, branch string) string {
	t.Helper()
	gitTemplateRepos.mu.Lock()
	defer gitTemplateRepos.mu.Unlock()
	if dir, ok := gitTemplateRepos.dirs[branch]; ok {
		return dir
	}
	dir, err := os.MkdirTemp("", "ssq-git-template-*")
	require.NoError(t, err)
	empty, isEmpty := strings.CutPrefix(branch, "empty:")
	if isEmpty {
		branch = empty
	}
	runGitOrFail(t, dir, "init", "-b", branch)
	runGitOrFail(t, dir, "config", "user.email", "test@example.com")
	runGitOrFail(t, dir, "config", "user.name", "Test")
	if isEmpty {
		gitTemplateRepos.dirs["empty:"+branch] = dir
		return dir
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644))
	runGitOrFail(t, dir, "add", "file.txt")
	runGitOrFail(t, dir, "commit", "-m", "initial commit")
	gitTemplateRepos.dirs[branch] = dir
	return dir
}

func removeGitTemplateRepos() {
	gitTemplateRepos.mu.Lock()
	defer gitTemplateRepos.mu.Unlock()
	for _, dir := range gitTemplateRepos.dirs {
		_ = os.RemoveAll(dir)
	}
}

// commitFile writes content to dir/name and commits it in-process with go-git
// (no git subprocesses). Use it on ordinary repos only: dir must not be a linked
// worktree, whose per-worktree index/HEAD go-git handles less faithfully than the CLI.
func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	repo, err := git.OpenRepo(dir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(name)
	require.NoError(t, err)
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}
	_, err = wt.Commit(msg, &gogit.CommitOptions{Author: sig, Committer: sig})
	require.NoError(t, err)
}

// headCommitSHA returns dir's HEAD commit in-process; same linked-worktree caveat as commitFile.
func headCommitSHA(t *testing.T, dir string) string {
	t.Helper()
	repo, err := git.OpenRepo(dir)
	require.NoError(t, err)
	ref, err := repo.Head()
	require.NoError(t, err)
	return ref.Hash().String()
}
