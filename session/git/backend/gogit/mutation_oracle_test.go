//go:build gitoracle

package gogit_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// Each mutant is the real gogit backend with one deliberate bug. The differential suite must
// notice every one; if a mutant passes, the suite has a blind spot.

type mutant struct {
	*gogit.Backend
	name          string
	currentBranch func(context.Context, backend.RepoLocation) (backend.BranchName, error)
	resolveRef    func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error)
	refExists     func(context.Context, backend.RepoLocation, backend.RefName) (bool, error)
	repoRoot      func(context.Context, backend.RepoLocation) (backend.RepoRoot, error)
	gitDir        func(context.Context, backend.RepoLocation) (backend.GitDir, error)
	listRefs      func(context.Context, backend.RepoLocation, backend.ListRefsRequest) ([]backend.RefName, error)
	mergeBase     func(context.Context, backend.RepoLocation, backend.MergeBaseRequest) (backend.CommitSHA, error)
	getConfig     func(context.Context, backend.RepoLocation, backend.ConfigKey) (string, error)
}

func (m mutant) CurrentBranch(ctx context.Context, l backend.RepoLocation) (backend.BranchName, error) {
	if m.currentBranch != nil {
		return m.currentBranch(ctx, l)
	}
	return m.Backend.CurrentBranch(ctx, l)
}

func (m mutant) ResolveRef(ctx context.Context, l backend.RepoLocation, r backend.RefName) (backend.CommitSHA, error) {
	if m.resolveRef != nil {
		return m.resolveRef(ctx, l, r)
	}
	return m.Backend.ResolveRef(ctx, l, r)
}

func (m mutant) RefExists(ctx context.Context, l backend.RepoLocation, r backend.RefName) (bool, error) {
	if m.refExists != nil {
		return m.refExists(ctx, l, r)
	}
	return m.Backend.RefExists(ctx, l, r)
}

func (m mutant) RepoRoot(ctx context.Context, l backend.RepoLocation) (backend.RepoRoot, error) {
	if m.repoRoot != nil {
		return m.repoRoot(ctx, l)
	}
	return m.Backend.RepoRoot(ctx, l)
}

func (m mutant) GitDir(ctx context.Context, l backend.RepoLocation) (backend.GitDir, error) {
	if m.gitDir != nil {
		return m.gitDir(ctx, l)
	}
	return m.Backend.GitDir(ctx, l)
}

func (m mutant) ListRefs(ctx context.Context, l backend.RepoLocation, r backend.ListRefsRequest) ([]backend.RefName, error) {
	if m.listRefs != nil {
		return m.listRefs(ctx, l, r)
	}
	return m.Backend.ListRefs(ctx, l, r)
}

func (m mutant) MergeBase(ctx context.Context, l backend.RepoLocation, r backend.MergeBaseRequest) (backend.CommitSHA, error) {
	if m.mergeBase != nil {
		return m.mergeBase(ctx, l, r)
	}
	return m.Backend.MergeBase(ctx, l, r)
}

func (m mutant) GetConfig(ctx context.Context, l backend.RepoLocation, k backend.ConfigKey) (string, error) {
	if m.getConfig != nil {
		return m.getConfig(ctx, l, k)
	}
	return m.Backend.GetConfig(ctx, l, k)
}

func fixtureNamed(t *testing.T, name string) gitoracle.Fixture {
	t.Helper()
	for _, fx := range refFixtures() {
		if fx.Name == name {
			return fx
		}
	}
	t.Fatalf("no fixture %q", name)
	return gitoracle.Fixture{}
}

func TestDifferentialSuiteCatchesBrokenBackends(t *testing.T) {
	good := newCandidate(t).(*gogit.Backend)
	mutants := []struct {
		m       mutant
		fixture string
	}{
		{mutant{name: "CurrentBranch ignores the linked worktree (main's HEAD)", currentBranch: func(ctx context.Context, l backend.RepoLocation) (backend.BranchName, error) {
			// resolve through the main worktree, the pre-EnableDotGitCommonDir bug
			root, err := good.CommonDir(ctx, l)
			if err != nil {
				return "", err
			}
			return good.CurrentBranch(ctx, backend.Local{Root: backend.RepoRoot(filepath.Dir(string(root)))})
		}}, "linked-worktrees"},
		{mutant{name: "CurrentBranch always shortens (ignores tag ambiguity)", currentBranch: func(ctx context.Context, l backend.RepoLocation) (backend.BranchName, error) {
			b, err := good.CurrentBranch(ctx, l)
			return backend.BranchName(strings.TrimPrefix(string(b), "heads/")), err
		}}, "head-names"},
		{mutant{name: "ResolveRef maps unborn HEAD to ErrRefNotFound", resolveRef: func(ctx context.Context, l backend.RepoLocation, r backend.RefName) (backend.CommitSHA, error) {
			sha, err := good.ResolveRef(ctx, l, r)
			if err != nil && strings.Contains(err.Error(), "unborn") {
				return "", backend.ErrRefNotFound
			}
			return sha, err
		}}, "unborn"},
		{mutant{name: "ResolveRef prefers branch over tag", resolveRef: func(ctx context.Context, l backend.RepoLocation, r backend.RefName) (backend.CommitSHA, error) {
			if sha, err := good.ResolveRef(ctx, l, "refs/heads/"+r); err == nil {
				return sha, nil
			}
			return good.ResolveRef(ctx, l, r)
		}}, "refs-rich"},
		{mutant{name: "ResolveRef returns a SHA for a missing object", resolveRef: func(ctx context.Context, l backend.RepoLocation, r backend.RefName) (backend.CommitSHA, error) {
			if r == "HEAD" || r == "main" {
				return backend.CommitSHA(strings.Repeat("1", 40)), nil
			}
			return good.ResolveRef(ctx, l, r)
		}}, "dangling-head"},
		{mutant{name: "RefExists says true for everything", refExists: func(context.Context, backend.RepoLocation, backend.RefName) (bool, error) { return true, nil }}, "linear"},
		{mutant{name: "GitDir returns the common dir in a linked worktree", gitDir: func(ctx context.Context, l backend.RepoLocation) (backend.GitDir, error) {
			c, err := good.CommonDir(ctx, l)
			return backend.GitDir(c), err
		}}, "linked-worktrees"},
		{mutant{name: "ListRefs ignores Limit", listRefs: func(ctx context.Context, l backend.RepoLocation, r backend.ListRefsRequest) ([]backend.RefName, error) {
			r.Limit = 0
			return good.ListRefs(ctx, l, r)
		}}, "refs-rich"},
		{mutant{name: "ListRefs drops packed refs", listRefs: func(ctx context.Context, l backend.RepoLocation, r backend.ListRefsRequest) ([]backend.RefName, error) {
			all, err := good.ListRefs(ctx, l, r)
			if len(all) > 1 {
				all = all[:len(all)-1]
			}
			return all, err
		}}, "packed-refs"},
		{mutant{name: "MergeBase returns the left side", mergeBase: func(ctx context.Context, l backend.RepoLocation, r backend.MergeBaseRequest) (backend.CommitSHA, error) {
			return good.ResolveRef(ctx, l, r.Left)
		}}, "diverged"},
		{mutant{name: "MergeBase reports no base as a ref error", mergeBase: func(ctx context.Context, l backend.RepoLocation, r backend.MergeBaseRequest) (backend.CommitSHA, error) {
			sha, err := good.MergeBase(ctx, l, r)
			if err != nil && strings.Contains(err.Error(), "no merge base") {
				return "", backend.ErrRefNotFound
			}
			return sha, err
		}}, "unrelated-histories"},
	}
	for _, c := range mutants {
		t.Run(c.m.name, func(t *testing.T) {
			t.Parallel()
			c.m.Backend = newCandidate(t).(*gogit.Backend)
			assert.NotEmpty(t, parityDiffs(t, c.m, fixtureNamed(t, c.fixture)), "the differential suite did not notice this bug")
		})
	}

	t.Run("RepoRoot is not symlink-resolved", func(t *testing.T) {
		t.Parallel()
		m := mutant{Backend: newCandidate(t).(*gogit.Backend), repoRoot: func(ctx context.Context, l backend.RepoLocation) (backend.RepoRoot, error) {
			return backend.RepoRoot(l.Dir()), nil
		}}
		assert.NotEmpty(t, rawPathDiffs(t, m))
	})
	t.Run("GetConfig returns the first value instead of the last", func(t *testing.T) {
		t.Parallel()
		broken := func(env []string) backend.Backend {
			real := gogit.New(gogit.WithEnviron(func() []string { return env }))
			return mutant{Backend: real, getConfig: func(ctx context.Context, l backend.RepoLocation, k backend.ConfigKey) (string, error) {
				v, err := real.GetConfig(ctx, l, k)
				if err == nil && k == "val.multi" {
					return "one", nil
				}
				return v, err
			}}
		}
		var found []string
		for _, sc := range configScenarios() {
			found = append(found, configDiffs(t, sc, broken)...)
		}
		require.NotEmpty(t, found)
	})
	t.Run("GetConfig ignores the global scope", func(t *testing.T) {
		t.Parallel()
		broken := func(env []string) backend.Backend {
			real := gogit.New(gogit.WithEnviron(func() []string {
				return []string{"HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
			}))
			return mutant{Backend: real}
		}
		var found []string
		for _, sc := range configScenarios() {
			found = append(found, configDiffs(t, sc, broken)...)
		}
		require.NotEmpty(t, found)
	})
}

// countingRunner counts the git processes the CLI backend starts.
type countingRunner struct {
	gitoracle.Runner
	runs *atomic.Int64
}

func (c countingRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c.runs.Add(1)
	return c.Runner.Run(ctx, dir, name, args...)
}

func (c countingRunner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c.runs.Add(1)
	return c.Runner.RunStdout(ctx, dir, name, args...)
}

// TestRouterServesRefsInProcessAndFallsBackForTheRest wires the real Router with the CLI as the
// fallback: a served read spawns no git process, an operation gogit does not serve (Log) runs on
// the CLI and returns the CLI's answer, so flipping the cohort cannot change a result.
func TestRouterServesRefsInProcessAndFallsBackForTheRest(t *testing.T) {
	gitoracle.RequireGit(t)
	home := t.TempDir()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	g := gitoracle.Git{T: t, Home: home}
	g.Run(root, "init", "-q", "-b", "main", "--template=")
	for _, name := range []string{"a.txt", "b.txt"} {
		g.Write(root, name, name)
		g.Commit(root, "add "+name)
	}
	g.Run(root, "branch", "feature", "HEAD~1")

	var spawns atomic.Int64
	cliBackend := cli.New(countingRunner{Runner: gitoracle.Runner{Home: home}, runs: &spawns}, cli.WithMeter(noop.NewMeterProvider().Meter("test")))
	env := hermeticEnviron(t)
	router, err := backend.NewRouter(backend.RouterConfig{
		Cohorts:   backend.CohortMap{}.With(backend.CohortRefs, backend.BackendGoGit),
		CLI:       cliBackend,
		GoGit:     gogit.New(gogit.WithEnviron(func() []string { return env })),
		Preflight: func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason { return nil },
		Meter:     noop.NewMeterProvider().Meter("test"),
	})
	require.NoError(t, err)
	ctx, loc := context.Background(), backend.Local{Root: backend.RepoRoot(root)}

	got, err := router.ResolveRef(ctx, loc, "feature")
	require.NoError(t, err)
	want, err := cliBackend.ResolveRef(ctx, loc, "feature")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	spawns.Store(0)
	for range 5 {
		_, err = router.ResolveRef(ctx, loc, "HEAD")
		require.NoError(t, err)
		_, err = router.CurrentBranch(ctx, loc)
		require.NoError(t, err)
		_, err = router.MergeBase(ctx, loc, backend.MergeBaseRequest{Left: "main", Right: "feature"})
		require.NoError(t, err)
		_, err = router.GetConfig(ctx, loc, "core.bare")
		require.NoError(t, err)
	}
	assert.Zero(t, spawns.Load(), "refs reads must not spawn git when the cohort is gogit")

	logs, err := router.Log(ctx, loc, backend.LogRequest{Limit: 5})
	require.NoError(t, err)
	wantLogs, err := cliBackend.Log(ctx, loc, backend.LogRequest{Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, wantLogs, logs)
	assert.Positive(t, spawns.Load(), "Log is not served by gogit, so the CLI must have run it")

	// A mutating operation gogit does not serve runs on the CLI (its cohort is cli here anyway).
	require.NoError(t, router.CreateBranch(ctx, loc, backend.CreateBranchRequest{Name: "made-by-cli", Base: "HEAD"}))
	ok, err := router.RefExists(ctx, loc, "made-by-cli")
	require.NoError(t, err)
	assert.True(t, ok, "the branch the CLI created is visible to the in-process reader")
}

// rawPathDiffs compares the paths the backend returns with the CLI's byte for byte. The oracle
// harness replaces every known path and its realpath with one token, so a missing symlink
// resolution is invisible to it; this check is the one that sees it (plan Story 2.1.1, "paths
// canonicalized").
func rawPathDiffs(t *testing.T, cand backend.Backend) []string {
	t.Helper()
	gitoracle.RequireGit(t)
	home := t.TempDir()
	g := gitoracle.Git{T: t, Home: home}
	places := fixtureNamed(t, "symlinked").Build(g, mustReal(t, t.TempDir()))
	oracle := gitoracle.NewOracle(home)
	var out []string
	for _, place := range []string{"main", "linked"} {
		loc := backend.Local{Root: backend.RepoRoot(places[place])}
		ctx := context.Background()
		type pair struct {
			name        string
			want, got   string
			wantE, gotE error
		}
		wr, werr := oracle.RepoRoot(ctx, loc)
		gr, gerr := cand.RepoRoot(ctx, loc)
		wg, wgerr := oracle.GitDir(ctx, loc)
		gg, ggerr := cand.GitDir(ctx, loc)
		wc, wcerr := oracle.CommonDir(ctx, loc)
		gc, gcerr := cand.CommonDir(ctx, loc)
		for _, p := range []pair{
			{"RepoRoot", string(wr), string(gr), werr, gerr}, {"GitDir", string(wg), string(gg), wgerr, ggerr}, {"CommonDir", string(wc), string(gc), wcerr, gcerr},
		} {
			if p.wantE != nil || p.gotE != nil || p.want != p.got {
				out = append(out, place+" "+p.name+": oracle "+p.want+" candidate "+p.got)
			}
		}
	}
	return out
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

func TestPathsAreRealpathsLikeGit(t *testing.T) {
	assert.Empty(t, rawPathDiffs(t, newCandidate(t)))
}
