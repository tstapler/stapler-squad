package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

// S1: `git checkout .` would silently discard edits; SwitchBranch must never act on a path.
func TestRealGitSwitchBranchNeverTouchesPaths(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}
		write(t, repo, "a.txt", "edited\n")

		assert.Error(t, b.SwitchBranch(ctx, loc, backend.SwitchRequest{Branch: "."}))
		assert.Error(t, b.SwitchBranch(ctx, loc, backend.SwitchRequest{Branch: "a.txt"}))

		got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
		require.NoError(t, err)
		assert.Equal(t, "edited\n", string(got), "uncommitted edit must survive")
	})
}

// S2: a branch name that is really a refspec would delete or force-push remote refs.
func TestRefspecLookalikeBranchesRejected(t *testing.T) {
	ctx := context.Background()
	for _, name := range []backend.BranchName{":main", "+main", "main:other", "refs/heads/a:refs/heads/b"} {
		fake := newFake()
		b := cli.New(nil)
		loc := remoteAt(fake)
		assert.ErrorIs(t, b.Push(ctx, loc, backend.PushRequest{Remote: "origin", Branch: name}), backend.ErrInvalidArgument, name)
		assert.ErrorIs(t, b.Pull(ctx, loc, backend.PullRequest{Remote: "origin", Branch: name}), backend.ErrInvalidArgument, name)
		assert.ErrorIs(t, b.Fetch(ctx, loc, backend.FetchRequest{Remote: "origin", Branch: name}), backend.ErrInvalidArgument, name)
		assert.Empty(t, fake.calls, name)
	}
}

// S3: credentials must not survive scrubbing in any URL shape, including at the truncation point.
func TestScrubHidesCredentials(t *testing.T) {
	cases := map[string]struct{ in, secret string }{
		"at-in-password":     {"fatal: unable to access 'https://user:p@ss/word@github.com/x/y.git/'", "ss/word"},
		"plain":              {"fatal: https://user:tok123@github.com/x/y.git", "tok123"},
		"scp-style":          {"fatal: could not read from user:tok456@host.example:o/r.git", "tok456"},
		"token-only":         {"remote: https://ghp_tok789@github.com/x/y.git", "ghp_tok789"},
		"scp-at-in-password": {"fatal: user:pa@ss@host.example:o/r.git", "ss@"},
		// The 2000-byte cut lands 3 bytes into the password: truncating first would leave "top".
		"cut-inside-password": {strings.Repeat("x", 1983) + " https://user:topsecretpw@github.com/x/y.git", "top"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFake().on("fetch", c.in, exitErr{128})
			err := cli.New(nil).Fetch(context.Background(), remoteAt(fake), backend.FetchRequest{})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), c.secret)
		})
	}
}

// S4: a combined-output runner cannot be trusted with unmarked NUL-delimited data.
func TestCombinedRunnerNulOutputNeverSilentlyWrong(t *testing.T) {
	ctx := context.Background()
	b := cli.New(nil)

	t.Run("leading banner on ls-files", func(t *testing.T) {
		fake := newFake().on("ls-files --others --exclude-standard -z", "warning: something\nreal.txt\x00", nil)
		_, err := b.ListUntracked(ctx, remoteAt(fake))
		assert.ErrorIs(t, err, backend.ErrNoisyOutput)
	})
	t.Run("unterminated output", func(t *testing.T) {
		fake := newFake().on("ls-files --others --exclude-standard -z", "a\x00b", nil)
		_, err := b.ListUntracked(ctx, remoteAt(fake))
		assert.ErrorIs(t, err, backend.ErrNoisyOutput)
	})
	t.Run("clean output is accepted", func(t *testing.T) {
		fake := newFake().on("ls-files --others --exclude-standard -z", "a\x00b c\x00", nil)
		got, err := b.ListUntracked(ctx, remoteAt(fake))
		require.NoError(t, err)
		assert.Equal(t, []backend.RepoPath{"a", "b c"}, got)
	})
	t.Run("status skips banner before first record", func(t *testing.T) {
		fake := newFake().on("status --porcelain=v2 --branch -z --untracked-files=all",
			"hint: x\n# branch.oid (initial)\x00# branch.head main\x00? u\x00", nil)
		st, err := b.Status(ctx, remoteAt(fake), backend.IntentDisplay)
		require.NoError(t, err)
		assert.True(t, st.Dirty())
	})
	t.Run("numstat banner is a loud error", func(t *testing.T) {
		fake := newFake().on("diff --no-color --no-ext-diff --no-textconv --numstat -z", "warning: w\n3\t1\ta.go\x00", nil)
		_, err := b.DiffNumstat(ctx, remoteAt(fake), backend.DiffSpec{})
		assert.Error(t, err)
	})
}

func TestRealGitUntrackedFileNamedLikeNoise(t *testing.T) {
	requireGit(t)
	repo := newRepo(t, true)
	write(t, repo, "hint: x", "x\n")
	loc := backend.Local{Root: backend.RepoRoot(repo)}
	h := t.TempDir()

	got, err := cli.New(execStdoutRunner{execRunner{home: h}}).ListUntracked(context.Background(), loc)
	require.NoError(t, err)
	assert.Equal(t, []backend.RepoPath{"hint: x"}, got, "a StdoutRunner is trusted")

	_, err = cli.New(execRunner{home: h}).ListUntracked(context.Background(), loc)
	assert.ErrorIs(t, err, backend.ErrNoisyOutput, "ambiguous on combined output: error, not an empty list")
}

// S5: every method that takes a free-form string must refuse option lookalikes.
func TestOptionLookalikesRejectedEverywhere(t *testing.T) {
	const x = "-x"
	type tc struct {
		name string
		run  func(b backend.Backend, l backend.RepoLocation) error
	}
	ctx := context.Background()
	cases := []tc{
		{"ResolveRef", func(b backend.Backend, l backend.RepoLocation) error { _, e := b.ResolveRef(ctx, l, x); return e }},
		{"RefExists", func(b backend.Backend, l backend.RepoLocation) error { _, e := b.RefExists(ctx, l, x); return e }},
		{"ListRefs", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListRefs(ctx, l, backend.ListRefsRequest{Pattern: x})
			return e
		}},
		{"MergeBase.Left", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.MergeBase(ctx, l, backend.MergeBaseRequest{Left: x, Right: "a"})
			return e
		}},
		{"MergeBase.Right", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.MergeBase(ctx, l, backend.MergeBaseRequest{Left: "a", Right: x})
			return e
		}},
		{"CountCommits.Exclude", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.CountCommits(ctx, l, backend.RangeSpec{Exclude: x})
			return e
		}},
		{"CountCommits.Include", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.CountCommits(ctx, l, backend.RangeSpec{Include: x})
			return e
		}},
		{"Log", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Log(ctx, l, backend.LogRequest{Range: backend.RangeSpec{Include: x}})
			return e
		}},
		{"GetConfig", func(b backend.Backend, l backend.RepoLocation) error { _, e := b.GetConfig(ctx, l, x); return e }},
		{"SetConfig.Key", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SetConfig(ctx, l, backend.SetConfigRequest{Key: x, Value: "v"})
		}},
		{"SetRemoteURL.Remote", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SetRemoteURL(ctx, l, backend.SetRemoteURLRequest{Remote: x, URL: "u"})
		}},
		{"SetRemoteURL.URL", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SetRemoteURL(ctx, l, backend.SetRemoteURLRequest{Remote: "origin", URL: x})
		}},
		{"Diff.Base", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(ctx, l, backend.DiffSpec{Base: x})
			return e
		}},
		{"Diff.Head", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(ctx, l, backend.DiffSpec{Head: x})
			return e
		}},
		{"DiffNumstat.Base", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.DiffNumstat(ctx, l, backend.DiffSpec{Base: x})
			return e
		}},
		{"ListBranches.Contains", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListBranches(ctx, l, backend.ListBranchesRequest{Contains: x})
			return e
		}},
		{"CreateBranch.Name", func(b backend.Backend, l backend.RepoLocation) error {
			return b.CreateBranch(ctx, l, backend.CreateBranchRequest{Name: x})
		}},
		{"CreateBranch.Base", func(b backend.Backend, l backend.RepoLocation) error {
			return b.CreateBranch(ctx, l, backend.CreateBranchRequest{Name: "n", Base: x})
		}},
		{"RenameCurrentBranch", func(b backend.Backend, l backend.RepoLocation) error { return b.RenameCurrentBranch(ctx, l, x) }},
		{"DeleteBranch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.DeleteBranch(ctx, l, backend.DeleteBranchRequest{Name: x})
		}},
		{"SetUpstream.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SetUpstream(ctx, l, backend.SetUpstreamRequest{Branch: x, Upstream: "o/b"})
		}},
		{"SetUpstream.Upstream", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SetUpstream(ctx, l, backend.SetUpstreamRequest{Branch: "b", Upstream: x})
		}},
		{"SwitchBranch.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SwitchBranch(ctx, l, backend.SwitchRequest{Branch: x})
		}},
		{"SwitchBranch.Base", func(b backend.Backend, l backend.RepoLocation) error {
			return b.SwitchBranch(ctx, l, backend.SwitchRequest{Branch: "b", Create: true, Base: x})
		}},
		{"CheckoutCommit", func(b backend.Backend, l backend.RepoLocation) error { return b.CheckoutCommit(ctx, l, x) }},
		{"Reset.Target", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Reset(ctx, l, backend.ResetRequest{Target: x})
		}},
		{"Fetch.Remote", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Fetch(ctx, l, backend.FetchRequest{Remote: x})
		}},
		{"Fetch.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Fetch(ctx, l, backend.FetchRequest{Remote: "origin", Branch: x})
		}},
		{"Pull.Remote", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Pull(ctx, l, backend.PullRequest{Remote: x})
		}},
		{"Pull.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Pull(ctx, l, backend.PullRequest{Remote: "origin", Branch: x})
		}},
		{"Push.Remote", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Push(ctx, l, backend.PushRequest{Remote: x})
		}},
		{"Push.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Push(ctx, l, backend.PushRequest{Remote: "origin", Branch: x})
		}},
		{"Clone.URL", func(b backend.Backend, l backend.RepoLocation) error {
			return b.Clone(ctx, l, backend.CloneRequest{URL: x})
		}},
		{"ListRemote.Remote", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListRemote(ctx, l, backend.ListRemoteRequest{Remote: x})
			return e
		}},
		{"ListRemote.Pattern", func(b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListRemote(ctx, l, backend.ListRemoteRequest{Remote: "origin", Pattern: x})
			return e
		}},
		{"AddWorktree.Path", func(b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktree(ctx, l, backend.AddWorktreeRequest{Path: x, Branch: "b"})
		}},
		{"AddWorktree.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktree(ctx, l, backend.AddWorktreeRequest{Path: "/w", Branch: x})
		}},
		{"AddWorktree.Base", func(b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktree(ctx, l, backend.AddWorktreeRequest{Path: "/w", Branch: "b", Base: x})
		}},
		{"AddWorktreeForExistingBranch.Path", func(b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktreeForExistingBranch(ctx, l, x, "b")
		}},
		{"AddWorktreeForExistingBranch.Branch", func(b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktreeForExistingBranch(ctx, l, "/w", x)
		}},
		{"RemoveWorktree", func(b backend.Backend, l backend.RepoLocation) error {
			return b.RemoveWorktree(ctx, l, backend.RemoveWorktreeRequest{Path: x})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFake()
			err := c.run(cli.New(nil), remoteAt(fake))
			assert.ErrorIs(t, err, backend.ErrInvalidArgument)
			assert.Empty(t, fake.calls, "an option lookalike must never reach git")
		})
	}
}

// D1: only a git "unknown revision" message is ErrRefNotFound; transport and ownership
// failures must stay plain command errors.
func TestResolveRefDoesNotMislabelOtherFailures(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		out string
		err error
	}{
		"ssh 255":   {"ssh: connect to host h port 22: Connection refused\n", exitErr{255}},
		"ownership": {"fatal: detected dubious ownership in repository at '/r'\n", exitErr{128}},
	} {
		t.Run(name, func(t *testing.T) {
			fake := newFake().on("rev-parse --verify main^{commit}", c.out, c.err)
			_, err := cli.New(nil).ResolveRef(ctx, remoteAt(fake), "main")
			require.Error(t, err)
			assert.NotErrorIs(t, err, backend.ErrRefNotFound)
			var cerr *backend.CommandError
			assert.ErrorAs(t, err, &cerr)
		})
	}
	fake := newFake().on("rev-parse --verify main^{commit}", "fatal: Needed a single revision\n", exitErr{128})
	_, err := cli.New(nil).ResolveRef(ctx, remoteAt(fake), "main")
	assert.ErrorIs(t, err, backend.ErrRefNotFound)
}

// D2 and D3 and D5 and D4 on real repositories.
func TestRealGitUnusualNamesAndConfig(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}

		names := []string{" lead.txt", "trail ", "\nstart", "mid\nnl.txt"}
		for _, n := range names {
			write(t, repo, n, "x\n")
		}
		untracked, err := b.ListUntracked(ctx, loc)
		require.NoError(t, err)
		st, err := b.Status(ctx, loc, backend.IntentDisplay)
		require.NoError(t, err)
		fromStatus := make([]backend.RepoPath, 0, len(st.Files))
		for _, f := range st.Files {
			require.True(t, f.Untracked)
			fromStatus = append(fromStatus, f.Path)
		}
		want := make([]backend.RepoPath, len(names))
		for i, n := range names {
			want[i] = backend.RepoPath(n)
		}
		assert.ElementsMatch(t, want, untracked)
		assert.ElementsMatch(t, want, fromStatus, "ListUntracked and Status must agree")

		// D3: a tag named like a branch must not change how branches and refs are named.
		mustGit(t, repo, "branch", "feat")
		mustGit(t, repo, "tag", "feat")
		refs, err := b.ListRefs(ctx, loc, backend.ListRefsRequest{Pattern: "refs/heads"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []backend.RefName{"refs/heads/main", "refs/heads/feat"}, refs)
		brs, err := b.ListBranches(ctx, loc, backend.ListBranchesRequest{})
		require.NoError(t, err)
		got := make([]backend.BranchName, 0, len(brs))
		for _, br := range brs {
			got = append(got, br.Name)
		}
		assert.ElementsMatch(t, []backend.BranchName{"main", "feat"}, got)

		// D5: user diff config must not leak into parsed or displayed output.
		mustGit(t, repo, "config", "color.ui", "always")
		mustGit(t, repo, "config", "diff.noprefix", "true")
		mustGit(t, repo, "config", "diff.external", "false")
		write(t, repo, "a.txt", "one\nchanged\n")
		text, err := b.Diff(ctx, loc, backend.DiffSpec{Base: "HEAD", Paths: []backend.RepoPath{"a.txt"}})
		require.NoError(t, err)
		assert.NotContains(t, text, "\x1b[")
		assert.Contains(t, text, "--- a/a.txt")
		rows, err := b.DiffNumstat(ctx, loc, backend.DiffSpec{Base: "HEAD"})
		require.NoError(t, err)
		assert.Equal(t, []backend.NumstatRow{{Path: "a.txt", Added: 1}}, rows)
	})
}

// D4: an unborn HEAD is ErrUnborn and only that.
func TestRealGitUnbornIsNotRefNotFound(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		loc := backend.Local{Root: backend.RepoRoot(newRepo(t, false))}
		_, err := b.ResolveRef(context.Background(), loc, "HEAD")
		require.ErrorIs(t, err, backend.ErrUnborn)
		assert.NotErrorIs(t, err, backend.ErrRefNotFound)
		_, err = b.CurrentBranch(context.Background(), loc)
		require.ErrorIs(t, err, backend.ErrUnborn)
		assert.NotErrorIs(t, err, backend.ErrRefNotFound)
	})
}

// D7: ResolveRef is commit-only.
func TestResolveRefRejectsTreeish(t *testing.T) {
	fake := newFake()
	_, err := cli.New(nil).ResolveRef(context.Background(), remoteAt(fake), "HEAD:a.txt")
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
	assert.Empty(t, fake.calls)
}

// D8: commit with nothing to commit, and a held lock, are typed.
func TestRealGitNothingToCommitAndLocked(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}

		err := b.Commit(ctx, loc, backend.CommitRequest{Message: "empty"})
		assert.ErrorIs(t, err, backend.ErrNothingToCommit)

		lock := filepath.Join(repo, ".git", "index.lock")
		require.NoError(t, os.WriteFile(lock, nil, 0o600))
		write(t, repo, "n.txt", "n\n")
		err = b.Add(ctx, loc, backend.AddRequest{All: true})
		require.ErrorIs(t, err, backend.ErrLocked{})
		var locked backend.ErrLocked
		require.ErrorAs(t, err, &locked)
		assert.Equal(t, lock, locked.Path)
	})
}

// D9: operations added for later plan stories (Reset modes, DeleteBranch, SetUpstream,
// CheckoutCommit, ListRemote, rm, mv).
func TestRealGitAddedOperations(t *testing.T) {
	onceBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		origin := newRepo(t, true)
		parent := realPath(t, t.TempDir())
		clone := filepath.Join(parent, "c")
		cloneLoc := backend.Local{Root: backend.RepoRoot(clone)}
		require.NoError(t, b.Clone(ctx, cloneLoc, backend.CloneRequest{URL: backend.RemoteURL(origin)}))

		remote, err := b.ListRemote(ctx, cloneLoc, backend.ListRemoteRequest{Remote: "origin", HeadsOnly: true})
		require.NoError(t, err)
		require.Len(t, remote, 1)
		assert.Equal(t, backend.RefName("refs/heads/main"), remote[0].Name)
		assert.Equal(t, backend.CommitSHA(mustGit(t, origin, "rev-parse", "HEAD")), remote[0].SHA)

		// rm and mv
		require.NoError(t, b.MoveFile(ctx, cloneLoc, backend.MoveFileRequest{From: "a.txt", To: "moved.txt"}))
		_, statErr := os.Stat(filepath.Join(clone, "moved.txt"))
		require.NoError(t, statErr)
		require.NoError(t, b.RemoveFiles(ctx, cloneLoc, backend.RemoveFilesRequest{Paths: []backend.RepoPath{"*.txt"}, Force: true}))
		_, statErr = os.Stat(filepath.Join(clone, "moved.txt"))
		assert.True(t, os.IsNotExist(statErr))

		// reset modes
		require.NoError(t, b.Reset(ctx, cloneLoc, backend.ResetRequest{Mode: backend.ResetMixed}))
		dirty, err := b.IsDirty(ctx, cloneLoc, backend.IntentDisplay)
		require.NoError(t, err)
		assert.True(t, dirty)
		require.NoError(t, b.Reset(ctx, cloneLoc, backend.ResetRequest{Mode: backend.ResetHard}))
		dirty, err = b.IsDirty(ctx, cloneLoc, backend.IntentDisplay)
		require.NoError(t, err)
		assert.False(t, dirty)

		// branches: unmerged branch needs Force; upstream; detach
		require.NoError(t, b.SwitchBranch(ctx, cloneLoc, backend.SwitchRequest{Branch: "topic", Create: true}))
		write(t, clone, "t.txt", "t\n")
		require.NoError(t, b.Add(ctx, cloneLoc, backend.AddRequest{All: true}))
		require.NoError(t, b.Commit(ctx, cloneLoc, backend.CommitRequest{Message: "t"}))
		require.NoError(t, b.SetUpstream(ctx, cloneLoc, backend.SetUpstreamRequest{Branch: "topic", Upstream: "origin/main"}))
		up, err := b.GetConfig(ctx, cloneLoc, "branch.topic.merge")
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", up)

		head, err := b.ResolveRef(ctx, cloneLoc, "HEAD")
		require.NoError(t, err)
		require.NoError(t, b.CheckoutCommit(ctx, cloneLoc, head))
		_, err = b.CurrentBranch(ctx, cloneLoc)
		assert.ErrorIs(t, err, backend.ErrDetachedHead)

		require.Error(t, b.DeleteBranch(ctx, cloneLoc, backend.DeleteBranchRequest{Name: "topic"}), "unmerged")
		require.NoError(t, b.DeleteBranch(ctx, cloneLoc, backend.DeleteBranchRequest{Name: "topic", Force: true}))
		ok, err := b.RefExists(ctx, cloneLoc, "topic")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

// R1: glob and other non-ref-name "branches" must be refused, and git must push nothing.
func TestValidRefNameAllowList(t *testing.T) {
	ctx := context.Background()
	bad := []backend.BranchName{"refs/heads/*", "a*", "a?", "a[b]", "a^", "a~1", "a b", "a\\b", "a..b", "a@{u}", "@",
		"a.lock", "a/b.lock/c", "a/", "/a", "a//b", "a.", ".hidden", "a/.b", "a\x01b", ":main", "+main", "main:other"}
	for _, name := range bad {
		fake := newFake()
		err := cli.New(nil).Push(ctx, remoteAt(fake), backend.PushRequest{Remote: "origin", Branch: name})
		assert.ErrorIs(t, err, backend.ErrInvalidArgument, "%q", name)
		assert.Empty(t, fake.calls, "%q", name)
	}
	for _, name := range []backend.BranchName{"main", "feature/x-1", "refs/heads/main", "a.b", "v1.0", "user@host"} {
		fake := newFake()
		assert.NoError(t, cli.New(nil).Push(ctx, remoteAt(fake), backend.PushRequest{Remote: "origin", Branch: name}), "%q", name)
	}
}

func TestRealGitGlobPushPushesNothing(t *testing.T) {
	requireGit(t)
	b := cli.New(execRunner{home: t.TempDir()})
	repo := newRepo(t, true)
	mustGit(t, repo, "branch", "other")
	bare := filepath.Join(realPath(t, t.TempDir()), "bare.git")
	mustGit(t, repo, "init", "-q", "--bare", bare)
	mustGit(t, repo, "remote", "add", "o", bare)

	err := b.Push(context.Background(), backend.Local{Root: backend.RepoRoot(repo)}, backend.PushRequest{Remote: "o", Branch: "refs/heads/*"})

	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
	assert.Empty(t, mustGit(t, bare, "branch", "--list"), "nothing may reach the remote")
}

// R2: ResolveRef is commit-only; a tree/blob peel must not return a non-commit SHA.
func TestResolveRefRejectsNonCommitPeels(t *testing.T) {
	for _, ref := range []backend.RefName{"HEAD^{tree}", "HEAD^{blob}", "HEAD^{}", "v1^{tag}", "HEAD^{tree}^{commit}"} {
		fake := newFake()
		_, err := cli.New(nil).ResolveRef(context.Background(), remoteAt(fake), ref)
		assert.ErrorIs(t, err, backend.ErrInvalidArgument, "%q", ref)
		_, err = cli.New(nil).RefExists(context.Background(), remoteAt(fake), ref)
		assert.ErrorIs(t, err, backend.ErrInvalidArgument, "%q", ref)
		assert.Empty(t, fake.calls)
	}
	fake := newFake().on("rev-parse --verify HEAD^{commit}", sha1+"\n", nil)
	_, err := cli.New(nil).ResolveRef(context.Background(), remoteAt(fake), "HEAD^{commit}")
	assert.NoError(t, err, "an explicit ^{commit} stays valid")
}

// R3: values that start with '-' are legitimate (core.compression=-1) and must round-trip.
func TestRealGitSetConfigDashValues(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		loc := backend.Local{Root: backend.RepoRoot(newRepo(t, false))}
		for _, v := range []string{"-1", "-x", "--foo", "plain"} {
			require.NoError(t, b.SetConfig(context.Background(), loc, backend.SetConfigRequest{Key: "core.compression", Value: v}), v)
			got, err := b.GetConfig(context.Background(), loc, "core.compression")
			require.NoError(t, err)
			assert.Equal(t, v, got)
		}
	})
}

// R5b: parsers keep names exactly and reject records that do not start where they should.
func TestParsersAreStrictAboutRecordStarts(t *testing.T) {
	ctx := context.Background()
	b := cli.New(nil)

	fake := newFake().on("status --porcelain=v2 --branch -z --untracked-files=all",
		"# branch.oid (initial)\x00# branch.head main\x002 R. N... 100644 100644 100644 a b R100 \nnew\x00\nold\x00? \nuntracked\x00", nil)
	st, err := b.Status(ctx, remoteAt(fake), backend.IntentDisplay)
	require.NoError(t, err)
	require.Len(t, st.Files, 2)
	assert.Equal(t, backend.RepoPath("\nnew"), st.Files[0].Path)
	assert.Equal(t, backend.RepoPath("\nold"), st.Files[0].OrigPath)
	assert.Equal(t, backend.RepoPath("\nuntracked"), st.Files[1].Path)

	fake = newFake().on("status --porcelain=v2 --branch -z --untracked-files=all", "# branch.oid (initial)\x00\n? x\x00", nil)
	_, err = b.Status(ctx, remoteAt(fake), backend.IntentDisplay)
	assert.Error(t, err, "a record starting with a stray newline is not silently repaired")

	fake = newFake().on("diff --no-color --no-ext-diff --no-textconv --numstat -z", "1\t0\t\nname\x00", nil)
	rows, err := b.DiffNumstat(ctx, remoteAt(fake), backend.DiffSpec{})
	require.NoError(t, err)
	assert.Equal(t, []backend.NumstatRow{{Path: "\nname", Added: 1}}, rows)

	fake = newFake().on("diff --no-color --no-ext-diff --no-textconv --numstat -z", "\n1\t0\tname\x00", nil)
	_, err = b.DiffNumstat(ctx, remoteAt(fake), backend.DiffSpec{})
	assert.Error(t, err)
}

// R5d: hostile user config must not change Diff or DiffNumstat output.
func TestRealGitDiffIgnoresHostileUserConfig(t *testing.T) {
	eachBackend(t, func(t *testing.T, b backend.Backend) {
		ctx := context.Background()
		repo := newRepo(t, true)
		loc := backend.Local{Root: backend.RepoRoot(repo)}
		write(t, repo, ".gitattributes", "*.txt diff=up\n")
		for k, v := range map[string]string{
			"color.ui": "always", "diff.noprefix": "true", "diff.mnemonicPrefix": "true",
			"diff.external": "false", "diff.up.textconv": "tr a-z A-Z <",
		} {
			mustGit(t, repo, "config", k, v)
		}
		write(t, repo, "a.txt", "one\nchanged\n")

		text, err := b.Diff(ctx, loc, backend.DiffSpec{Base: "HEAD", Paths: []backend.RepoPath{"a.txt"}})
		require.NoError(t, err)
		assert.NotContains(t, text, "\x1b[", "no color escapes")
		assert.Contains(t, text, "--- a/a.txt", "standard a/ b/ prefixes")
		assert.Contains(t, text, "+changed", "textconv filter not applied")

		rows, err := b.DiffNumstat(ctx, loc, backend.DiffSpec{Base: "HEAD", Paths: []backend.RepoPath{"a.txt"}})
		require.NoError(t, err)
		assert.Equal(t, []backend.NumstatRow{{Path: "a.txt", Added: 1}}, rows)
	})
}

// R5e: an unborn HEAD found through a classified "unknown revision" failure is ErrUnborn
// only, and the command error stays reachable.
func TestUnbornDoesNotCarryRefNotFound(t *testing.T) {
	fake := newFake().
		on("rev-parse --verify HEAD^{commit}", "fatal: Needed a single revision\n", exitErr{128}).
		on("symbolic-ref -q HEAD", "refs/heads/main\n", nil)
	_, err := cli.New(nil).ResolveRef(context.Background(), remoteAt(fake), "HEAD")
	require.ErrorIs(t, err, backend.ErrUnborn)
	assert.NotErrorIs(t, err, backend.ErrRefNotFound)
	var cerr *backend.CommandError
	require.ErrorAs(t, err, &cerr)
	assert.Equal(t, backend.OpResolveRef, cerr.Operation)

	// Probe also fails: not unborn, and the original classification is kept.
	fake = newFake().on("rev-parse --verify HEAD^{commit}", "fatal: Needed a single revision\n", exitErr{128}).
		on("symbolic-ref -q HEAD", "", exitErr{1})
	_, err = cli.New(nil).ResolveRef(context.Background(), remoteAt(fake), "HEAD")
	assert.ErrorIs(t, err, backend.ErrRefNotFound)
	assert.NotErrorIs(t, err, backend.ErrUnborn)
}
