package wsconn

// Test doubles the websocket tests share with the services core tests. They are copies, not imports: a _test.go
// helper of package services is not visible here, and the core cannot import this package.

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/instance"
)

// testMetricReader backs the one MeterProvider OTel's global delegate accepts per process.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

// collectMetric returns the collected metric named name, or nil if it has no data points yet.
func collectMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// captureInfoLog redirects the log package's slog seam to a buffer; the returned func restores it and returns the output.
func captureInfoLog() func() string {
	svctest.SlogDefaultMu.Lock()
	buf := &svctest.SyncLogBuffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := ssqlog.SetSlogDefaultForTest(slog.New(h))
	return func() string {
		ssqlog.SetSlogDefaultForTest(original)
		svctest.SlogDefaultMu.Unlock()
		return buf.String()
	}
}

// fakePtyFactory is a tmux.PtyFactory double that hands back a genuine PTY pair and never forks.
type fakePtyFactory struct {
	startErr error

	mu      sync.Mutex
	created []*os.File
}

func (f *fakePtyFactory) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	return f.StartWithSize(cmd, nil)
}

func (f *fakePtyFactory) StartWithSize(_ *exec.Cmd, _ *pty.Winsize) (*os.File, *exec.Cmd, error) {
	if f.startErr != nil {
		return nil, nil, f.startErr
	}
	master, slave, err := pty.Open()
	if err != nil {
		return nil, nil, err
	}
	_ = slave.Close()
	f.mu.Lock()
	f.created = append(f.created, master)
	f.mu.Unlock()
	return master, safeexec.CommandContext(context.Background(), "true"), nil
}

func (f *fakePtyFactory) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, file := range f.created {
		_ = file.Close()
	}
	f.created = nil
}

// fakeTmuxSender counts calls on the TmuxInputSender seam.
type fakeTmuxSender struct {
	mu      sync.Mutex
	inputs  [][]byte
	resizes [][2]int
}

func (f *fakeTmuxSender) SendInput(_, _ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, append([]byte(nil), data...))
	return nil
}

func (f *fakeTmuxSender) Resize(_ services.ExternalPaneTarget, cols, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, [2]int{cols, rows})
}

func (f *fakeTmuxSender) counts() (inputs, resizes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inputs), len(f.resizes)
}

func hiddenInstance(t *testing.T, hidden bool) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{
		Title:   "access-" + t.Name(),
		Path:    t.TempDir(),
		Program: "claude",
		Hidden:  hidden,
	})
	require.NoError(t, err)
	return inst
}

// bkfFlagWriter returns a setter that writes a flag through the production RPC into a config dir private to the test.
func bkfFlagWriter(t *testing.T) func(name string, on bool) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	svc := services.NewFeatureFlagService()
	return func(name string, on bool) {
		t.Helper()
		_, err := svc.UpdateFeatureFlag(context.Background(),
			connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: on}))
		require.NoError(t, err)
	}
}

// replyPM is a pane that reports itself alive and records nothing; enough for a started test instance.
type replyPM struct{ instance.ProcessManager }

func (r *replyPM) GetSessionIdentifier() string     { return "" }
func (r *replyPM) HasSession() bool                 { return true }
func (r *replyPM) IsAlive() bool                    { return true }
func (r *replyPM) HasMeaningfulContent(string) bool { return false }
