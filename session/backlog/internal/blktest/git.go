package blktest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// InitialCommitRepo makes dir a git repo on branch with one commit containing
// file.txt ("hello\n", message "initial commit"). dir must exist and be empty.
func InitialCommitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, os.CopyFS(dir, os.DirFS(gitTemplateRepo(t, branch))))
}

// InitConfiguredRepo makes dir an empty git repo on branch with the test
// identity configured (the init + two config calls tests used to fork for).
func InitConfiguredRepo(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, os.CopyFS(dir, os.DirFS(gitTemplateRepo(t, "empty:"+branch))))
}

func gitTemplateRepo(t *testing.T, branch string) string {
	t.Helper()
	sessiontest.GitTemplateRepos.Mu.Lock()
	defer sessiontest.GitTemplateRepos.Mu.Unlock()
	if dir, ok := sessiontest.GitTemplateRepos.Dirs[branch]; ok {
		return dir
	}
	dir, err := os.MkdirTemp("", "ssq-git-template-*")
	require.NoError(t, err)
	empty, isEmpty := strings.CutPrefix(branch, "empty:")
	if isEmpty {
		branch = empty
	}
	sessiontest.RunGitOrFail(t, dir, "init", "-b", branch)
	sessiontest.RunGitOrFail(t, dir, "config", "user.email", "test@example.com")
	sessiontest.RunGitOrFail(t, dir, "config", "user.name", "Test")
	// Every `git commit` otherwise spawns a detached `git maintenance run --auto`
	// that briefly creates .git/objects/maintenance.lock, which a concurrent
	// os.CopyFS of this template can list and then fail to open (ENOENT).
	sessiontest.RunGitOrFail(t, dir, "config", "maintenance.auto", "false")
	sessiontest.RunGitOrFail(t, dir, "config", "gc.auto", "0")
	if isEmpty {
		sessiontest.GitTemplateRepos.Dirs["empty:"+branch] = dir
		return dir
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o644))
	sessiontest.RunGitOrFail(t, dir, "add", "file.txt")
	sessiontest.RunGitOrFail(t, dir, "commit", "-m", "initial commit")
	sessiontest.GitTemplateRepos.Dirs[branch] = dir
	return dir
}

// CommitFile writes content to dir/name and commits it in-process with go-git
// (no git subprocesses). Use it on ordinary repos only: dir must not be a linked
// worktree, whose per-worktree index/HEAD go-git handles less faithfully than the CLI.
func CommitFile(t *testing.T, dir, name, content, msg string) {
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

// HeadCommitSHA returns dir's HEAD commit in-process; same linked-worktree caveat as CommitFile.
func HeadCommitSHA(t *testing.T, dir string) string {
	t.Helper()
	repo, err := git.OpenRepo(dir)
	require.NoError(t, err)
	ref, err := repo.Head()
	require.NoError(t, err)
	return ref.Hash().String()
}
