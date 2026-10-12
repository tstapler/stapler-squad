package backend_test

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

type spawnPoint struct {
	labels map[string]string
	value  int64
}

// points returns every data point of an Int64 sum metric with its full label set.
func (m *metricsRig) points(name string) []spawnPoint {
	m.t.Helper()
	var rm metricdata.ResourceMetrics
	if err := m.reader.Collect(context.Background(), &rm); err != nil {
		m.t.Fatalf("collect: %v", err)
	}
	var out []spawnPoint
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			sum, ok := met.Data.(metricdata.Sum[int64])
			if !ok || met.Name != name {
				continue
			}
			for _, dp := range sum.DataPoints {
				labels := map[string]string{}
				for _, kv := range dp.Attributes.ToSlice() {
					labels[string(kv.Key)] = kv.Value.AsString()
				}
				out = append(out, spawnPoint{labels, dp.Value})
			}
		}
	}
	return out
}

// quietRunner answers every git command with empty output and records the argv count.
type quietRunner struct {
	mu    sync.Mutex
	calls int
}

func (q *quietRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	return nil, nil
}

func (q *quietRunner) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.calls
}

// spawnRouter wires a Router over the real CLI backend and one private meter provider.
func spawnRouter(t *testing.T, cohorts backend.CohortMap, gg backend.Backend, local backend.Runner) (*backend.Router, *metricsRig) {
	t.Helper()
	rig, mp := newMetricsRig(t)
	r, err := backend.NewRouter(backend.RouterConfig{
		Cohorts: cohorts, CLI: cli.New(local, cli.WithMeter(mp.Meter("test"))), GoGit: gg,
		Preflight: noPreflight, Meter: mp.Meter("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r, rig
}

func TestSpawnLabels(t *testing.T) {
	ctx := backend.WithCallInfo(context.Background(), backend.CallInfo{Op: backend.OpFetch, Reason: backend.ReasonCapabilitySSHProxy})
	cases := []struct {
		name       string
		ctx        context.Context
		served     backend.OperationName
		wantOp     backend.OperationName
		wantReason backend.FallbackReason
	}{
		{"router attribution wins", ctx, backend.OpLog, backend.OpFetch, backend.ReasonCapabilitySSHProxy},
		{"no call info", context.Background(), backend.OpLog, backend.OpLog, backend.SpawnReasonUnattributed},
		{"unknown reason is not a label", backend.WithCallInfo(context.Background(), backend.CallInfo{Op: backend.OpFetch, Reason: "https://u:tok@host"}), backend.OpLog, backend.OpLog, backend.SpawnReasonUnattributed},
		{"unknown operation is not a label", backend.WithCallInfo(context.Background(), backend.CallInfo{Op: "rm -rf /", Reason: backend.ReasonConfig}), backend.OpLog, backend.OpLog, backend.SpawnReasonUnattributed},
		{"unknown served op", context.Background(), "weird", "unknown", backend.SpawnReasonUnattributed},
	}
	if op, reason := backend.SpawnLabels(context.Background(), backend.OpLog, true); op != backend.OpLog || reason != backend.ReasonRemoteHost {
		t.Errorf("remote without call info: got (%s,%s), want remote_host", op, reason)
	}
	for _, c := range cases {
		op, reason := backend.SpawnLabels(c.ctx, c.served, false)
		if op != c.wantOp || reason != c.wantReason {
			t.Errorf("%s: got (%s,%s), want (%s,%s)", c.name, op, reason, c.wantOp, c.wantReason)
		}
	}
}

// Plan Story 1.3.2 AC1: a Fetch routed to the CLI for capability_ssh_proxy counts once per
// Runner.Run, and a method that runs several commands counts each (DiscardChanges is in the
// localwrite cohort, which is not opted in here, so it is served for reason config).
func TestSpawnCounterAttributesOperationAndReasonPerRun(t *testing.T) {
	local := &quietRunner{}
	gg := strictGoGit(t, newEventLog())
	pre := func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
		return []backend.FallbackReason{backend.ReasonCapabilitySSHProxy}
	}
	rig, mp := newMetricsRig(t)
	r, err := backend.NewRouter(backend.RouterConfig{
		Cohorts: allCohorts(backend.BackendGoGit), CLI: cli.New(local, cli.WithMeter(mp.Meter("test"))), GoGit: gg,
		Preflight: pre, Meter: mp.Meter("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Fetch(ctxBG, backend.Local{Root: "/repo"}, backend.FetchRequest{Remote: "origin"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := rig.count(backend.MetricCLISpawnTotal, "operation", "Fetch", "reason", "capability_ssh_proxy"); got != 1 {
		t.Fatalf("Fetch/capability_ssh_proxy = %d, want 1", got)
	}

	before := local.count()
	if err := r.DiscardChanges(ctxBG, backend.Local{Root: "/repo"}); err != nil {
		t.Fatalf("DiscardChanges: %v", err)
	}
	if ran := local.count() - before; ran < 2 {
		t.Fatalf("DiscardChanges ran %d git commands, want a multi-command method", ran)
	}
	if got, want := rig.count(backend.MetricCLISpawnTotal, "operation", "DiscardChanges", "reason", "config"), int64(local.count()-before); got != want {
		t.Fatalf("DiscardChanges counted %d spawns for %d Runner.Run calls", got, want)
	}
}

func TestSpawnReasonsPerRoute(t *testing.T) {
	t.Run("cli cohort is the configured default", func(t *testing.T) {
		local := &quietRunner{}
		r, rig := spawnRouter(t, allCohorts(backend.BackendCLI), nil, local)
		_, _ = r.CurrentBranch(ctxBG, backend.Local{Root: "/repo"})
		if rig.count(backend.MetricCLISpawnTotal, "operation", "CurrentBranch", "reason", "config") != 1 {
			t.Fatalf("points = %v", rig.points(backend.MetricCLISpawnTotal))
		}
	})
	t.Run("remote host", func(t *testing.T) {
		remote := &quietRunner{}
		r, rig := spawnRouter(t, allCohorts(backend.BackendGoGit), strictGoGit(t, newEventLog()), &quietRunner{})
		_, _ = r.CurrentBranch(ctxBG, backend.Remote{Host: "h", Path: "/p", Runner: remote})
		if rig.count(backend.MetricCLISpawnTotal, "operation", "CurrentBranch", "reason", "remote_host") != 1 || remote.count() != 1 {
			t.Fatalf("points = %v remote runs = %d", rig.points(backend.MetricCLISpawnTotal), remote.count())
		}
	})
	t.Run("go-git failure falls back with its reason", func(t *testing.T) {
		gg := strictGoGit(t, newEventLog())
		gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
			return "", backend.ErrObjectNotFound
		}
		r, rig := spawnRouter(t, allCohorts(backend.BackendGoGit), gg, &quietRunner{})
		_, _ = r.ResolveRef(ctxBG, backend.Local{Root: "/repo"}, "main")
		if rig.count(backend.MetricCLISpawnTotal, "operation", "ResolveRef", "reason", "object_not_found") != 1 {
			t.Fatalf("points = %v", rig.points(backend.MetricCLISpawnTotal))
		}
	})
	t.Run("destructive confirmation", func(t *testing.T) {
		gg := strictGoGit(t, newEventLog())
		gg.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return false, nil }
		r, rig := spawnRouter(t, allCohorts(backend.BackendGoGit), gg, &quietRunner{})
		_, _ = r.IsDirty(ctxBG, backend.Local{Root: "/repo"}, backend.IntentDestructive)
		if rig.count(backend.MetricCLISpawnTotal, "operation", "IsDirty", "reason", "destructive_confirm") != 1 {
			t.Fatalf("points = %v", rig.points(backend.MetricCLISpawnTotal))
		}
	})
	t.Run("shadow serves from the cli by configuration", func(t *testing.T) {
		gg := strictGoGit(t, newEventLog())
		gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
			return "", nil
		}
		gg.gitDir = func(context.Context, backend.RepoLocation) (backend.GitDir, error) { return ".git", nil }
		r, rig := spawnRouter(t, allCohorts(backend.BackendShadow), gg, &quietRunner{})
		_, _ = r.ResolveRef(ctxBG, backend.Local{Root: "/repo"}, "main")
		if rig.count(backend.MetricCLISpawnTotal, "operation", "ResolveRef", "reason", "config") < 1 {
			t.Fatalf("points = %v", rig.points(backend.MetricCLISpawnTotal))
		}
	})
	t.Run("a call that skipped the router is explicitly unattributed", func(t *testing.T) {
		rig, mp := newMetricsRig(t)
		b := cli.New(&quietRunner{}, cli.WithMeter(mp.Meter("test")))
		_, _ = b.CurrentBranch(ctxBG, backend.Local{Root: "/repo"})
		if rig.count(backend.MetricCLISpawnTotal, "operation", "CurrentBranch", "reason", "unattributed") != 1 {
			t.Fatalf("points = %v", rig.points(backend.MetricCLISpawnTotal))
		}
	})
}

// Allow-list ratchet (plan Alerts (a)): every method, on every route, produces only
// (operation, reason) pairs inside the closed enums, labelled with exactly those two keys,
// and the operation label is the method actually called.
func TestSpawnCounterAllowList(t *testing.T) {
	allowedOps := backend.AllOperations()
	allowedReasons := backend.SpawnReasons()
	rt := reflect.TypeOf((*backend.Backend)(nil)).Elem()

	scenarios := map[string]struct {
		build func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation)
		want  backend.FallbackReason
	}{
		"local cli cohort": {want: backend.ReasonConfig, build: func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation) {
			r, rig := spawnRouter(t, allCohorts(backend.BackendCLI), nil, &quietRunner{})
			return r, rig, backend.Local{Root: "/repo"}
		}},
		"remote host": {want: backend.ReasonRemoteHost, build: func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation) {
			r, rig := spawnRouter(t, allCohorts(backend.BackendCLI), nil, nil)
			return r, rig, backend.Remote{Host: "h", Path: "/p", Runner: &quietRunner{}}
		}},
		"direct cli remote, no router": {want: backend.ReasonRemoteHost, build: func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation) {
			rig, mp := newMetricsRig(t)
			return cli.New(nil, cli.WithMeter(mp.Meter("test"))), rig, backend.Remote{Host: "h", Path: "/p", Runner: &quietRunner{}}
		}},
		"direct cli, remote, no router": {want: backend.ReasonRemoteHost, build: func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation) {
			rig, mp := newMetricsRig(t)
			return cli.New(nil, cli.WithMeter(mp.Meter("test"))), rig, backend.Remote{Host: "h", Path: "/p", Runner: &quietRunner{}}
		}},
		"direct cli, no router": {want: backend.SpawnReasonUnattributed, build: func(t *testing.T) (backend.Backend, *metricsRig, backend.RepoLocation) {
			rig, mp := newMetricsRig(t)
			return cli.New(&quietRunner{}, cli.WithMeter(mp.Meter("test"))), rig, backend.Local{Root: "/repo"}
		}},
	}
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			b, rig, loc := sc.build(t)
			rv := reflect.ValueOf(b)
			spawned := 0
			for i := 0; i < rt.NumMethod(); i++ {
				method := rt.Method(i).Name
				before := total(rig.points(backend.MetricCLISpawnTotal))
				func() {
					defer func() { _ = recover() }()
					m := rv.MethodByName(method)
					m.Call(zeroArgs(m.Type(), loc))
				}()
				after := rig.points(backend.MetricCLISpawnTotal)
				if total(after) > before {
					spawned++
					if got := sumFor(after, "operation", method); got == 0 {
						t.Errorf("%s spawned git but no spawn was attributed to operation %q: %v", method, method, after)
					}
				}
			}
			if spawned < 10 {
				t.Fatalf("only %d methods spawned; the sweep is not exercising the backend", spawned)
			}
			if name != "direct cli, no router" && sumFor(rig.points(backend.MetricCLISpawnTotal), "reason", string(backend.SpawnReasonUnattributed)) != 0 {
				t.Errorf("a routed or remote call produced unattributed spawns: %v", rig.points(backend.MetricCLISpawnTotal))
			}
			for _, p := range rig.points(backend.MetricCLISpawnTotal) {
				if len(p.labels) != 2 {
					t.Errorf("labels = %v, want exactly operation and reason", p.labels)
				}
				if !slices.Contains(allowedOps, backend.OperationName(p.labels["operation"])) {
					t.Errorf("operation %q outside the allow-list", p.labels["operation"])
				}
				if !slices.Contains(allowedReasons, backend.FallbackReason(p.labels["reason"])) {
					t.Errorf("reason %q outside the allow-list", p.labels["reason"])
				}
				if backend.FallbackReason(p.labels["reason"]) != sc.want {
					t.Errorf("%v: reason %q, want %q", p.labels, p.labels["reason"], sc.want)
				}
			}
		})
	}
}

func total(ps []spawnPoint) (n int64) {
	for _, p := range ps {
		n += p.value
	}
	return n
}

func sumFor(ps []spawnPoint, key, val string) (n int64) {
	for _, p := range ps {
		if p.labels[key] == val {
			n += p.value
		}
	}
	return n
}

func TestSpawnReasonsAreClosedAndIncludeUnattributed(t *testing.T) {
	reasons := backend.SpawnReasons()
	if !slices.Contains(reasons, backend.SpawnReasonUnattributed) || len(reasons) != len(backend.ReasonPrecedence())+1 {
		t.Fatalf("SpawnReasons = %v", reasons)
	}
	if backend.SpawnReasonUnattributed.Known() {
		t.Fatal("unattributed must stay outside the router's FallbackReason enum")
	}
}
