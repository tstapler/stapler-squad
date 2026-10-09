// Package gittest holds git repository fixtures shared by the session/git and
// session/git/native test suites.
package gittest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/gitfixture"
)

// SetupTestRepo creates a temporary git repository on branch main with an initial commit
// and a configured user identity, and returns its path.
//
// Uses go-git directly rather than shelling out — see the `prefer-go-git-over-subshells`
// skill.
func SetupTestRepo(t *testing.T) string {
	t.Helper()
	return SetupTestRepoWithIdentity(t, true)
}

// SetupTestRepoWithIdentity is SetupTestRepo with the user identity optional.
func SetupTestRepoWithIdentity(t *testing.T, configureIdentity bool) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	require.NoError(t, err)
	if configureIdentity {
		gitfixture.ConfigureGoGitIdentity(t, repo)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: gitfixture.UserName, Email: gitfixture.UserEmail, When: time.Now()},
	})
	require.NoError(t, err)

	return dir
}

// RunGit runs a real git command in dir and fails the test on error.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}

// SetupBenchRepo is SetupTestRepo's *testing.B counterpart — testing.T-only helpers
// can't be called from a benchmark, so this duplicates the same go-git-only setup.
func SetupBenchRepo(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0o600); err != nil {
		b.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		b.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		b.Fatal(err)
	}
	if _, err := wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Bench User", Email: "bench@example.com", When: time.Now()},
	}); err != nil {
		b.Fatal(err)
	}
	return dir
}
