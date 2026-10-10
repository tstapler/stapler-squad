package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

const sha1 = "0123456789abcdef0123456789abcdef01234567"

func remoteAt(f backend.Runner) backend.Remote {
	return backend.Remote{Host: "h", Path: "/p", Runner: f}
}

// Acceptance: a Remote location runs through its own Runner, in its own directory, never local.
func TestCurrentBranchRemoteArgv(t *testing.T) {
	fake := newFake().on("rev-parse --abbrev-ref HEAD", "feature/x\n", nil)
	local := newFake()
	b := cli.New(local)

	got, err := b.CurrentBranch(context.Background(), remoteAt(fake))

	require.NoError(t, err)
	assert.Equal(t, backend.BranchName("feature/x"), got)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, call{Dir: "/p", Name: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}}, fake.calls[0])
	assert.Empty(t, local.calls, "a Remote location must never touch the local runner")
}

func TestLocationErrors(t *testing.T) {
	ctx := context.Background()
	local := newFake()
	b := cli.New(local)

	_, err := b.CurrentBranch(ctx, backend.Remote{Host: "h", Path: "/p"})
	assert.ErrorIs(t, err, backend.ErrNoRemoteRunner)
	assert.Empty(t, local.calls, "nil remote runner must not fall through to local")

	_, err = cli.New(nil).CurrentBranch(ctx, backend.Local{Root: "/r"})
	assert.ErrorIs(t, err, backend.ErrNoLocalRunner)

	_, err = b.CurrentBranch(ctx, backend.Local{})
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)

	_, err = b.CurrentBranch(ctx, nil)
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
}

// Runner dial errors surface as-is; nothing is retried locally.
func TestRemoteRunnerErrorPropagates(t *testing.T) {
	dial := errors.New("ssh: connect: connection refused")
	fake := newFake().on("rev-parse --abbrev-ref HEAD", "", dial)
	local := newFake()

	_, err := cli.New(local).CurrentBranch(context.Background(), remoteAt(fake))

	assert.ErrorIs(t, err, dial)
	assert.Empty(t, local.calls)
}

// argvCases lists the exact argv each operation builds (Remote at /p, so dir is "/p").
func argvCases() []struct {
	name string
	run  func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error
	want string
	out  string
} {
	type tc = struct {
		name string
		run  func(ctx context.Context, b backend.Backend, loc backend.RepoLocation) error
		want string
		out  string
	}
	return []tc{
		{"HeadRef", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.HeadRef(c, l)
			return e
		}, "symbolic-ref -q HEAD", "refs/heads/main\n"},
		{"ResolveRef", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ResolveRef(c, l, "origin/main")
			return e
		}, "rev-parse --verify origin/main^{commit}", sha1 + "\n"},
		{"RefExists", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.RefExists(c, l, "main")
			return e
		}, "rev-parse --verify --quiet main^{commit}", sha1},
		{"RepoRoot", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.RepoRoot(c, l)
			return e
		}, "rev-parse --show-toplevel", "/r\n"},
		{"GitDir", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.GitDir(c, l)
			return e
		}, "rev-parse --path-format=absolute --git-dir", "/r/.git\n"},
		{"CommonDir", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.CommonDir(c, l)
			return e
		}, "rev-parse --path-format=absolute --git-common-dir", "/r/.git\n"},
		{"ListRefs", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListRefs(c, l, backend.ListRefsRequest{Pattern: "refs/heads", Limit: 1})
			return e
		}, "for-each-ref --format=%(refname) --count=1 refs/heads", "main\n"},
		{"MergeBase", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.MergeBase(c, l, backend.MergeBaseRequest{Left: "HEAD", Right: "origin/main"})
			return e
		}, "merge-base HEAD origin/main", sha1},
		{"CountCommits", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.CountCommits(c, l, backend.RangeSpec{Exclude: "origin/HEAD", Include: "HEAD"})
			return e
		}, "rev-list --count origin/HEAD..HEAD", "3\n"},
		{"Log", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Log(c, l, backend.LogRequest{Limit: 5})
			return e
		}, "log --format=%H%x1f%s -n 5 HEAD", ""},
		{"GetConfig", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.GetConfig(c, l, "remote.origin.url")
			return e
		}, "config --get remote.origin.url", "u\n"},
		{"SetConfig", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.SetConfig(c, l, backend.SetConfigRequest{Key: "user.name", Value: "T"})
		}, "config -- user.name T", ""},
		{"SetRemoteURL", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.SetRemoteURL(c, l, backend.SetRemoteURLRequest{Remote: "origin", URL: "https://h/x.git"})
		}, "remote set-url origin https://h/x.git", ""},
		{"IsDirty", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.IsDirty(c, l, backend.IntentDisplay)
			return e
		}, "status --porcelain", ""},
		{"Status", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Status(c, l, backend.IntentDestructive)
			return e
		}, "status --porcelain=v2 --branch -z --untracked-files=all", ""},
		{"ListUntracked", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListUntracked(c, l)
			return e
		}, "ls-files --others --exclude-standard -z", ""},
		{"DiffWorktree", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(c, l, backend.DiffSpec{Base: "HEAD"})
			return e
		}, "diff --no-color --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/ HEAD", ""},
		{"DiffStagedPath", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(c, l, backend.DiffSpec{Staged: true, Paths: []backend.RepoPath{"a.go"}})
			return e
		}, "diff --no-color --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/ --cached -- a.go", ""},
		{"DiffRange", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(c, l, backend.DiffSpec{Base: sha1, Head: "feat"})
			return e
		}, "diff --no-color --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/ " + sha1 + "..feat", ""},
		{"DiffMergeBase", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.Diff(c, l, backend.DiffSpec{Base: "main", FromMergeBase: true})
			return e
		}, "diff --no-color --no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/ main...HEAD", ""},
		{"DiffNumstat", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.DiffNumstat(c, l, backend.DiffSpec{Staged: true})
			return e
		}, "diff --no-color --no-ext-diff --no-textconv --numstat -z --cached", ""},
		{"ListBranches", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListBranches(c, l, backend.ListBranchesRequest{IncludeRemote: true, Contains: sha1})
			return e
		}, "branch -a --contains " + sha1 + " --format=%(refname:lstrip=2)%1f%(objectname:short)%1f%(upstream:lstrip=2)", ""},
		{"CreateBranch", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.CreateBranch(c, l, backend.CreateBranchRequest{Name: "n", Base: "main"})
		}, "branch n main", ""},
		{"RenameCurrentBranch", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.RenameCurrentBranch(c, l, "new")
		}, "branch -m new", ""},
		{"SwitchBranchCreate", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.SwitchBranch(c, l, backend.SwitchRequest{Branch: "t", Create: true, Base: "main"})
		}, "switch -c t main", ""},
		{"SwitchBranch", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.SwitchBranch(c, l, backend.SwitchRequest{Branch: "t"})
		}, "switch t", ""},
		{"StashPush", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.StashPush(c, l, backend.StashRequest{Message: "wip"})
		}, "stash push -m wip", ""},
		{"StashPop", func(c context.Context, b backend.Backend, l backend.RepoLocation) error { return b.StashPop(c, l) }, "stash pop", ""},
		{"AddAll", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Add(c, l, backend.AddRequest{All: true})
		}, "add -A", ""},
		{"AddPaths", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Add(c, l, backend.AddRequest{Paths: []backend.RepoPath{"a", "b"}})
		}, "add -- a b", ""},
		{"RestoreStaged", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Restore(c, l, backend.RestoreRequest{Staged: true, Paths: []backend.RepoPath{"a"}})
		}, "restore --staged -- a", ""},
		{"ResetMixed", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Reset(c, l, backend.ResetRequest{})
		}, "reset --mixed HEAD", ""},
		{"ResetHardTarget", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Reset(c, l, backend.ResetRequest{Mode: backend.ResetHard, Target: "origin/main"})
		}, "reset --hard origin/main", ""},
		{"ResetSoft", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Reset(c, l, backend.ResetRequest{Mode: backend.ResetSoft})
		}, "reset --soft HEAD", ""},
		{"DeleteBranch", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.DeleteBranch(c, l, backend.DeleteBranchRequest{Name: "old"})
		}, "branch -d old", ""},
		{"DeleteBranchForce", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.DeleteBranch(c, l, backend.DeleteBranchRequest{Name: "old", Force: true})
		}, "branch -D old", ""},
		{"SetUpstream", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.SetUpstream(c, l, backend.SetUpstreamRequest{Branch: "b", Upstream: "origin/b"})
		}, "branch --set-upstream-to=origin/b b", ""},
		{"CheckoutCommit", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.CheckoutCommit(c, l, sha1)
		}, "switch --detach " + sha1, ""},
		{"ListRemote", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListRemote(c, l, backend.ListRemoteRequest{Remote: "origin", HeadsOnly: true, Pattern: "refs/heads/main"})
			return e
		}, "ls-remote --heads origin refs/heads/main", sha1 + "\trefs/heads/main\n"},
		{"RemoveFiles", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.RemoveFiles(c, l, backend.RemoveFilesRequest{Paths: []backend.RepoPath{"*.log"}, Cached: true, Recursive: true})
		}, "rm --cached -r -- *.log", ""},
		{"MoveFile", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.MoveFile(c, l, backend.MoveFileRequest{From: "a", To: "b"})
		}, "mv -- a b", ""},
		{"Commit", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Commit(c, l, backend.CommitRequest{Message: "m"})
		}, "commit -m m", ""},
		{"CommitAmendNoEdit", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Commit(c, l, backend.CommitRequest{Amend: true})
		}, "commit --amend --no-edit", ""},
		{"CommitAmendMessage", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Commit(c, l, backend.CommitRequest{Amend: true, Message: "m"})
		}, "commit --amend -m m", ""},
		{"FetchBranch", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Fetch(c, l, backend.FetchRequest{Remote: "origin", Branch: "main"})
		}, "fetch origin -- main", ""},
		{"FetchAllPrune", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Fetch(c, l, backend.FetchRequest{All: true, Prune: true})
		}, "fetch --all --prune", ""},
		{"Pull", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Pull(c, l, backend.PullRequest{})
		}, "pull", ""},
		{"PushUpstream", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Push(c, l, backend.PushRequest{Remote: "origin", Branch: "feat", SetUpstream: true})
		}, "push --set-upstream origin feat", ""},
		{"PushForce", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.Push(c, l, backend.PushRequest{Force: true})
		}, "push --force", ""},
		{"ListWorktrees", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			_, e := b.ListWorktrees(c, l)
			return e
		}, "worktree list --porcelain", ""},
		{"AddWorktree", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktree(c, l, backend.AddWorktreeRequest{Path: "/w", Branch: "b", Base: "main"})
		}, "worktree add -b b -- /w main", ""},
		{"AddWorktreeExisting", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.AddWorktreeForExistingBranch(c, l, "/w", "b")
		}, "worktree add -- /w b", ""},
		{"RemoveWorktree", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.RemoveWorktree(c, l, backend.RemoveWorktreeRequest{Path: "/w", Force: true})
		}, "worktree remove --force -- /w", ""},
		{"PruneWorktrees", func(c context.Context, b backend.Backend, l backend.RepoLocation) error {
			return b.PruneWorktrees(c, l)
		}, "worktree prune", ""},
	}
}

func TestArgvTable(t *testing.T) {
	for _, tc := range argvCases() {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFake()
			// Reply keyed on the expected argv so output parsing has something valid to read.
			fake.on(tc.want, tc.out, nil)

			err := tc.run(context.Background(), cli.New(nil), remoteAt(fake))

			require.NoError(t, err)
			require.Len(t, fake.calls, 1)
			assert.Equal(t, "/p", fake.calls[0].Dir)
			assert.Equal(t, "git "+tc.want, fake.calls[0].argv())
		})
	}
}

func TestClone(t *testing.T) {
	fake := newFake()
	err := cli.New(nil).Clone(context.Background(), remoteAt(fake), backend.CloneRequest{URL: "https://h/x.git"})
	require.NoError(t, err)
	assert.Equal(t, call{Dir: "", Name: "git", Args: []string{"clone", "--", "https://h/x.git", "/p"}}, fake.last())
}

func TestDiscardChangesSequence(t *testing.T) {
	fake := newFake().on("reset HEAD", "", errors.New("unborn"))
	require.NoError(t, cli.New(nil).DiscardChanges(context.Background(), remoteAt(fake)))
	got := make([]string, 0, len(fake.calls))
	for _, c := range fake.calls {
		got = append(got, c.argv())
	}
	assert.Equal(t, []string{"git reset HEAD", "git checkout -- .", "git clean -fd"}, got)
}

func TestInvalidArguments(t *testing.T) {
	ctx := context.Background()
	fake := newFake()
	b := cli.New(nil)
	loc := remoteAt(fake)

	_, err := b.ResolveRef(ctx, loc, "--upload-pack=evil")
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
	_, err = b.ResolveRef(ctx, loc, "")
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Add(ctx, loc, backend.AddRequest{}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Restore(ctx, loc, backend.RestoreRequest{Paths: []backend.RepoPath{"a"}}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Commit(ctx, loc, backend.CommitRequest{}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Fetch(ctx, loc, backend.FetchRequest{Branch: "main"}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Push(ctx, loc, backend.PushRequest{Branch: "main"}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.Clone(ctx, loc, backend.CloneRequest{URL: "-oProxyCommand=x"}), backend.ErrInvalidArgument)
	assert.ErrorIs(t, b.AddWorktreeForExistingBranch(ctx, loc, "/w", "-b"), backend.ErrInvalidArgument)
	assert.Empty(t, fake.calls, "invalid arguments must never reach the runner")
}

// Stderr warning/hint lines in combined output must not reach the parsers (rev-parse,
// symbolic-ref, for-each-ref), for runners without StdoutRunner.
func TestWarningNoiseTolerance(t *testing.T) {
	noise := "warning: unable to access '/home/u/.config/git/attributes': Permission denied\nhint: some advice\n"
	type tc struct {
		name string
		args string
		out  string
		run  func(b backend.Backend, loc backend.RepoLocation) (any, error)
		want any
	}
	cases := []tc{
		{"rev-parse abbrev-ref", "rev-parse --abbrev-ref HEAD", noise + "main\n",
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.CurrentBranch(context.Background(), l)
			}, backend.BranchName("main")},
		{"rev-parse abbrev-ref noise after", "rev-parse --abbrev-ref HEAD", "main\n" + noise,
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.CurrentBranch(context.Background(), l)
			}, backend.BranchName("main")},
		{"rev-parse verify", "rev-parse --verify HEAD^{commit}", "Warning: Permanently added 'h' to the list of known hosts.\n" + sha1 + "\n",
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.ResolveRef(context.Background(), l, "HEAD")
			}, backend.CommitSHA(sha1)},
		{"rev-parse show-toplevel", "rev-parse --show-toplevel", noise + "/r\n",
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.RepoRoot(context.Background(), l)
			}, backend.RepoRoot("/r")},
		{"symbolic-ref", "symbolic-ref -q HEAD", noise + "refs/heads/main\n",
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.HeadRef(context.Background(), l)
			}, backend.RefName("refs/heads/main")},
		{"for-each-ref", "for-each-ref --format=%(refname) refs/heads", noise + "a\nb\n",
			func(b backend.Backend, l backend.RepoLocation) (any, error) {
				return b.ListRefs(context.Background(), l, backend.ListRefsRequest{Pattern: "refs/heads"})
			}, []backend.RefName{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := newFake().on(c.args, c.out, nil)
			got, err := c.run(cli.New(nil), remoteAt(fake))
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestStdoutRunnerPreferred(t *testing.T) {
	fr := &fakeStdoutRunner{fakeRunner: newFake(), stdoutOut: "main\n"}
	got, err := cli.New(nil).CurrentBranch(context.Background(), remoteAt(fr))
	require.NoError(t, err)
	assert.Equal(t, backend.BranchName("main"), got)
	assert.Equal(t, 1, fr.stdoutCalls)
}

func TestCurrentBranchDetachedAndUnborn(t *testing.T) {
	ctx := context.Background()
	fake := newFake().on("rev-parse --abbrev-ref HEAD", "HEAD\n", nil)
	_, err := cli.New(nil).CurrentBranch(ctx, remoteAt(fake))
	assert.ErrorIs(t, err, backend.ErrDetachedHead)

	// rev-parse fails but symbolic-ref resolves: unborn branch, not a dropped connection.
	fake = newFake().
		on("rev-parse --abbrev-ref HEAD", "fatal: ambiguous argument 'HEAD': unknown revision\n", exitErr{128}).
		on("symbolic-ref -q HEAD", "refs/heads/main\n", nil)
	_, err = cli.New(nil).CurrentBranch(ctx, remoteAt(fake))
	assert.ErrorIs(t, err, backend.ErrUnborn)

	// Both fail: a connection problem, which must not be reported as unborn.
	fake = newFake().
		on("rev-parse --abbrev-ref HEAD", "", exitErr{255}).
		on("symbolic-ref -q HEAD", "", exitErr{255})
	_, err = cli.New(nil).CurrentBranch(ctx, remoteAt(fake))
	require.Error(t, err)
	assert.NotErrorIs(t, err, backend.ErrUnborn)
}

func TestExitStatusMapping(t *testing.T) {
	ctx := context.Background()
	loc := func(f *fakeRunner) backend.RepoLocation { return remoteAt(f) }

	exists, err := cli.New(nil).RefExists(ctx, loc(newFake().on("rev-parse --verify --quiet nope^{commit}", "", exitErr{1})), "nope")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = cli.New(nil).RefExists(ctx, loc(newFake().on("rev-parse --verify --quiet x^{commit}", "", exitErr{255})), "x")
	assert.Error(t, err)

	_, err = cli.New(nil).GetConfig(ctx, loc(newFake().on("config --get a.b", "", exitErr{1})), "a.b")
	assert.ErrorIs(t, err, backend.ErrConfigUnset)

	_, err = cli.New(nil).MergeBase(ctx, loc(newFake().on("merge-base a b", "", exitErr{1})), backend.MergeBaseRequest{Left: "a", Right: "b"})
	assert.ErrorIs(t, err, backend.ErrNoMergeBase)

	_, err = cli.New(nil).RepoRoot(ctx, loc(newFake().on("rev-parse --show-toplevel", "fatal: not a git repository (or any of the parent directories): .git\n", exitErr{128})))
	assert.ErrorIs(t, err, backend.ErrNotARepo)

	err = cli.New(nil).Add(ctx, loc(newFake().on("add -A", "fatal: Unable to create '/r/.git/index.lock': File exists.\n", exitErr{128})), backend.AddRequest{All: true})
	assert.ErrorIs(t, err, backend.ErrLocked{})
	var locked backend.ErrLocked
	require.ErrorAs(t, err, &locked)
	assert.Equal(t, "/r/.git/index.lock", locked.Path)

	_, err = cli.New(nil).Log(ctx, loc(newFake().on("log --format=%H%x1f%s HEAD", "fatal: your current branch 'main' does not have any commits yet\n", exitErr{128})), backend.LogRequest{})
	assert.ErrorIs(t, err, backend.ErrUnborn)
}

func TestCommandErrorScrubsCredentials(t *testing.T) {
	out := "fatal: unable to access 'https://user:ghp_secret@github.com/o/r.git/': 403\n"
	fake := newFake().on("fetch", out, exitErr{128})
	err := cli.New(nil).Fetch(context.Background(), remoteAt(fake), backend.FetchRequest{})
	var cerr *backend.CommandError
	require.ErrorAs(t, err, &cerr)
	assert.NotContains(t, err.Error(), "ghp_secret")
	assert.Contains(t, err.Error(), "https://***@github.com")
	assert.Equal(t, backend.OpFetch, cerr.Operation)
	var ec interface{ ExitCode() int }
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, 128, ec.ExitCode(), "runner error stays reachable")
}

func TestParseOutputs(t *testing.T) {
	ctx := context.Background()

	t.Run("status v2", func(t *testing.T) {
		z := "# branch.oid " + sha1 + "\x00# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +2 -1\x00" +
			"1 .M N... 100644 100644 100644 aaa bbb dir/mod.go\x00" +
			"2 R. N... 100644 100644 100644 aaa bbb R100 new name.go\x00old name.go\x00" +
			"u UU N... 100644 100644 100644 100644 a b c conflict.go\x00" +
			"? untracked.txt\x00! ignored.log\x00"
		fake := newFake().on("status --porcelain=v2 --branch -z --untracked-files=all", z, nil)
		st, err := cli.New(nil).Status(ctx, remoteAt(fake), backend.IntentDisplay)
		require.NoError(t, err)
		assert.Equal(t, backend.BranchName("main"), st.Branch)
		assert.Equal(t, backend.CommitSHA(sha1), st.HeadOID)
		assert.Equal(t, backend.RefName("origin/main"), st.Upstream)
		assert.Equal(t, 2, st.Ahead)
		assert.Equal(t, 1, st.Behind)
		assert.True(t, st.Dirty())
		require.Len(t, st.Files, 5)
		assert.Equal(t, backend.FileStatus{Path: "dir/mod.go", Index: '.', Worktree: 'M'}, st.Files[0])
		assert.Equal(t, backend.FileStatus{Path: "new name.go", OrigPath: "old name.go", Index: 'R', Worktree: '.'}, st.Files[1])
		assert.True(t, st.Files[2].Unmerged)
		assert.True(t, st.Files[3].Untracked)
		assert.True(t, st.Files[4].Ignored)
	})

	t.Run("status unborn and clean", func(t *testing.T) {
		fake := newFake().on("status --porcelain=v2 --branch -z --untracked-files=all", "# branch.oid (initial)\x00# branch.head main\x00! x\x00", nil)
		st, err := cli.New(nil).Status(ctx, remoteAt(fake), backend.IntentDisplay)
		require.NoError(t, err)
		assert.True(t, st.Unborn)
		assert.False(t, st.Dirty(), "ignored files do not make a repository dirty")
	})

	t.Run("numstat", func(t *testing.T) {
		z := "3\t1\ta.go\x00-\t-\timg.png\x002\t0\t\x00old.go\x00new.go\x00"
		fake := newFake().on("diff --no-color --no-ext-diff --no-textconv --numstat -z", z, nil)
		rows, err := cli.New(nil).DiffNumstat(ctx, remoteAt(fake), backend.DiffSpec{})
		require.NoError(t, err)
		assert.Equal(t, []backend.NumstatRow{
			{Path: "a.go", Added: 3, Deleted: 1},
			{Path: "img.png", Binary: true},
			{Path: "new.go", OrigPath: "old.go", Added: 2},
		}, rows)
	})

	t.Run("worktrees", func(t *testing.T) {
		text := "worktree /r\nHEAD " + sha1 + "\nbranch refs/heads/main\n\nworktree /w\nHEAD " + sha1 + "\ndetached\nlocked reason\nprunable gone\n\nworktree /bare\nbare\n"
		fake := newFake().on("worktree list --porcelain", text, nil)
		wts, err := cli.New(nil).ListWorktrees(ctx, remoteAt(fake))
		require.NoError(t, err)
		assert.Equal(t, []backend.WorktreeInfo{
			{Path: "/r", HeadSHA: sha1, Branch: "refs/heads/main"},
			{Path: "/w", HeadSHA: sha1, Detached: true, Locked: true, Prunable: true},
			{Path: "/bare", Bare: true},
		}, wts)
	})

	t.Run("log and branches", func(t *testing.T) {
		fake := newFake().
			on("log --format=%H%x1f%s HEAD", sha1+"\x1ffix: a thing\n", nil).
			on("branch --format=%(refname:lstrip=2)%1f%(objectname:short)%1f%(upstream:lstrip=2)", "main\x1fabc1234\x1forigin/main\n(HEAD detached at abc)\x1fabc\x1f\n", nil)
		entries, err := cli.New(nil).Log(ctx, remoteAt(fake), backend.LogRequest{})
		require.NoError(t, err)
		assert.Equal(t, []backend.LogEntry{{SHA: sha1, Subject: "fix: a thing"}}, entries)
		brs, err := cli.New(nil).ListBranches(ctx, remoteAt(fake), backend.ListBranchesRequest{})
		require.NoError(t, err)
		assert.Equal(t, []backend.BranchInfo{{Name: "main", ShortSHA: "abc1234", Upstream: "origin/main"}}, brs)
	})

	t.Run("garbage sha rejected", func(t *testing.T) {
		fake := newFake().on("rev-parse --verify HEAD^{commit}", "not-a-sha\n", nil)
		_, err := cli.New(nil).ResolveRef(ctx, remoteAt(fake), "HEAD")
		assert.Error(t, err)
	})

	t.Run("count", func(t *testing.T) {
		fake := newFake().on("rev-list --count HEAD", "x\n", nil)
		_, err := cli.New(nil).CountCommits(ctx, remoteAt(fake), backend.RangeSpec{})
		assert.Error(t, err)
	})
}
