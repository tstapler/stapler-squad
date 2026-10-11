package backend_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

type rig struct {
	t    *testing.T
	r    *backend.Router
	cli  *fakeBackend
	gg   *fakeBackend
	log  *eventLog
	m    *metricsRig
	rec  *recordingRecorder
	pins *pinScript
}

// pinScript hands out scripted pins; the default is one constant valid pin.
type pinScript struct{ next func() backend.StatePin }

func cohortsOf(modes map[backend.Cohort]backend.BackendMode) backend.CohortMap {
	var m backend.CohortMap
	for c, mode := range modes {
		m = m.With(c, mode)
	}
	return m
}

func allCohorts(mode backend.BackendMode) backend.CohortMap {
	var m backend.CohortMap
	for _, c := range backend.AllCohorts() {
		m = m.With(c, mode)
	}
	return m
}

func noPreflight(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
	return nil
}

func newRig(t *testing.T, cohorts backend.CohortMap, mutate ...func(*backend.RouterConfig)) *rig {
	t.Helper()
	log := newEventLog()
	m, _ := newMetricsRig(t)
	rec := &recordingRecorder{}
	pins := &pinScript{next: func() backend.StatePin { return backend.StatePin{Head: "head0", IndexMTime: 1, Valid: true} }}
	cli := &fakeBackend{name: "cli", log: log}
	wireCLIDefaults(cli)
	gg := strictGoGit(t, log)
	cfg := backend.RouterConfig{
		Cohorts: cohorts, CLI: cli, GoGit: gg, Preflight: noPreflight, Recorder: rec, Meter: m.meter,
		Pinner: func(context.Context, backend.Local) backend.StatePin { return pins.next() },
	}
	for _, f := range mutate {
		f(&cfg)
	}
	router, err := backend.NewRouter(cfg)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return &rig{t: t, r: router, cli: cli, gg: gg, log: log, m: m, rec: rec, pins: pins}
}

func wireCLIDefaults(f *fakeBackend) {
	f.currentBranch = func(context.Context, backend.RepoLocation) (backend.BranchName, error) { return "cli-branch", nil }
	f.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "clisha", nil
	}
	f.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return false, nil }
	f.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
		return backend.StatusResult{}, nil
	}
	f.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		return nil, nil
	}
	f.diff = func(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) { return "", nil }
	f.commit = func(context.Context, backend.RepoLocation, backend.CommitRequest) error { return nil }
	f.push = func(context.Context, backend.RepoLocation, backend.PushRequest) error { return nil }
	f.pull = func(context.Context, backend.RepoLocation, backend.PullRequest) error { return nil }
	f.listRemote = func(context.Context, backend.RepoLocation, backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
		return nil, nil
	}
	f.removeWt = func(context.Context, backend.RepoLocation, backend.RemoveWorktreeRequest) error { return nil }
}

// strictGoGit fails the test on any call that a test did not wire explicitly.
func strictGoGit(t *testing.T, log *eventLog) *fakeBackend {
	t.Helper()
	f := &fakeBackend{name: "gogit", log: log}
	bad := func(op string) { t.Errorf("unexpected gogit call: %s", op) }
	f.currentBranch = func(context.Context, backend.RepoLocation) (backend.BranchName, error) {
		bad("CurrentBranch")
		return "", nil
	}
	f.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		bad("ResolveRef")
		return "", nil
	}
	f.gitDir = func(context.Context, backend.RepoLocation) (backend.GitDir, error) { bad("GitDir"); return "", nil }
	f.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) {
		bad("IsDirty")
		return false, nil
	}
	f.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
		bad("Status")
		return backend.StatusResult{}, nil
	}
	f.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		bad("DiffNumstat")
		return nil, nil
	}
	f.diff = func(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) {
		bad("Diff")
		return "", nil
	}
	f.commit = func(context.Context, backend.RepoLocation, backend.CommitRequest) error { bad("Commit"); return nil }
	f.push = func(context.Context, backend.RepoLocation, backend.PushRequest) error { bad("Push"); return nil }
	f.pull = func(context.Context, backend.RepoLocation, backend.PullRequest) error { bad("Pull"); return nil }
	f.listRemote = func(context.Context, backend.RepoLocation, backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
		bad("ListRemote")
		return nil, nil
	}
	f.removeWt = func(context.Context, backend.RepoLocation, backend.RemoveWorktreeRequest) error {
		bad("RemoveWorktree")
		return nil
	}
	return f
}

var (
	localRepo = backend.Local{Root: "/repo"}
	ctxBG     = context.Background()
)

func (g *rig) fallbacks(op backend.OperationName, cohort backend.Cohort, reason backend.FallbackReason) int64 {
	return g.m.count(backend.MetricFallbackTotal, "operation", string(op), "cohort", cohort.String(), "reason", string(reason))
}

func TestNewRouterRequiresCLI(t *testing.T) {
	if _, err := backend.NewRouter(backend.RouterConfig{}); err == nil {
		t.Fatal("NewRouter without a CLI backend must fail")
	}
}

func TestRoutesToGoGitAndCountsObjectNotFoundFallback(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortRefs: backend.BackendGoGit}))
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrObjectNotFound
	}
	got, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD")
	if err != nil || got != "clisha" {
		t.Fatalf("ResolveRef = %q, %v; want the CLI answer", got, err)
	}
	if n := g.fallbacks(backend.OpResolveRef, backend.CohortRefs, backend.ReasonObjectNotFound); n != 1 {
		t.Fatalf("fallback_total{object_not_found} = %d, want 1", n)
	}
}

func TestGoGitAnswerReturnedWhenItSucceeds(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit))
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "ggsha", nil
	}
	got, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD")
	if err != nil || got != "ggsha" {
		t.Fatalf("ResolveRef = %q, %v", got, err)
	}
	if g.log.count("cli.ResolveRef") != 0 || g.m.totalCount(backend.MetricFallbackTotal) != 0 {
		t.Fatal("a successful in-process read must not touch the CLI or count a fallback")
	}
}

func TestCLIModeNeverTouchesGoGitAndCountsNothing(t *testing.T) {
	g := newRig(t, backend.CohortMap{})
	if _, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if g.log.count("gogit.ResolveRef") != 0 || g.m.totalCount(backend.MetricFallbackTotal) != 0 {
		t.Fatal("cli mode is the default, not a fallback")
	}
}

func TestNilGoGitRoutesEverythingToCLIWithReasonConfig(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) { c.GoGit = nil })
	if _, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if n := g.fallbacks(backend.OpResolveRef, backend.CohortRefs, backend.ReasonConfig); n != 1 {
		t.Fatalf("config fallback = %d, want 1", n)
	}
}

func TestMissingOrPanickingPreflightFailsClosed(t *testing.T) {
	cases := map[string]backend.Preflight{
		"nil":      nil,
		"panicked": func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason { panic("boom") },
	}
	for name, pf := range cases {
		t.Run(name, func(t *testing.T) {
			g := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) { c.Preflight = pf })
			if _, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD"); err != nil {
				t.Fatal(err)
			}
			if n := g.fallbacks(backend.OpResolveRef, backend.CohortRefs, backend.ReasonCapabilityDetectError); n != 1 {
				t.Fatalf("capability_detect_error = %d, want 1", n)
			}
		})
	}
}

func TestPreflightVetoRoutesToCLIBeforeGoGit(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit))
	g.r = rebuild(t, g, func(c *backend.RouterConfig) {
		c.Preflight = func(_ context.Context, _ backend.Local, op backend.OperationName) []backend.FallbackReason {
			g.log.add("preflight." + string(op))
			return []backend.FallbackReason{backend.ReasonCapabilityLFS}
		}
	})
	if _, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD"); err != nil {
		t.Fatal(err)
	}
	if n := g.fallbacks(backend.OpResolveRef, backend.CohortRefs, backend.ReasonCapabilityLFS); n != 1 {
		t.Fatalf("capability_lfs = %d, want 1", n)
	}
}

// rebuild creates a router sharing the rig's fakes and meter, with extra config applied.
func rebuild(t *testing.T, g *rig, mutate func(*backend.RouterConfig)) *backend.Router {
	t.Helper()
	cfg := backend.RouterConfig{
		Cohorts: allCohorts(backend.BackendGoGit), CLI: g.cli, GoGit: g.gg, Preflight: noPreflight,
		Recorder: g.rec, Meter: g.m.meter,
		Pinner: func(context.Context, backend.Local) backend.StatePin { return g.pins.next() },
	}
	mutate(&cfg)
	r, err := backend.NewRouter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFallbackFailureModes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		fallback bool
		reason   backend.FallbackReason
	}{
		{"torn read", backend.ErrTornRead, true, backend.ReasonTornRead},
		{"object missing", backend.ErrObjectMissing, true, backend.ReasonObjectMissing},
		{"wrapped object not found", fmt.Errorf("x: %w", backend.ErrObjectNotFound), true, backend.ReasonObjectNotFound},
		{"unclassified failure", errors.New("boom"), true, backend.ReasonError},
		{"eligible failure", backend.ErrFallbackEligible{Err: errors.New("x")}, true, backend.ReasonError},
		{"domain answer is returned", backend.ErrRefNotFound, false, ""},
		{"unborn is returned", backend.ErrUnborn, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newRig(t, allCohorts(backend.BackendGoGit))
			g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
				return "", c.err
			}
			_, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD")
			if c.fallback {
				if err != nil || g.fallbacks(backend.OpResolveRef, backend.CohortRefs, c.reason) != 1 || g.log.count("cli.ResolveRef") != 1 {
					t.Fatalf("want one CLI fallback with reason %s, got err=%v", c.reason, err)
				}
				return
			}
			if !errors.Is(err, c.err) || g.log.count("cli.ResolveRef") != 0 || g.m.totalCount(backend.MetricErrorTotal) != 0 {
				t.Fatalf("a domain answer must be returned as is, uncounted: err=%v", err)
			}
		})
	}
}

func TestCancelledContextIsNotAFallback(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit))
	ctx, cancel := context.WithCancel(ctxBG)
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		cancel()
		return "", context.Canceled
	}
	if _, err := g.r.ResolveRef(ctx, localRepo, "HEAD"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if g.log.count("cli.ResolveRef") != 0 {
		t.Fatal("a cancelled call must not be replayed on the CLI")
	}
}

// ---- remote ----

type nopRunner struct{ calls int }

func (n *nopRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	n.calls++
	return nil, nil
}

func TestRemoteAlwaysCLINeverGoGit(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit)) // every gogit method fails the test if called
	loc := backend.Remote{Host: "h", Path: "/does/not/exist/locally", Runner: &nopRunner{}}
	got, err := g.r.CurrentBranch(ctxBG, loc)
	if err != nil || got != "cli-branch" {
		t.Fatalf("CurrentBranch = %q, %v", got, err)
	}
	if n := g.fallbacks(backend.OpCurrentBranch, backend.CohortRefs, backend.ReasonRemoteHost); n != 1 {
		t.Fatalf("remote_host = %d, want 1", n)
	}
	// a shadow cohort must not shadow a remote either
	g2 := newRig(t, allCohorts(backend.BackendShadow))
	if _, err := g2.r.CurrentBranch(ctxBG, loc); err != nil || g2.log.count("gogit.CurrentBranch") != 0 {
		t.Fatalf("shadow must not reach gogit for a remote: %v", err)
	}
}

func TestRemoteNilRunnerIsTypedErrorForEveryMethod(t *testing.T) {
	var typedNil *nopRunner
	for name, loc := range map[string]backend.RepoLocation{
		"nil":       backend.Remote{Host: "h", Path: "/p"},
		"typed nil": backend.Remote{Host: "h", Path: "/p", Runner: typedNil},
	} {
		t.Run(name, func(t *testing.T) {
			g := newRig(t, allCohorts(backend.BackendGoGit))
			rv := reflect.ValueOf(g.r)
			for i := 0; i < rv.NumMethod(); i++ {
				m := rv.Method(i)
				args := zeroArgs(m.Type(), loc)
				out := m.Call(args)
				err, _ := out[len(out)-1].Interface().(error)
				if !errors.Is(err, backend.ErrNoRemoteRunner) {
					t.Errorf("%s: err = %v, want ErrNoRemoteRunner", rv.Type().Method(i).Name, err)
				}
			}
			if g.log.count("cli.CurrentBranch") != 0 {
				t.Fatal("the CLI must not be called for a remote with no runner")
			}
		})
	}
}

func zeroArgs(mt reflect.Type, loc backend.RepoLocation) []reflect.Value {
	args := make([]reflect.Value, mt.NumIn())
	for i := range args {
		switch i {
		case 0:
			args[i] = reflect.ValueOf(ctxBG)
		case 1:
			args[i] = reflect.ValueOf(&loc).Elem()
		default:
			args[i] = reflect.Zero(mt.In(i))
		}
	}
	return args
}

func TestRouterMethodsReportTheirOwnOperation(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit))
	loc := backend.Remote{Host: "h", Path: "/p", Runner: &nopRunner{}}
	rt := reflect.TypeOf((*backend.Backend)(nil)).Elem()
	if len(backend.AllOperations()) != rt.NumMethod() {
		t.Fatalf("operations (%d) != Backend methods (%d)", len(backend.AllOperations()), rt.NumMethod())
	}
	rv := reflect.ValueOf(g.r)
	for i := 0; i < rt.NumMethod(); i++ {
		name := rt.Method(i).Name
		m := rv.MethodByName(name)
		before := g.m.count(backend.MetricFallbackTotal, "operation", name, "reason", "remote_host")
		func() {
			defer func() { _ = recover() }() // the fake CLI only implements some methods; routing is counted before the call
			m.Call(zeroArgs(m.Type(), loc))
		}()
		if after := g.m.count(backend.MetricFallbackTotal, "operation", name, "reason", "remote_host"); after != before+1 {
			t.Errorf("Router.%s did not route as operation %q", name, name)
		}
	}
}

func TestEveryOperationHasACohort(t *testing.T) {
	for _, op := range backend.AllOperations() {
		if !backend.HasCohort(op) {
			t.Errorf("operation %s has no cohort", op)
		}
	}
}

// ---- write scope ----

func TestWriteFallsBackOnlyWhenNothingWasWritten(t *testing.T) {
	cases := []struct {
		name       string
		gogitErr   error
		markWrote  bool
		wantCLI    bool
		wantWrote  bool
		wantErrCnt int64
	}{
		{"eligible and unwritten", backend.ErrFallbackEligible{Wrote: false, Err: errors.New("x")}, false, true, false, 0},
		{"eligible but wrote", backend.ErrFallbackEligible{Wrote: true, Err: errors.New("x")}, false, false, true, 1},
		{"plain error on a write", errors.New("boom"), false, false, false, 1},
		{"plain object-not-found on a write", backend.ErrObjectNotFound, false, false, false, 1},
		{"second scope wrote, error says not", backend.ErrFallbackEligible{Wrote: false, Err: errors.New("x")}, true, false, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit}), optedIn(false))
			g.gg.commit = func(ctx context.Context, _ backend.RepoLocation, _ backend.CommitRequest) error {
				g.log.add("gogit.commit.start")
				if c.markWrote {
					backend.CallStateFrom(ctx).MarkWrote()
				}
				return c.gogitErr
			}
			err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{})
			if c.wantCLI {
				if err != nil || g.log.count("cli.Commit") != 1 || g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonError) != 1 {
					t.Fatalf("want one CLI replay, err=%v", err)
				}
				return
			}
			if g.log.count("cli.Commit") != 0 {
				t.Fatal("a write that may have applied must never be replayed on the CLI")
			}
			var fe backend.ErrFallbackEligible
			if c.wantWrote {
				if !errors.As(err, &fe) || !fe.Wrote {
					t.Fatalf("err = %v, want ErrFallbackEligible{Wrote:true}", err)
				}
			} else if err == nil || errors.As(err, &fe) && fe.Wrote {
				t.Fatalf("err = %v", err)
			}
			if n := g.m.count(backend.MetricErrorTotal, "operation", "Commit", "implementation", "gogit"); n != c.wantErrCnt {
				t.Fatalf("git_backend_error_total = %d, want %d", n, c.wantErrCnt)
			}
		})
	}
}

// optedIn lists /repo for localwrite and reports no live session.
func optedIn(live bool) func(*backend.RouterConfig) {
	return func(c *backend.RouterConfig) {
		c.LocalWriteRepos = []backend.RepoRoot{"/repo"}
		c.LiveSessions = func(string) bool { return live }
	}
}

func TestPreflightRunsBeforeTheGoGitCall(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit}), optedIn(false))
	g.r = rebuild(t, g, func(c *backend.RouterConfig) {
		optedIn(false)(c)
		c.Cohorts = cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit})
		c.Preflight = func(_ context.Context, _ backend.Local, op backend.OperationName) []backend.FallbackReason {
			g.log.add("preflight." + string(op))
			return nil
		}
	})
	g.gg.commit = func(context.Context, backend.RepoLocation, backend.CommitRequest) error { return nil }
	if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
		t.Fatal(err)
	}
	ev := g.log.snapshot()
	if len(ev) != 2 || ev[0] != "preflight.Commit" || ev[1] != "gogit.Commit" {
		t.Fatalf("event order = %v, want preflight then gogit", ev)
	}
}

// ---- localwrite opt-in and live session ----

func TestLocalWriteRequiresOptInAndNoLiveSession(t *testing.T) {
	localwrite := cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit})

	t.Run("unlisted repo goes to the CLI with reason config", func(t *testing.T) {
		g := newRig(t, localwrite, func(c *backend.RouterConfig) {
			c.LocalWriteRepos = []backend.RepoRoot{"/other"}
			c.LiveSessions = func(string) bool { t.Error("probe must not run for an unlisted repo"); return false }
		})
		if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonConfig) != 1 {
			t.Fatal("want reason config")
		}
	})
	t.Run("subdirectory of a listed repo is not a match", func(t *testing.T) {
		g := newRig(t, localwrite, optedIn(false))
		if err := g.r.Commit(ctxBG, backend.Local{Root: "/repo/sub"}, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonConfig) != 1 {
			t.Fatal("want reason config")
		}
	})
	t.Run("live session goes to the CLI", func(t *testing.T) {
		g := newRig(t, localwrite, optedIn(true))
		if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonLiveSession) != 1 {
			t.Fatal("want reason live_session")
		}
	})
	t.Run("missing probe assumes a live session", func(t *testing.T) {
		g := newRig(t, localwrite, func(c *backend.RouterConfig) { c.LocalWriteRepos = []backend.RepoRoot{"/repo"} })
		if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonLiveSession) != 1 {
			t.Fatal("want reason live_session")
		}
	})
	t.Run("listed and idle runs in process, probe evaluated per call", func(t *testing.T) {
		probes, live := 0, false
		g := newRig(t, localwrite, func(c *backend.RouterConfig) {
			c.LocalWriteRepos = []backend.RepoRoot{"/repo"}
			c.LiveSessions = func(root string) bool {
				probes++
				if root != "/repo" {
					t.Errorf("probe root = %q", root)
				}
				return live
			}
		})
		g.gg.commit = func(context.Context, backend.RepoLocation, backend.CommitRequest) error { return nil }
		if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if g.log.count("gogit.Commit") != 1 || g.log.count("cli.Commit") != 0 {
			t.Fatal("want in-process commit")
		}
		live = true // a session starts between calls: the next call must see it
		if err := g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}); err != nil {
			t.Fatal(err)
		}
		if probes != 2 || g.log.count("gogit.Commit") != 1 || g.log.count("cli.Commit") != 1 {
			t.Fatalf("probes=%d, gogit=%d, cli=%d: probe must run per call", probes, g.log.count("gogit.Commit"), g.log.count("cli.Commit"))
		}
	})
}

// ---- reason precedence ----

func TestFirstReasonFollowsPlanPrecedenceForEveryAdjacentPair(t *testing.T) {
	order := backend.ReasonPrecedence()
	want := []backend.FallbackReason{
		"remote_host", "config", "agent_tool", "capability_detect_error", "capability_local_transport",
		"capability_hooks", "capability_gpgsign", "capability_lfs", "capability_unsupported_index",
		"capability_sparse", "capability_shallow", "capability_submodule", "capability_ssh_proxy",
		"live_session", "unsafe_worktree_write", "unsupported_pull_mode", "lock_unavailable",
		"torn_read", "object_missing", "object_not_found", "destructive_confirm", "error",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("precedence = %v\nwant %v", order, want)
	}
	for i := 0; i+1 < len(order); i++ {
		if got := backend.FirstReason(order[i+1], order[i]); got != order[i] {
			t.Errorf("FirstReason(%s, %s) = %s, want %s", order[i+1], order[i], got, order[i])
		}
	}
	if backend.FirstReason() != "" {
		t.Error("no reasons must give no winner")
	}
	if got := backend.FirstReason("made_up", backend.ReasonLockUnavailable); got != backend.ReasonLockUnavailable {
		t.Errorf("unknown reason ranks as error, got %s", got)
	}
	if got := backend.FirstReason("made_up"); got != backend.ReasonError {
		t.Errorf("a lone unknown reason must still route to the CLI as error, got %s", got)
	}
}

func TestHookedRepoRemoveReportsCapabilityHooksBeforeUnsafeWrite(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) {
		c.Preflight = func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
			return []backend.FallbackReason{backend.ReasonCapabilityHooks}
		}
	})
	if err := g.r.RemoveWorktree(ctxBG, localRepo, backend.RemoveWorktreeRequest{}); err != nil {
		t.Fatal(err)
	}
	if g.fallbacks(backend.OpRemoveWorktree, backend.CohortWorktree, backend.ReasonCapabilityHooks) != 1 ||
		g.fallbacks(backend.OpRemoveWorktree, backend.CohortWorktree, backend.ReasonUnsafeWorktreeWrite) != 0 {
		t.Fatal("capability_hooks must win over unsafe_worktree_write")
	}
}

func TestWorktreeWriterWithoutParityTestIsUnsafe(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit))
	if err := g.r.RemoveWorktree(ctxBG, localRepo, backend.RemoveWorktreeRequest{}); err != nil {
		t.Fatal(err)
	}
	if g.fallbacks(backend.OpRemoveWorktree, backend.CohortWorktree, backend.ReasonUnsafeWorktreeWrite) != 1 {
		t.Fatal("want unsafe_worktree_write")
	}
	verified := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) {
		c.VerifiedWorktreeWriters = map[backend.OperationName]bool{backend.OpRemoveWorktree: true}
	})
	verified.gg.removeWt = func(context.Context, backend.RepoLocation, backend.RemoveWorktreeRequest) error { return nil }
	if err := verified.r.RemoveWorktree(ctxBG, localRepo, backend.RemoveWorktreeRequest{}); err != nil || verified.log.count("gogit.RemoveWorktree") != 1 {
		t.Fatalf("a verified worktree writer runs in process: %v", err)
	}
}

func TestPullModeReasonIsNotAnError(t *testing.T) {
	g := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) {
		c.VerifiedWorktreeWriters = map[backend.OperationName]bool{backend.OpPull: true}
		c.Preflight = func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
			return []backend.FallbackReason{backend.ReasonUnsupportedPullMode}
		}
	})
	if err := g.r.Pull(ctxBG, localRepo, backend.PullRequest{}); err != nil {
		t.Fatal(err)
	}
	if g.fallbacks(backend.OpPull, backend.CohortNetwork, backend.ReasonUnsupportedPullMode) != 1 || g.fallbacks(backend.OpPull, backend.CohortNetwork, backend.ReasonError) != 0 {
		t.Fatal("want unsupported_pull_mode, never error")
	}
}

// ---- destructive intent ----

func TestDestructiveCleanIsConfirmedByTheCLI(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendGoGit}))
	g.gg.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return false, nil } // wrongly clean
	g.cli.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return true, nil }

	if dirty, err := g.r.IsDirty(ctxBG, localRepo, backend.IntentDestructive); err != nil || !dirty {
		t.Fatalf("destructive IsDirty = %v, %v; the CLI must override a go-git clean", dirty, err)
	}
	if g.fallbacks(backend.OpIsDirty, backend.CohortDiffStatus, backend.ReasonDestructiveConfirm) != 1 {
		t.Fatal("want destructive_confirm counted")
	}
	// zero value of Intent is Destructive: a caller that forgets is protected
	if dirty, _ := g.r.IsDirty(ctxBG, localRepo, 0); !dirty {
		t.Fatal("the zero Intent must behave as Destructive")
	}
	if dirty, err := g.r.IsDirty(ctxBG, localRepo, backend.IntentDisplay); err != nil || dirty {
		t.Fatalf("display IsDirty = %v, %v; go-git's (wrong) clean is what shadow exists to catch", dirty, err)
	}
	cliCalls := g.log.count("cli.IsDirty")

	g.gg.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return true, nil }
	if dirty, _ := g.r.IsDirty(ctxBG, localRepo, backend.IntentDestructive); !dirty || g.log.count("cli.IsDirty") != cliCalls {
		t.Fatal("a go-git dirty is trusted with no CLI spawn")
	}
}

func TestDestructiveCleanAppliesToStatusAndNumstat(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendGoGit}))
	dirtyStatus := backend.StatusResult{Files: []backend.FileStatus{{Path: "a"}}}
	dirtyRows := []backend.NumstatRow{{Path: "a", Added: 1}}
	g.cli.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
		return dirtyStatus, nil
	}
	g.gg.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
		return backend.StatusResult{}, nil
	}
	g.cli.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		return dirtyRows, nil
	}
	g.gg.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		return nil, nil
	}

	if st, _ := g.r.Status(ctxBG, localRepo, backend.IntentDestructive); !st.Dirty() {
		t.Error("Status(Destructive) must return the CLI's dirty answer")
	}
	if st, _ := g.r.Status(ctxBG, localRepo, backend.IntentDisplay); st.Dirty() {
		t.Error("Status(Display) returns go-git's answer")
	}
	if rows, _ := g.r.DiffNumstat(ctxBG, localRepo, backend.DiffSpec{Intent: backend.IntentDestructive}); len(rows) != 1 {
		t.Error("DiffNumstat(Destructive) must return the CLI's rows")
	}
	if rows, _ := g.r.DiffNumstat(ctxBG, localRepo, backend.DiffSpec{Intent: backend.IntentDisplay}); len(rows) != 0 {
		t.Error("DiffNumstat(Display) returns go-git's answer")
	}
	for _, op := range []backend.OperationName{backend.OpStatus, backend.OpDiffNumstat} {
		if g.fallbacks(op, backend.CohortDiffStatus, backend.ReasonDestructiveConfirm) != 1 {
			t.Errorf("%s: want exactly one destructive_confirm", op)
		}
	}
}

// ---- network never shadows a write ----

func TestShadowCohortNeverRunsMutatingOperationsInProcess(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortNetwork: backend.BackendShadow}))
	if err := g.r.Push(ctxBG, localRepo, backend.PushRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := g.r.Pull(ctxBG, localRepo, backend.PullRequest{}); err != nil {
		t.Fatal(err)
	}
	if g.log.count("cli.Push") != 1 || g.log.count("cli.Pull") != 1 || g.m.totalCount(backend.MetricShadowCallsTotal) != 0 {
		t.Fatal("push and pull must run on the CLI only and not shadow")
	}
	g.gg.listRemote = func(context.Context, backend.RepoLocation, backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
		return nil, nil
	}
	if _, err := g.r.ListRemote(ctxBG, localRepo, backend.ListRemoteRequest{}); err != nil {
		t.Fatal(err)
	}
	if g.m.count(backend.MetricShadowCallsTotal, "operation", "ListRemote") != 1 {
		t.Fatal("ls-remote is a read and may shadow")
	}
}

func TestErrorsFromTheCLIAreCountedUnlessTheyAreAnswers(t *testing.T) {
	g := newRig(t, backend.CohortMap{})
	g.cli.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrRefNotFound
	}
	_, _ = g.r.ResolveRef(ctxBG, localRepo, "x")
	g.cli.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", &backend.CommandError{Operation: backend.OpResolveRef, Err: errors.New("ssh down")}
	}
	_, _ = g.r.ResolveRef(ctxBG, localRepo, "x")
	if n := g.m.count(backend.MetricErrorTotal, "operation", "ResolveRef", "implementation", "cli"); n != 1 {
		t.Fatalf("error_total = %d, want 1 (only the operational failure)", n)
	}
	if strings.Contains(fmt.Sprint(backend.MetricErrorTotal), " ") {
		t.Fatal("metric name must not contain spaces")
	}
}
