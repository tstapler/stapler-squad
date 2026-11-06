package backend_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// A linked worktree whose admin directory under <main>/.git/worktrees is gone makes git answer
// "not a git repository" for every command. The refs=shadow soak recorded the in-process backend
// falling back instead (CommonDir x4, GetConfig x1) and classified each as a real mismatch,
// which blocks the cohort flip. This runs the real CLI and the real in-process backend through a
// shadow Router and requires agreement on every refs-cohort read.
func TestShadowDanglingWorktreeGitdirAgreesWithCLI(t *testing.T) {
	gitoracle.RequireGit(t)
	home := t.TempDir()
	g := gitoracle.Git{T: t, Home: home}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(root, "main")
	if err := os.Mkdir(main, 0o700); err != nil {
		t.Fatal(err)
	}
	g.Run(main, "init", "-q", "-b", "main", "--template=")
	g.Write(main, "a.txt", "1\n")
	g.Commit(main, "first")
	wt := filepath.Join(root, "wt")
	g.Run(main, "worktree", "add", "-q", "-b", "wt", wt)
	if err := os.RemoveAll(filepath.Join(main, ".git", "worktrees", "wt")); err != nil {
		t.Fatal(err)
	}

	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	m, mp := newMetricsRig(t)
	rec := &recordingRecorder{}
	r, err := backend.NewRouter(backend.RouterConfig{
		Cohorts:   cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortRefs: backend.BackendShadow}),
		CLI:       cli.New(gitoracle.Runner{Home: home}),
		GoGit:     gogit.New(gogit.WithEnviron(func() []string { return env })),
		Preflight: noPreflight, Recorder: rec, Meter: mp.Meter("test"),
		Pinner: func(context.Context, backend.Local) backend.StatePin {
			return backend.StatePin{Head: "head0", IndexMTime: 1, Valid: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	loc := backend.Local{Root: backend.RepoRoot(wt)}
	ctx := context.Background()
	_, e1 := r.CommonDir(ctx, loc)
	_, e2 := r.GitDir(ctx, loc)
	_, e3 := r.RepoRoot(ctx, loc)
	_, e4 := r.CurrentBranch(ctx, loc)
	_, e5 := r.GetConfig(ctx, loc, "user.name")
	_, e6 := r.ResolveRef(ctx, loc, "HEAD")
	for i, e := range []error{e1, e2, e3, e4, e5, e6} {
		if e == nil {
			t.Errorf("call %d: the CLI answered without error for a repository git rejects", i)
		}
	}
	for _, op := range []backend.OperationName{backend.OpCommonDir, backend.OpGitDir, backend.OpRepoRoot, backend.OpCurrentBranch, backend.OpGetConfig, backend.OpResolveRef} {
		if got := m.count(backend.MetricShadowCallsTotal, "operation", string(op)); got != 1 {
			t.Errorf("%s: shadow_calls_total = %d, want 1 (the call must reach the shadow comparison)", op, got)
		}
		for _, class := range []backend.MismatchClass{backend.MismatchReal, backend.MismatchRacy, backend.MismatchFalseClean} {
			if got := m.count(backend.MetricShadowMismatchTotal, "operation", string(op), "class", string(class)); got != 0 {
				t.Errorf("%s: %d %s mismatch(es), want agreement with the CLI", op, got, class)
			}
		}
	}
	if recs := rec.all(); len(recs) != 0 {
		t.Errorf("records = %+v, want none", recs)
	}
}
