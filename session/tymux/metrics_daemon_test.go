package tymux

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func histogramCountByLabel(t *testing.T, name, labelKey, labelValue string) uint64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	h, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var total uint64
	for _, dp := range h.DataPoints {
		if v, ok := dp.Attributes.Value(attribute.Key(labelKey)); ok && v.AsString() == labelValue {
			total += dp.Count
		}
	}
	return total
}

func TestEnsureDaemonRunning_Spawn_RecordsColdStartRestartAndStartDuration(t *testing.T) {
	withFastRetryBounds(t, 5, time.Millisecond, 4*time.Millisecond)
	var healthyCalls int32
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool {
		return atomic.AddInt32(&healthyCalls, 1) >= 3
	})()
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { return &os.Process{}, nil })()
	defer stubStopTymuxd(func() error { return nil })()

	restartsBefore := counterByLabel(t, "tymux_daemon_restarts_total", "reason", "cold_start")
	okBefore := histogramCountByLabel(t, "tymux_daemon_start_duration_ms", "outcome", "ok")

	_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19990", BinaryPath: "tymuxd"})
	require.NoError(t, err)

	assert.Equal(t, restartsBefore+1, counterByLabel(t, "tymux_daemon_restarts_total", "reason", "cold_start"))
	assert.Equal(t, okBefore+1, histogramCountByLabel(t, "tymux_daemon_start_duration_ms", "outcome", "ok"))
	m := collectMetric(t, "tymux_daemon_start_duration_ms")
	require.NotNil(t, m)
	assert.Equal(t, "ms", m.Unit)
}

func TestEnsureDaemonRunning_NeverHealthy_RecordsFailedStartDuration(t *testing.T) {
	withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { return &os.Process{}, nil })()
	defer stubStopTymuxd(func() error { return nil })()
	defer stubPortListening(func(DaemonConfig) bool { return false })()

	failedBefore := histogramCountByLabel(t, "tymux_daemon_start_duration_ms", "outcome", "failed")

	_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19991", BinaryPath: "tymuxd"})
	require.Error(t, err)

	assert.Equal(t, failedBefore+1, histogramCountByLabel(t, "tymux_daemon_start_duration_ms", "outcome", "failed"))
}

func TestReconnectLoop_DaemonRestartDetected_RecordsRestartReason(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)
	transport.reviveSessionFn = func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
		return connect.NewResponse(&v1.ReviveSessionResponse{}), nil
	}
	var calls int32
	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		if atomic.AddInt32(&calls, 1) == 1 {
			s.recvErr = connect.NewError(connect.CodeFailedPrecondition, errors.New("pane exited"))
		} else {
			s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE}}})
		}
		return s
	}
	before := counterByLabel(t, "tymux_daemon_restarts_total", "reason", "detected_restart")

	close(stream.events) // drop -> redial reports a dead pane -> revive

	wait.RequireEventually(t, func() bool {
		return counterByLabel(t, "tymux_daemon_restarts_total", "reason", "detected_restart") == before+1
	}, time.Second, time.Millisecond, "a detected daemon restart must be counted")
}

func int64GaugeValue(t *testing.T, name string) int64 {
	t.Helper()
	m := collectMetric(t, name)
	require.NotNil(t, m, "%s not exported", name)
	g, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "unexpected data type %T", m.Data)
	require.Len(t, g.DataPoints, 1)
	return g.DataPoints[0].Value
}

func TestDetectDaemonVersions_RecordsVersionMismatchGauge(t *testing.T) {
	cfg := DaemonConfig{Addr: "127.0.0.1:1", BinaryPath: "/installed/tymuxd"}

	stubVersionSeams(t, "1.3.0", "1.4.0", 4242)
	DetectDaemonVersions(context.Background(), cfg)
	assert.EqualValues(t, 1, int64GaugeValue(t, "tymux_daemon_version_mismatch"), "running 1.3.0 vs installed 1.4.0")

	stubVersionSeams(t, "1.4.0", "1.4.0", 4242)
	DetectDaemonVersions(context.Background(), cfg)
	assert.EqualValues(t, 0, int64GaugeValue(t, "tymux_daemon_version_mismatch"), "matching versions")

	stubVersionSeams(t, "1.3.0", "1.4.0", 4242)
	DetectDaemonVersions(context.Background(), cfg)
	stubVersionSeams(t, "", "1.4.0", 0) // nothing running: direction unknown, gauge untouched
	DetectDaemonVersions(context.Background(), cfg)
	assert.EqualValues(t, 1, int64GaugeValue(t, "tymux_daemon_version_mismatch"), "unknown direction must not overwrite the last known value")
}

func TestReplaceDaemon_RecordsReplaceRestart(t *testing.T) {
	stubVersionSeams(t, "1.3.0", "1.2.0", 4242)
	origLive, origSig, origPort, origCheck, origStart, origStop := liveSessionsFn, signalProcessFn, portListeningFn, checkDaemonHealthyFn, startDaemonAttemptFn, stopTymuxdFn
	t.Cleanup(func() {
		liveSessionsFn, signalProcessFn, portListeningFn, checkDaemonHealthyFn, startDaemonAttemptFn, stopTymuxdFn = origLive, origSig, origPort, origCheck, origStart, origStop
	})
	liveSessionsFn = func(context.Context, DaemonConfig) (int, error) { return 0, nil }
	stopped := false
	signalProcessFn = func(int, os.Signal) error { stopped = true; return nil }
	portListeningFn = func(DaemonConfig) bool { return !stopped }
	spawned := false
	startDaemonAttemptFn = func(DaemonConfig) (*os.Process, error) { spawned = true; return &os.Process{}, nil }
	checkDaemonHealthyFn = func(context.Context, DaemonConfig) bool { return spawned }
	stopTymuxdFn = func() error { return nil }
	before := counterByLabel(t, "tymux_daemon_restarts_total", "reason", "replace")
	coldBefore := counterByLabel(t, "tymux_daemon_restarts_total", "reason", "cold_start")

	_, err := ReplaceDaemon(context.Background(), DaemonConfig{BinaryPath: "/installed/tymuxd"}, true)
	require.NoError(t, err)

	assert.Equal(t, before+1, counterByLabel(t, "tymux_daemon_restarts_total", "reason", "replace"))
	assert.Equal(t, coldBefore, counterByLabel(t, "tymux_daemon_restarts_total", "reason", "cold_start"),
		"a deliberate replace must not also count as a cold start")
}
