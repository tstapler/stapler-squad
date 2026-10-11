package gitwiring_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitwiring"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// One process-lifetime reader: both spawn counters are built against the global meter at init,
// and only the first SetMeterProvider rewires them (see executor/safeexec/safeexec_pg_test.go).
// Assertions therefore diff against a baseline instead of reading absolute values.
var gateReader = sdkmetric.NewManualReader()

func init() { otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(gateReader))) }

type spawnTotals struct{ backstop, backendLocal, backendRemote int64 }

func readTotals(t *testing.T) spawnTotals {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := gateReader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var tot spawnTotals
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				switch m.Name {
				case safeexec.MetricGitSpawnBackstop:
					tot.backstop += dp.Value
				case backend.MetricCLISpawnTotal:
					if v, _ := dp.Attributes.Value("reason"); v.AsString() == string(backend.ReasonRemoteHost) {
						tot.backendRemote += dp.Value
					} else {
						tot.backendLocal += dp.Value
					}
				}
			}
		}
	}
	return tot
}

func (a spawnTotals) since(b spawnTotals) spawnTotals {
	return spawnTotals{a.backstop - b.backstop, a.backendLocal - b.backendLocal, a.backendRemote - b.backendRemote}
}

// gateHolds is the plan's gate invariant: sum(backstop) <= sum(backend, reason != remote_host).
func (s spawnTotals) gateHolds() bool { return s.backstop <= s.backendLocal }

type fakeSSH struct{ runs int }

func (f *fakeSSH) Run(context.Context, string, string, ...string) ([]byte, error) {
	f.runs++
	return []byte("main\n"), nil
}

func newLocalRouter() backend.Backend {
	return gitwiring.NewRouter(backend.CohortMap{}, tmux.LocalRunner{})
}

func TestSpawnGateInvariant(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() // not a repository: git still starts, which is all the counters see

	t.Run("a bypass raises the backstop only and fails the gate", func(t *testing.T) {
		before := readTotals(t)
		_ = safeexec.CommandContext(ctx, "git", "status")
		d := readTotals(t).since(before)
		if d.backstop != 1 || d.backendLocal != 0 || d.backendRemote != 0 {
			t.Fatalf("delta = %+v, want backstop=1 and no backend counts", d)
		}
		if d.gateHolds() {
			t.Fatal("the gate must fail when a spawn bypasses the backend counter")
		}
	})

	t.Run("a local call through LocalRunner raises both by one and passes", func(t *testing.T) {
		r := newLocalRouter()
		before := readTotals(t)
		_, _ = r.CurrentBranch(ctx, backend.Local{Root: backend.RepoRoot(dir)})
		d := readTotals(t).since(before)
		if d.backstop != 1 || d.backendLocal != 1 || d.backendRemote != 0 {
			t.Fatalf("delta = %+v, want 1/1/0", d)
		}
		if !d.gateHolds() {
			t.Fatal("gate must hold")
		}
		// An over-counting wrapper (backend counted a spawn that never happened) would push
		// backendLocal above the backstop; in this controlled run they must be equal.
		if d.backendLocal != d.backstop {
			t.Fatalf("backend counted %d local spawns but %d git processes started", d.backendLocal, d.backstop)
		}
	})

	t.Run("a remote call raises only the remote_host backend count and passes", func(t *testing.T) {
		r := newLocalRouter()
		ssh := &fakeSSH{}
		before := readTotals(t)
		if _, err := r.CurrentBranch(ctx, backend.Remote{Host: "h", Path: "/p", Runner: ssh}); err != nil {
			t.Fatal(err)
		}
		d := readTotals(t).since(before)
		if d.backstop != 0 || d.backendLocal != 0 || d.backendRemote != 1 || ssh.runs != 1 {
			t.Fatalf("delta = %+v, ssh runs = %d, want 0/0/1 and one run", d, ssh.runs)
		}
		if !d.gateHolds() {
			t.Fatal("remote runs must neither misfire the gate nor mask a local bypass")
		}
	})

	t.Run("remote spawns do not mask a local bypass", func(t *testing.T) {
		r := newLocalRouter()
		before := readTotals(t)
		_, _ = r.CurrentBranch(ctx, backend.Remote{Host: "h", Path: "/p", Runner: &fakeSSH{}})
		_ = safeexec.CommandContext(ctx, "git", "status")
		if readTotals(t).since(before).gateHolds() {
			t.Fatal("a remote run offset a local bypass; remote_host must be excluded from the right side")
		}
	})
}

func TestSpawnDumpEnvNamesAgree(t *testing.T) {
	if backend.SpawnDumpEnv != safeexec.SpawnDumpEnv {
		t.Fatalf("backend %q and safeexec %q disagree on the dump variable", backend.SpawnDumpEnv, safeexec.SpawnDumpEnv)
	}
}

// Plan Story 1.3.2 AC3: with the dump set, each spawn's file:line and operation are written.
func TestSpawnDumpCarriesFileLineAndOperation(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "d.log")
	t.Setenv(backend.SpawnDumpEnv, dump)
	r := newLocalRouter()
	_, _ = r.CurrentBranch(context.Background(), backend.Local{Root: backend.RepoRoot(t.TempDir())})
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	var sawBackend, sawExec bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		switch {
		case strings.HasPrefix(line, "backend ") && strings.Contains(line, "spawn_gate_test.go:") &&
			strings.Contains(line, "operation=CurrentBranch") && strings.Contains(line, "reason=config"):
			sawBackend = true
		case strings.HasPrefix(line, "exec ") && strings.Contains(line, "spawn_gate_test.go:") && strings.HasSuffix(line, "subcommand=rev-parse"):
			sawExec = true
		}
	}
	if !sawBackend || !sawExec {
		t.Fatalf("dump missing a line (backend=%v exec=%v):\n%s", sawBackend, sawExec, data)
	}
}
