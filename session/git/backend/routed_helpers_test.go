package backend_test

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// fakeBackend answers only the methods a test wires; any other method panics on the nil
// embedded interface, which fails the test loudly.
type fakeBackend struct {
	backend.Backend
	name string
	log  *eventLog

	currentBranch func(ctx context.Context, loc backend.RepoLocation) (backend.BranchName, error)
	resolveRef    func(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error)
	gitDir        func(ctx context.Context, loc backend.RepoLocation) (backend.GitDir, error)
	isDirty       func(ctx context.Context, loc backend.RepoLocation, intent backend.Intent) (bool, error)
	status        func(ctx context.Context, loc backend.RepoLocation, intent backend.Intent) (backend.StatusResult, error)
	numstat       func(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) ([]backend.NumstatRow, error)
	diff          func(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) (string, error)
	commit        func(ctx context.Context, loc backend.RepoLocation, req backend.CommitRequest) error
	push          func(ctx context.Context, loc backend.RepoLocation, req backend.PushRequest) error
	pull          func(ctx context.Context, loc backend.RepoLocation, req backend.PullRequest) error
	listRemote    func(ctx context.Context, loc backend.RepoLocation, req backend.ListRemoteRequest) ([]backend.RemoteRef, error)
	removeWt      func(ctx context.Context, loc backend.RepoLocation, req backend.RemoveWorktreeRequest) error
}

// eventLog records cross-component ordering (preflight before gogit, and so on).
type eventLog struct {
	mu     sync.Mutex
	events []string
	counts map[string]int
}

func newEventLog() *eventLog { return &eventLog{counts: map[string]int{}} }

func (l *eventLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	l.counts[e]++
}

func (l *eventLog) count(e string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[e]
}

func (l *eventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

func (f *fakeBackend) rec(op string) { f.log.add(f.name + "." + op) }

func (f *fakeBackend) CurrentBranch(ctx context.Context, loc backend.RepoLocation) (backend.BranchName, error) {
	f.rec("CurrentBranch")
	return f.currentBranch(ctx, loc)
}

func (f *fakeBackend) ResolveRef(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	f.rec("ResolveRef")
	return f.resolveRef(ctx, loc, ref)
}

func (f *fakeBackend) GitDir(ctx context.Context, loc backend.RepoLocation) (backend.GitDir, error) {
	f.rec("GitDir")
	return f.gitDir(ctx, loc)
}

func (f *fakeBackend) IsDirty(ctx context.Context, loc backend.RepoLocation, in backend.Intent) (bool, error) {
	f.rec("IsDirty")
	return f.isDirty(ctx, loc, in)
}

func (f *fakeBackend) Status(ctx context.Context, loc backend.RepoLocation, in backend.Intent) (backend.StatusResult, error) {
	f.rec("Status")
	return f.status(ctx, loc, in)
}

func (f *fakeBackend) DiffNumstat(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) ([]backend.NumstatRow, error) {
	f.rec("DiffNumstat")
	return f.numstat(ctx, loc, spec)
}

func (f *fakeBackend) Diff(ctx context.Context, loc backend.RepoLocation, spec backend.DiffSpec) (string, error) {
	f.rec("Diff")
	return f.diff(ctx, loc, spec)
}

func (f *fakeBackend) Commit(ctx context.Context, loc backend.RepoLocation, req backend.CommitRequest) error {
	f.rec("Commit")
	return f.commit(ctx, loc, req)
}

func (f *fakeBackend) Push(ctx context.Context, loc backend.RepoLocation, req backend.PushRequest) error {
	f.rec("Push")
	return f.push(ctx, loc, req)
}

func (f *fakeBackend) Pull(ctx context.Context, loc backend.RepoLocation, req backend.PullRequest) error {
	f.rec("Pull")
	return f.pull(ctx, loc, req)
}

func (f *fakeBackend) ListRemote(ctx context.Context, loc backend.RepoLocation, req backend.ListRemoteRequest) ([]backend.RemoteRef, error) {
	f.rec("ListRemote")
	return f.listRemote(ctx, loc, req)
}

func (f *fakeBackend) RemoveWorktree(ctx context.Context, loc backend.RepoLocation, req backend.RemoveWorktreeRequest) error {
	f.rec("RemoveWorktree")
	return f.removeWt(ctx, loc, req)
}

// metricsRig reads the counters a Router registers on a private in-memory meter provider.
type metricsRig struct {
	reader *sdkmetric.ManualReader
	meter  metric.Meter
	t      *testing.T
}

func newMetricsRig(t *testing.T) (*metricsRig, *sdkmetric.MeterProvider) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return &metricsRig{reader: reader, meter: mp.Meter("test"), t: t}, mp
}

// count sums the data points of metric name whose attributes include every key/value pair given.
func (m *metricsRig) count(name string, kv ...string) int64 {
	m.t.Helper()
	var rm metricdata.ResourceMetrics
	if err := m.reader.Collect(context.Background(), &rm); err != nil {
		m.t.Fatalf("collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, met := range sm.Metrics {
			sum, ok := met.Data.(metricdata.Sum[int64])
			if !ok || met.Name != name {
				continue
			}
			for _, dp := range sum.DataPoints {
				match := true
				for i := 0; i+1 < len(kv); i += 2 {
					if v, ok := dp.Attributes.Value(attrKey(kv[i])); !ok || v.AsString() != kv[i+1] {
						match = false
					}
				}
				if match {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// totalCount sums every data point of the named metric.
func (m *metricsRig) totalCount(name string) int64 { return m.count(name) }

type recordingRecorder struct {
	mu   sync.Mutex
	recs []backend.MismatchRecord
}

func (r *recordingRecorder) Record(rec backend.MismatchRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recs = append(r.recs, rec)
}

func (r *recordingRecorder) all() []backend.MismatchRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]backend.MismatchRecord(nil), r.recs...)
}
