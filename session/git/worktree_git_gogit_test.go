package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// addLinkedWorktree creates a real `git worktree add -b branchName <dir> <from>`
// linked worktree off repoDir, and configures the shared (common) git config's
// user.name/user.email — the same setup TestInitBaseCommitSHA_UsesWorktreePath_NotRepoPathAmbientCheckout
// (worktree_ops_test.go) uses. Returns the worktree's directory.
func addLinkedWorktree(t *testing.T, repoDir, branchName, from string) string {
	t.Helper()
	worktreeDir := t.TempDir()
	for _, args := range [][]string{
		{"-C", repoDir, "worktree", "add", "-b", branchName, worktreeDir, from},
	} {
		out, err := safeexec.CommandContext(context.Background(), "git", args...).CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}
	return worktreeDir
}

// TestWorktreeIsDirty_DetectsUntrackedAndModifiedFiles covers IsDirtyWithHint's
// go-git-backed dirty check (worktreeIsDirty): a fresh clean checkout must
// report clean, and both an untracked file and a modification to a tracked
// file must report dirty — matching `git status --porcelain` producing
// non-empty output.
func TestWorktreeIsDirty_DetectsUntrackedAndModifiedFiles(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	dirty, err := worktreeIsDirty(repoDir)
	require.NoError(t, err)
	assert.False(t, dirty, "freshly committed repo must report clean")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new"), 0o644))
	dirty, err = worktreeIsDirty(repoDir)
	require.NoError(t, err)
	assert.True(t, dirty, "an untracked file must report dirty")

	require.NoError(t, os.Remove(filepath.Join(repoDir, "untracked.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("changed"), 0o644))
	dirty, err = worktreeIsDirty(repoDir)
	require.NoError(t, err)
	assert.True(t, dirty, "a modified tracked file must report dirty")
}

// TestIsDirtyWithHint_DetectsRealWorktreeChanges is IsDirtyWithHint's
// end-to-end companion to the singleflight/cache-focused tests in
// worktree_git_test.go: proves the default dirtyChecker (worktreeIsDirty) is
// actually wired up against a real repo, cache invalidation included.
func TestIsDirtyWithHint_DetectsRealWorktreeChanges(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-session", "main", "")

	dirty, err := wt.IsDirty()
	require.NoError(t, err)
	assert.False(t, dirty)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("x"), 0o644))
	wt.InvalidateDirtyCache()

	dirty, err = wt.IsDirty()
	require.NoError(t, err)
	assert.True(t, dirty)
}

// TestHasStagedChanges_ExcludesUntrackedButIncludesStaged is the regression
// test for the "Untracked staging code counts as staged" bug caught while
// converting HasStagedChanges to go-git's Worktree.Status(): go-git gives a
// plain untracked file a Staging value of Untracked, not Unmodified, which is
// easy to mistake for "some kind of staged change" if the check only
// excludes Unmodified.
func TestHasStagedChanges_ExcludesUntrackedButIncludesStaged(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-session", "main", "")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new"), 0o644))
	has, err := wt.HasStagedChanges()
	require.NoError(t, err)
	assert.False(t, has, "a plain untracked file must not count as a staged change")

	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	goGitWt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = goGitWt.Add("untracked.txt")
	require.NoError(t, err)

	has, err = wt.HasStagedChanges()
	require.NoError(t, err)
	assert.True(t, has, "a newly-staged file must count as a staged change")
}

// TestStageAllExceptScaffolding_StagesDeletedFiles is the required regression
// test for AddWithOptions{All: true}'s handling of deletions — go-git's Add
// path has had rough edges around staging removed files historically, so this
// proves the conversion still stages a deletion (equivalent to `git status
// --porcelain` showing "D  <path>").
func TestStageAllExceptScaffolding_StagesDeletedFiles(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	require.NoError(t, os.Remove(filepath.Join(repoDir, "README.md")))

	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-session", "main", "")
	require.NoError(t, wt.StageAllExceptScaffolding())

	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	goGitWt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := goGitWt.Status()
	require.NoError(t, err)
	require.Contains(t, status, "README.md")
	assert.Equal(t, git.Deleted, status["README.md"].Staging, "deleting a tracked file must show up staged as Deleted")

	// Belt-and-suspenders: confirm the CLI agrees, since that's the real
	// semantics being preserved.
	porcelain := runGit(t, repoDir, "status", "--porcelain")
	assert.Contains(t, porcelain, "D  README.md")
}

// setLocalGitIdentity sets repo's local (.git/config) user.name/user.email.
func setLocalGitIdentity(t *testing.T, repo *git.Repository, name, email string) {
	t.Helper()
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = name
	cfg.User.Email = email
	require.NoError(t, repo.SetConfig(cfg))
}

// gitConfigTestEnv isolates a test from the running machine's real
// GIT_AUTHOR_*/GIT_AUTHOR_EMAIL env vars and global git config, so
// resolveCommitAuthorIdentity's precedence can be tested deterministically
// regardless of what's configured on the host running the test (this repo's
// `deterministic-fast-tests` skill). Returns the isolated $HOME.
func gitConfigTestEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

// TestResolveCommitAuthorIdentity_UsesLocalConfigOverride proves a repo's own
// local `user.name`/`user.email` (.git/config) wins even when a different
// global identity is also configured — local always wins, matching real
// git's own precedence.
func TestResolveCommitAuthorIdentity_UsesLocalConfigOverride(t *testing.T) {
	home := gitConfigTestEnv(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"),
		[]byte("[user]\n\tname = Global User\n\temail = global@example.com\n"), 0o644))

	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	setLocalGitIdentity(t, repo, "Local User", "local@example.com")

	sig, err := resolveCommitAuthorIdentity(repo)
	require.NoError(t, err)
	assert.Equal(t, "Local User", sig.Name)
	assert.Equal(t, "local@example.com", sig.Email)
}

// TestResolveCommitAuthorIdentity_FallsBackToGlobalConfig proves a repo with
// no local user.name/user.email set falls back to the global ~/.gitconfig,
// matching real git's own local -> global
// precedence chain.
func TestResolveCommitAuthorIdentity_FallsBackToGlobalConfig(t *testing.T) {
	home := gitConfigTestEnv(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"),
		[]byte("[user]\n\tname = Global User\n\temail = global@example.com\n"), 0o644))

	repoDir := setupTestRepoWithoutIdentity(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)

	sig, err := resolveCommitAuthorIdentity(repo)
	require.NoError(t, err)
	assert.Equal(t, "Global User", sig.Name)
	assert.Equal(t, "global@example.com", sig.Email)
}

// TestResolveCommitAuthorIdentity_PrefersEnvVars proves GIT_AUTHOR_NAME/
// GIT_AUTHOR_EMAIL win over both local and global config, matching real
// git's own env-var-first precedence.
func TestResolveCommitAuthorIdentity_PrefersEnvVars(t *testing.T) {
	gitConfigTestEnv(t)
	t.Setenv("GIT_AUTHOR_NAME", "Env User")
	t.Setenv("GIT_AUTHOR_EMAIL", "env@example.com")

	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	setLocalGitIdentity(t, repo, "Local User", "local@example.com")

	sig, err := resolveCommitAuthorIdentity(repo)
	require.NoError(t, err)
	assert.Equal(t, "Env User", sig.Name)
	assert.Equal(t, "env@example.com", sig.Email)
}

// TestResolveCommitAuthorIdentity_PartialEnvVarFallsBackPerField proves each
// of GIT_AUTHOR_NAME/GIT_AUTHOR_EMAIL is resolved independently, matching
// real git's own per-field ident resolution (setting one does not force the
// other to also come from the environment).
func TestResolveCommitAuthorIdentity_PartialEnvVarFallsBackPerField(t *testing.T) {
	gitConfigTestEnv(t)
	t.Setenv("GIT_AUTHOR_NAME", "Env User")

	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	setLocalGitIdentity(t, repo, "Local User", "local@example.com")

	sig, err := resolveCommitAuthorIdentity(repo)
	require.NoError(t, err)
	assert.Equal(t, "Env User", sig.Name, "GIT_AUTHOR_NAME must win for the name field")
	assert.Equal(t, "local@example.com", sig.Email, "unset GIT_AUTHOR_EMAIL must fall back to config")
}

// TestResolveCommitAuthorIdentity_ErrorsWhenNothingConfigured proves the
// helper fails loudly rather than silently falling back to a placeholder
// identity (see resolveCommitAuthorIdentity's doc comment for why a
// placeholder like createInitialCommit's is not an acceptable fallback here).
func TestResolveCommitAuthorIdentity_ErrorsWhenNothingConfigured(t *testing.T) {
	gitConfigTestEnv(t)

	repoDir := setupTestRepoWithoutIdentity(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)

	_, err = resolveCommitAuthorIdentity(repo)
	require.Error(t, err)
}

// TestStageAndCommit_CommitsWithConfiguredIdentity_InLinkedWorktree exercises
// stageAndCommit's real production shape: a `git worktree add`-created linked
// worktree (not the main repo path), matching how every real backlog work
// session invokes CommitChanges/PushChanges. Confirms the commit lands with
// the identity configured via the worktree's (shared) local git config, not
// a placeholder, and that go-git resolves the shared local config correctly
// from the worktree path — this package has documented real go-git
// worktree-path bugs before (see util.go's getHeadCommitSHA doc comment), so
// this is deliberately not just "does it work from the main repo path".
func TestStageAndCommit_CommitsWithConfiguredIdentity_InLinkedWorktree(t *testing.T) {
	gitConfigTestEnv(t)
	repoDir := setupTestRepo(t)
	worktreeDir := addLinkedWorktree(t, repoDir, "feature-x", "main")

	repo, err := OpenRepo(worktreeDir)
	require.NoError(t, err)
	setLocalGitIdentity(t, repo, "Real Author", "real@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "feature.txt"), []byte("work"), 0o644))

	wt := NewGitWorktreeFromStorage(repoDir, worktreeDir, "test-session", "feature-x", "")
	require.NoError(t, wt.CommitChanges("real work"))

	repo, err = OpenRepo(worktreeDir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)
	assert.Equal(t, "real work", commit.Message)
	assert.Equal(t, "Real Author", commit.Author.Name)
	assert.Equal(t, "real@example.com", commit.Author.Email)
}

// TestRemoteURL_ReturnsConfiguredURL proves RemoteURL's go-git conversion
// returns the same single-URL string `git remote get-url` would.
func TestRemoteURL_ReturnsConfiguredURL(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://example.com/owner/repo.git"},
	})
	require.NoError(t, err)

	url, err := RemoteURL(repoDir, "origin")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/owner/repo.git", url)
}

// TestRemoteURL_ReturnsErrorForUnknownRemote proves RemoteURL still errors
// (rather than e.g. returning an empty string) for a remote that doesn't
// exist, matching `git remote get-url`'s non-zero exit for the same case.
func TestRemoteURL_ReturnsErrorForUnknownRemote(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	_, err := RemoteURL(repoDir, "does-not-exist")
	require.Error(t, err)
}

// TestCheckoutBranch_ReturnsErrorForNonexistentBranch is CheckoutBranch's
// negative counterpart to TestCheckoutBranch_SwitchesToRealBranch (ops_test.go):
// go-git's Checkout must still fail for a branch that doesn't exist, matching
// the CLI's non-zero exit for the same case.
func TestCheckoutBranch_ReturnsErrorForNonexistentBranch(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	err := CheckoutBranch(repoDir, "does-not-exist")
	require.Error(t, err)
}
