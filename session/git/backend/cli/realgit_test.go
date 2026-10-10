package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

// execRunner is a test-only Runner over safeexec with a hermetic git environment.
// stdoutOnly additionally makes it a StdoutRunner (stdout separate from stderr).
type execRunner struct{ home string }

type execStdoutRunner struct{ execRunner }

// hermeticEnv drops every inherited GIT_* variable (GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, ...)
// so a test run from inside a hook or worktree cannot leak into the temp repos.
func hermeticEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, backend.RequiredGitEnv()...)
	return append(env,
		"HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com",
	)
}

func (r execRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, hermeticEnv(r.home)
	return cmd.CombinedOutput()
}

func (r execStdoutRunner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, hermeticEnv(r.home)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ee := (&exec.ExitError{}); err != nil && asExit(err, &ee) {
		ee.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

func asExit(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// mustGit runs git for test setup. One shared HOME avoids a TempDir per call.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := execRunner{home: sharedHome()}.Run(context.Background(), dir, "git", args...)
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

// Fixture repositories are built once (three git spawns) and copied per test (no spawns).
var (
	fixtureRoot    string
	fixtureRootErr error
	fixtureOnce    sync.Once
	fixtureMu      sync.Mutex
	fixtures       = map[bool]string{}
)

func sharedHome() string {
	fixtureOnce.Do(func() { fixtureRoot, fixtureRootErr = os.MkdirTemp("", "backend-cli-fixtures-") })
	if fixtureRootErr != nil {
		panic(fixtureRootErr)
	}
	return fixtureRoot
}

func TestMain(m *testing.M) {
	code := m.Run()
	if fixtureRoot != "" {
		_ = os.RemoveAll(fixtureRoot)
	}
	os.Exit(code)
}

// fixture returns the path of a prepared repository: branch main with one commit "first"
// (a.txt = "one\n") when commit is set, otherwise unborn.
func fixture(t *testing.T, commit bool) string {
	t.Helper()
	fixtureMu.Lock()
	defer fixtureMu.Unlock()
	if dir, ok := fixtures[commit]; ok {
		return dir
	}
	dir, err := os.MkdirTemp(sharedHome(), "fx-")
	require.NoError(t, err)
	dir = realPath(t, dir)
	mustGit(t, dir, "init", "-q", "-b", "main", "--template=")
	if commit {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600))
		mustGit(t, dir, "add", "-A")
		mustGit(t, dir, "commit", "-q", "-m", "first")
	}
	fixtures[commit] = dir
	return dir
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	}))
}

func newRepo(t *testing.T, commit bool) string {
	t.Helper()
	dir := realPath(t, t.TempDir())
	copyTree(t, fixture(t, commit), dir)
	return dir
}

// backends returns the CLI backend wired both ways a Runner can be supplied.
func backends(t *testing.T) map[string]backend.Backend {
	h := t.TempDir()
	return map[string]backend.Backend{
		"combined": cli.New(execRunner{home: h}),
		"stdout":   cli.New(execStdoutRunner{execRunner{home: h}}),
	}
}

// onceBackend runs fn against the combined-output runner only (the remote/SSH path). For
// operations whose output is not parsed the runner flavour cannot change the result, so the
// stdout flavour would only double the git spawns.
func onceBackend(t *testing.T, fn func(t *testing.T, b backend.Backend)) {
	requireGit(t)
	t.Parallel()
	fn(t, cli.New(execRunner{home: sharedHome()}))
}

func eachBackend(t *testing.T, fn func(t *testing.T, b backend.Backend)) {
	requireGit(t)
	t.Parallel()
	for name, b := range backends(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fn(t, b)
		})
	}
}

// Acceptance: ResolveRef(HEAD) on a repository with no commits is the typed ErrUnborn.
func TestRealGitUnborn(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		loc := backend.Local{Root: backend.RepoRoot(newRepo(t, false))}

		_, err := b.ResolveRef(ctx, loc, "HEAD")
		assert.ErrorIs(t, err, backend.ErrUnborn)

		_, err = b.CurrentBranch(ctx, loc)
		assert.ErrorIs(t, err, backend.ErrUnborn)

		ref, err := b.HeadRef(ctx, loc)
		require.NoError(t, err)
		assert.Equal(t, backend.RefName("refs/heads/main"), ref)

		_, err = b.ResolveRef(ctx, loc, "no-such-ref")
		assert.ErrorIs(t, err, backend.ErrRefNotFound)
		assert.NotErrorIs(t, err, backend.ErrUnborn)

		st, err := b.Status(ctx, loc, backend.IntentDisplay)
		require.NoError(t, err)
		assert.True(t, st.Unborn)
	})
}

func TestRealGitNotARepo(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		_, err := b.RepoRoot(context.Background(), backend.Local{Root: backend.RepoRoot(t.TempDir())})
		assert.ErrorIs(t, err, backend.ErrNotARepo)
	})
}

func TestRealGitRefsAndIdentity(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}
		head := mustGit(t, repo, "rev-parse", "HEAD")

		br, err := b.CurrentBranch(ctx, loc)
		require.NoError(t, err)
		assert.Equal(t, backend.BranchName("main"), br)

		sha, err := b.ResolveRef(ctx, loc, "HEAD")
		require.NoError(t, err)
		assert.Equal(t, backend.CommitSHA(head), sha)

		ok, err := b.RefExists(ctx, loc, "main")
		require.NoError(t, err)
		assert.True(t, ok)
		ok, err = b.RefExists(ctx, loc, "nope")
		require.NoError(t, err)
		assert.False(t, ok)

		root, err := b.RepoRoot(ctx, backend.Local{Root: backend.RepoRoot(repo)})
		require.NoError(t, err)
		assert.Equal(t, backend.RepoRoot(repo), root)
		gd, err := b.GitDir(ctx, loc)
		require.NoError(t, err)
		assert.Equal(t, backend.GitDir(filepath.Join(repo, ".git")), gd)
		cd, err := b.CommonDir(ctx, loc)
		require.NoError(t, err)
		assert.Equal(t, backend.CommonDir(filepath.Join(repo, ".git")), cd)

		refs, err := b.ListRefs(ctx, loc, backend.ListRefsRequest{Pattern: "refs/heads"})
		require.NoError(t, err)
		assert.Equal(t, []backend.RefName{"refs/heads/main"}, refs)

		mustGit(t, repo, "checkout", "-q", "-b", "feat")
		require.NoError(t, os.WriteFile(filepath.Join(repo, "b.txt"), []byte("two\n"), 0o600))
		mustGit(t, repo, "add", "-A")
		mustGit(t, repo, "commit", "-q", "-m", "second: with | pipe")

		n, err := b.CountCommits(ctx, loc, backend.RangeSpec{Exclude: "main", Include: "feat"})
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		mb, err := b.MergeBase(ctx, loc, backend.MergeBaseRequest{Left: "feat", Right: "main"})
		require.NoError(t, err)
		assert.Equal(t, backend.CommitSHA(head), mb)

		entries, err := b.Log(ctx, loc, backend.LogRequest{Limit: 1})
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "second: with | pipe", entries[0].Subject)

		_, err = b.CurrentBranch(ctx, loc)
		require.NoError(t, err)
		mustGit(t, repo, "checkout", "-q", "--detach")
		_, err = b.CurrentBranch(ctx, loc)
		assert.ErrorIs(t, err, backend.ErrDetachedHead)
		_, err = b.HeadRef(ctx, loc)
		assert.ErrorIs(t, err, backend.ErrDetachedHead)
	})
}

func TestRealGitConfigAndRemote(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}

		_, err := b.GetConfig(ctx, loc, "remote.origin.url")
		assert.ErrorIs(t, err, backend.ErrConfigUnset)

		mustGit(t, repo, "remote", "add", "origin", "https://example.com/a.git")
		require.NoError(t, b.SetRemoteURL(ctx, loc, backend.SetRemoteURLRequest{Remote: "origin", URL: "https://example.com/b.git"}))
		got, err := b.GetConfig(ctx, loc, "remote.origin.url")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/b.git", got)

		require.NoError(t, b.SetConfig(ctx, loc, backend.SetConfigRequest{Key: "ssq.test", Value: "v v"}))
		got, err = b.GetConfig(ctx, loc, "ssq.test")
		require.NoError(t, err)
		assert.Equal(t, "v v", got)
	})
}

func TestRealGitWorkingTree(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}

		dirty, err := b.IsDirty(ctx, loc, backend.IntentDestructive)
		require.NoError(t, err)
		assert.False(t, dirty)

		require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nmore\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(repo, "new file.txt"), []byte("x\n"), 0o600))

		dirty, err = b.IsDirty(ctx, loc, backend.IntentDestructive)
		require.NoError(t, err)
		assert.True(t, dirty)

		untracked, err := b.ListUntracked(ctx, loc)
		require.NoError(t, err)
		assert.Equal(t, []backend.RepoPath{"new file.txt"}, untracked)

		st, err := b.Status(ctx, loc, backend.IntentDisplay)
		require.NoError(t, err)
		assert.Equal(t, backend.BranchName("main"), st.Branch)
		require.Len(t, st.Files, 2)
		assert.Equal(t, backend.FileStatus{Path: "a.txt", Index: '.', Worktree: 'M'}, st.Files[0])
		assert.True(t, st.Files[1].Untracked)

		rows, err := b.DiffNumstat(ctx, loc, backend.DiffSpec{Base: "HEAD"})
		require.NoError(t, err)
		assert.Equal(t, []backend.NumstatRow{{Path: "a.txt", Added: 1}}, rows)

		text, err := b.Diff(ctx, loc, backend.DiffSpec{Base: "HEAD"})
		require.NoError(t, err)
		assert.Contains(t, text, "+more")

		require.NoError(t, b.Add(ctx, loc, backend.AddRequest{All: true}))
		rows, err = b.DiffNumstat(ctx, loc, backend.DiffSpec{Staged: true})
		require.NoError(t, err)
		assert.Len(t, rows, 2)

		require.NoError(t, b.Restore(ctx, loc, backend.RestoreRequest{Staged: true, Paths: []backend.RepoPath{"new file.txt"}}))
		untracked, err = b.ListUntracked(ctx, loc)
		require.NoError(t, err)
		assert.Len(t, untracked, 1)

		require.NoError(t, b.Reset(ctx, loc, backend.ResetRequest{}))
		require.NoError(t, b.Add(ctx, loc, backend.AddRequest{All: true}))
		require.NoError(t, b.Commit(ctx, loc, backend.CommitRequest{Message: "second"}))
		dirty, err = b.IsDirty(ctx, loc, backend.IntentDestructive)
		require.NoError(t, err)
		assert.False(t, dirty)

		require.NoError(t, b.Commit(ctx, loc, backend.CommitRequest{Amend: true, Message: "amended"}))
		entries, err := b.Log(ctx, loc, backend.LogRequest{Limit: 1})
		require.NoError(t, err)
		assert.Equal(t, "amended", entries[0].Subject)

		// Discard removes uncommitted edits and untracked files.
		require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("junk\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(repo, "junk.txt"), []byte("j\n"), 0o600))
		require.NoError(t, b.DiscardChanges(ctx, loc))
		dirty, err = b.IsDirty(ctx, loc, backend.IntentDestructive)
		require.NoError(t, err)
		assert.False(t, dirty)

		// Stash round trip.
		require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("stashed\n"), 0o600))
		require.NoError(t, b.StashPush(ctx, loc, backend.StashRequest{Message: "wip"}))
		dirty, _ = b.IsDirty(ctx, loc, backend.IntentDisplay)
		assert.False(t, dirty)
		require.NoError(t, b.StashPop(ctx, loc))
		dirty, _ = b.IsDirty(ctx, loc, backend.IntentDisplay)
		assert.True(t, dirty)
	})
}

func TestRealGitBranches(t *testing.T) {
	onceBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}
		head := mustGit(t, repo, "rev-parse", "HEAD")

		require.NoError(t, b.CreateBranch(ctx, loc, backend.CreateBranchRequest{Name: "topic", Base: "main"}))
		require.NoError(t, b.SwitchBranch(ctx, loc, backend.SwitchRequest{Branch: "topic"}))
		require.NoError(t, b.RenameCurrentBranch(ctx, loc, "renamed"))
		require.NoError(t, b.SwitchBranch(ctx, loc, backend.SwitchRequest{Branch: "fresh", Create: true, Base: "main"}))

		brs, err := b.ListBranches(ctx, loc, backend.ListBranchesRequest{Contains: backend.CommitSHA(head)})
		require.NoError(t, err)
		names := make([]backend.BranchName, 0, len(brs))
		for _, br := range brs {
			names = append(names, br.Name)
		}
		assert.ElementsMatch(t, []backend.BranchName{"main", "renamed", "fresh"}, names)
	})
}

func TestRealGitWorktrees(t *testing.T) {
	onceBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}
		base := realPath(t, t.TempDir())
		wt1 := filepath.Join(base, "wt1")
		wt2 := filepath.Join(base, "wt2")

		require.NoError(t, b.AddWorktree(ctx, loc, backend.AddWorktreeRequest{Path: backend.WorktreePath(wt1), Branch: "b1", Base: "main"}))
		require.NoError(t, b.CreateBranch(ctx, loc, backend.CreateBranchRequest{Name: "b2"}))
		require.NoError(t, b.AddWorktreeForExistingBranch(ctx, loc, backend.WorktreePath(wt2), "b2"))

		wts, err := b.ListWorktrees(ctx, loc)
		require.NoError(t, err)
		require.Len(t, wts, 3)
		assert.Equal(t, backend.WorktreePath(repo), wts[0].Path)
		assert.Equal(t, backend.RefName("refs/heads/b1"), wts[1].Branch)

		// In a linked worktree GitDir and CommonDir differ.
		wloc := backend.Local{Root: backend.RepoRoot(wt1)}
		gd, err := b.GitDir(ctx, wloc)
		require.NoError(t, err)
		cd, err := b.CommonDir(ctx, wloc)
		require.NoError(t, err)
		assert.Equal(t, backend.CommonDir(filepath.Join(repo, ".git")), cd)
		assert.NotEqual(t, string(cd), string(gd))

		require.NoError(t, b.RemoveWorktree(ctx, loc, backend.RemoveWorktreeRequest{Path: backend.WorktreePath(wt1), Force: true}))
		require.NoError(t, os.RemoveAll(wt2))
		require.NoError(t, b.PruneWorktrees(ctx, loc))
		wts, err = b.ListWorktrees(ctx, loc)
		require.NoError(t, err)
		assert.Len(t, wts, 1)
	})
}

func TestRealGitNetwork(t *testing.T) {
	onceBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		origin := newRepo(t, true)
		parent := realPath(t, t.TempDir())
		clone := filepath.Join(parent, "clone")

		require.NoError(t, b.Clone(ctx, backend.Local{Root: backend.RepoRoot(clone)}, backend.CloneRequest{URL: backend.RemoteURL(origin)}))
		cloneLoc := backend.Local{Root: backend.RepoRoot(clone)}
		url, err := b.GetConfig(ctx, cloneLoc, "remote.origin.url")
		require.NoError(t, err)
		assert.Equal(t, origin, url)

		// New commit upstream, then fetch it into the clone.
		require.NoError(t, os.WriteFile(filepath.Join(origin, "c.txt"), []byte("c\n"), 0o600))
		mustGit(t, origin, "add", "-A")
		mustGit(t, origin, "commit", "-q", "-m", "upstream")
		require.NoError(t, b.Fetch(ctx, cloneLoc, backend.FetchRequest{Remote: "origin", Branch: "main"}))
		require.NoError(t, b.Fetch(ctx, cloneLoc, backend.FetchRequest{All: true, Prune: true}))
		n, err := b.CountCommits(ctx, cloneLoc, backend.RangeSpec{Exclude: "HEAD", Include: "origin/main"})
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		require.NoError(t, b.Pull(ctx, cloneLoc, backend.PullRequest{Remote: "origin", Branch: "main"}))
		n, err = b.CountCommits(ctx, cloneLoc, backend.RangeSpec{Exclude: "origin/main", Include: "HEAD"})
		require.NoError(t, err)
		assert.Equal(t, 0, n)

		// Push a new branch to a bare remote.
		bare := filepath.Join(realPath(t, t.TempDir()), "bare.git")
		mustGit(t, parent, "init", "-q", "--bare", bare)
		mustGit(t, clone, "remote", "add", "bare", bare)
		require.NoError(t, b.SwitchBranch(ctx, cloneLoc, backend.SwitchRequest{Branch: "pushme", Create: true, Base: "main"}))
		require.NoError(t, b.Push(ctx, cloneLoc, backend.PushRequest{Remote: "bare", Branch: "pushme", SetUpstream: true}))
		assert.Contains(t, mustGit(t, bare, "branch", "--list", "pushme"), "pushme")
	})
}
