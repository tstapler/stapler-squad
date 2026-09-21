package tymux

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/lifecycle"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// testTymuxMetricReader backs the single real MeterProvider installed for
// this test binary. session/lifecycle's package-level instruments are
// constructed against OTel's global delegating meter at package-init time,
// before this file's init() runs -- otel.SetMeterProvider rewires what that
// delegating meter forwards to, so a process-lifetime manual reader with
// delta-based (before/after) assertions is the correct fit here, mirroring
// session/lifecycle/observability_test.go's and
// session/pi_status_source_metrics_test.go's identical pattern.
var testTymuxMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testTymuxMetricReader)))
}

// collectMetric returns name's current collected data, or nil if it has no
// recorded data points yet.
func collectMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testTymuxMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumForSubsystem sums an int64 Sum metric's data points whose "subsystem"
// attribute equals subsystem, further filtered by reason when reason != ""
// (session_lifecycle_active_generations carries no "reason" attribute, so
// callers pass "" for that metric).
func sumForSubsystem(t *testing.T, m *metricdata.Metrics, subsystem, reason string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		sv, ok := dp.Attributes.Value(attribute.Key("subsystem"))
		if !ok || sv.AsString() != subsystem {
			continue
		}
		if reason != "" {
			rv, ok := dp.Attributes.Value(attribute.Key("reason"))
			if !ok || rv.AsString() != reason {
				continue
			}
		}
		total += dp.Value
	}
	return total
}

// fakeAttachStream is a hand-driven attachStream substitute: Receive()
// plays back queued events (send more anytime via push) until either the
// events channel is closed (io.EOF, mimicking a server-ended stream) or
// ctx is cancelled (mimicking DetachSafely()'s full-cancellation contract,
// Task 2.3.4b) — matching how the real
// *connect.BidiStreamForClient[...]'s Receive reacts to context
// cancellation.
type fakeAttachStream struct {
	ctx    context.Context
	events chan *v1.AttachEvent
	sendFn func(*v1.AttachRequest) error

	// recvErr, if set, makes every Receive() call return this error
	// immediately instead of consulting events/ctx — used by Epic 2.5's
	// reconnect tests to simulate a stream that ends immediately with a
	// specific RPC error (e.g. connect.CodeFailedPrecondition, Task
	// 2.5.3a) without a live server.
	recvErr error

	mu   sync.Mutex
	sent []*v1.AttachRequest
}

func newFakeAttachStream(ctx context.Context) *fakeAttachStream {
	return &fakeAttachStream{ctx: ctx, events: make(chan *v1.AttachEvent, 16)}
}

func (f *fakeAttachStream) Send(req *v1.AttachRequest) error {
	f.mu.Lock()
	f.sent = append(f.sent, req)
	f.mu.Unlock()
	if f.sendFn != nil {
		return f.sendFn(req)
	}
	return nil
}

func (f *fakeAttachStream) Receive() (*v1.AttachEvent, error) {
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	select {
	case ev, ok := <-f.events:
		if !ok {
			return nil, io.EOF
		}
		return ev, nil
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	}
}

func (f *fakeAttachStream) CloseRequest() error  { return nil }
func (f *fakeAttachStream) CloseResponse() error { return nil }

func (f *fakeAttachStream) push(ev *v1.AttachEvent) { f.events <- ev }

func (f *fakeAttachStream) sentRequests() []*v1.AttachRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*v1.AttachRequest, len(f.sent))
	copy(out, f.sent)
	return out
}

var _ attachStream = (*fakeAttachStream)(nil)

// wedgedAttachStream's Receive() never returns, regardless of ctx
// cancellation — simulating incident #3's stuck reader (a transport that
// doesn't honor context cancellation), the exact shape
// teardownStandingStream's bounded wait (lifecycle.AwaitBounded) exists to
// survive. Unlike fakeAttachStream, which selects on ctx.Done(), this type
// deliberately never does.
type wedgedAttachStream struct {
	sent chan *v1.AttachRequest
}

func (w *wedgedAttachStream) Send(req *v1.AttachRequest) error {
	select {
	case w.sent <- req:
	default:
	}
	return nil
}

func (w *wedgedAttachStream) Receive() (*v1.AttachEvent, error) {
	select {} // never returns
}

func (w *wedgedAttachStream) CloseRequest() error  { return nil }
func (w *wedgedAttachStream) CloseResponse() error { return nil }

var _ attachStream = (*wedgedAttachStream)(nil)

// startedSessionWithStream builds a started tymuxGRPCSession (via the
// fakeTransport helpers in session_test.go) and returns it alongside the
// fakeAttachStream the standing stream opened against, and the
// fakeTransport itself (for attachCalls assertions).
func startedSessionWithStream(t *testing.T) (TymuxManager, *fakeAttachStream, *fakeTransport) {
	t.Helper()
	dir := t.TempDir()
	var stream *fakeAttachStream
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	transport.attachFn = func(ctx context.Context) attachStream {
		stream = newFakeAttachStream(ctx)
		return stream
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	require.NotNil(t, stream)
	// Every test built from this helper starts a real readAttachLoop
	// goroutine (blocked in fakeAttachStream.Receive()'s select until the
	// stream ends). Without this cleanup it leaks for the rest of the
	// test binary's lifetime — Close() is idempotent-safe even for tests
	// that already ended the session themselves (e.g. via DetachSafely
	// or a simulated stream exhaustion).
	t.Cleanup(func() { _ = sess.Close() })
	return sess, stream, transport
}

// --- Story 2.3.1 ---

func TestStandingStream_OpensExactlyOnce_NotPerSendKeysCall(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)

	_, err := sess.SendKeys("a")
	require.NoError(t, err)
	_, err = sess.SendKeys("b")
	require.NoError(t, err)
	_, err = sess.SendKeys("c")
	require.NoError(t, err)

	assert.EqualValues(t, 1, transport.attachCalls, "exactly one Attach stream must be opened, not one per SendKeys call")
}

func TestStandingStream_SendsPaneIdAsFirstMessage(t *testing.T) {
	_, stream, _ := startedSessionWithStream(t)

	sent := stream.sentRequests()
	require.Len(t, sent, 1)
	assert.Equal(t, "pane-1", sent[0].GetPaneId())
}

func TestStandingStream_SnapshotEvent_SeedsLiveness(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
		Snapshot: &v1.PaneSnapshot{PaneId: "pane-1", Liveness: v1.Liveness_LIVENESS_DEAD},
	}})

	// IsAlive() always falls back to a fresh CapturePane RPC (Task
	// 2.2.1c) rather than trusting the cache, so assert on the cached
	// field the reader goroutine actually writes instead.
	concrete := sess.(*tymuxGRPCSession)
	wait.RequireEventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.liveness == v1.Liveness_LIVENESS_DEAD
	}, time.Second, time.Millisecond, "snapshot event should seed cached liveness from PaneSnapshot.liveness")
}

// --- Story 2.3.2 ---

func TestStandingStream_OutputEvent_FansOutToAllSubscribers(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	_, ch1 := sess.SubscribeToControlModeUpdates()
	_, ch2 := sess.SubscribeToControlModeUpdates()

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("hi")}})

	for i, ch := range []chan []byte{ch1, ch2} {
		select {
		case got := <-ch:
			assert.Equal(t, []byte("hi"), got, "subscriber %d", i)
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d never received the Output event", i)
		}
	}
}

func TestStandingStream_UnsubscribeFromControlModeUpdates_StopsDelivery(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	id, ch := sess.SubscribeToControlModeUpdates()

	sess.UnsubscribeFromControlModeUpdates(id)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("hi")}})

	_, open := <-ch
	assert.False(t, open, "the channel must be closed after Unsubscribe")
}

// --- Story 2.3.3 ---

func TestStandingStream_SendKeys_SendsInputOnExistingStream(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)

	n, err := sess.SendKeys("echo hi\n")

	require.NoError(t, err)
	assert.Equal(t, len("echo hi\n"), n)
	assert.EqualValues(t, 1, transport.attachCalls, "SendKeys must not open a new stream")
	sent := stream.sentRequests()
	require.Len(t, sent, 2) // [0] = pane_id, [1] = this Input
	assert.Equal(t, []byte("echo hi\n"), sent[1].GetInput())
}

func TestStandingStream_TapEnter_SendsCarriageReturn(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.TapEnter())

	sent := stream.sentRequests()
	require.Len(t, sent, 2)
	assert.Equal(t, []byte{0x0D}, sent[1].GetInput())
}

func TestStandingStream_ExitedEvent_UpdatesLivenessAndFiresRegisteredCallback(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	reasons := make(chan string, 1)
	sess.SetOnExitCallback(func(reason string) { reasons <- reason })

	code := int32(42)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	select {
	case reason := <-reasons:
		assert.Equal(t, "exited: code=42", reason)
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired")
	}

	concrete := sess.(*tymuxGRPCSession)
	wait.RequireEventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.liveness == v1.Liveness_LIVENESS_DEAD
	}, time.Second, time.Millisecond, "Exited event must update cached liveness to DEAD")
}

// --- Story 2.3.4 ---

// TestStandingStream_Attach_ReturnsChannelClosedWhenDeliberatelyDetached
// replaces the old (pre-Epic-2.5)
// TestStandingStream_Attach_ReturnsChannelClosedWhenStreamEnds: under
// Story 2.5.1, ending the stream WITHOUT a preceding Close()/
// DetachSafely() is a drop that triggers ReconnectLoop, not an immediate
// "the standing stream ended" signal — see the reconnect-specific tests
// below. Attach()'s channel only closes immediately for a deliberate
// detach, exercised here via DetachSafely() itself rather than directly
// closing the fake stream's events channel.
func TestStandingStream_Attach_ReturnsChannelClosedWhenDeliberatelyDetached(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)

	done, err := sess.Attach()
	require.NoError(t, err)

	select {
	case <-done:
		t.Fatal("Attach's channel closed before DetachSafely was called")
	default:
	}

	require.NoError(t, sess.DetachSafely())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Attach's channel never closed after a deliberate DetachSafely")
	}
}

func TestStandingStream_DetachSafely_CancelsTheAttachContext(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.DetachSafely())

	select {
	case <-stream.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("DetachSafely must fully cancel the Attach call's context")
	}
}

func TestStandingStream_DetachSafely_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.Error(t, sess.DetachSafely())
}

func TestStandingStream_Attach_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	_, err := sess.Attach()
	assert.Error(t, err)
}

// --- Story 2.4.1 ---

func TestStandingStream_SetWindowSize_SendsResizeOnStandingStream(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.SetWindowSize(100, 40))

	sent := stream.sentRequests()
	require.Len(t, sent, 2) // [0] is the initial PaneId, [1] is our Resize
	resize := sent[1].GetResize()
	require.NotNil(t, resize)
	assert.Equal(t, uint32(100), resize.GetCols())
	assert.Equal(t, uint32(40), resize.GetRows())
}

// TestStandingStream_SetDetachedSize_SendsResizeOnStandingStream_EvenWithNoActiveAttachCaller
// asserts the acceptance criterion from plan.md Story 2.4.1 verbatim: a
// SetDetachedSize call still succeeds and sends Resize on the always-open
// standing stream, even though nothing here ever calls sess.Attach() (i.e.
// no active Attach() caller from stapler-squad's own UI layer) —
// the standing-stream design makes detached resize possible where tmux's
// own detached-resize needs a separate mechanism (architecture.md §1 Gap
// #3).
func TestStandingStream_SetDetachedSize_SendsResizeOnStandingStream_EvenWithNoActiveAttachCaller(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	require.NoError(t, sess.SetDetachedSize(100, 40, "title"))

	sent := stream.sentRequests()
	require.Len(t, sent, 2)
	resize := sent[1].GetResize()
	require.NotNil(t, resize)
	assert.Equal(t, uint32(100), resize.GetCols())
	assert.Equal(t, uint32(40), resize.GetRows())
}

func TestStandingStream_SetWindowSize_BeforeStart_ReturnsError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.Error(t, sess.SetWindowSize(80, 24))
}

func TestRefreshClient_AlwaysReturnsNil(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	assert.NoError(t, sess.RefreshClient())
}

// --- Epic 2.5: reconnect loop and resync ---

// setReconnectBackoff overrides a started session's ReconnectLoop backoff
// bounds for fast, deterministic tests — the production defaults
// (session.go's defaultReconnect* consts) would make these tests slow.
func setReconnectBackoff(sess TymuxManager, base, max time.Duration, maxAttempts int) *tymuxGRPCSession {
	c := sess.(*tymuxGRPCSession)
	c.mu.Lock()
	c.reconnectBaseDelay = base
	c.reconnectMaxDelay = max
	c.reconnectMaxAttempts = maxAttempts
	c.mu.Unlock()
	return c
}

// setTeardownWait overrides a session's teardownStandingStream bound (Task
// 2.3.1a) — the production default (maxTeardownWait) would make wedged-
// reader tests slow, mirroring setReconnectBackoff's convention above.
func setTeardownWait(sess TymuxManager, wait time.Duration) *tymuxGRPCSession {
	c := sess.(*tymuxGRPCSession)
	c.mu.Lock()
	c.teardownWait = wait
	c.mu.Unlock()
	return c
}

// --- Story 2.5.1 ---

func TestReconnectLoop_DoesNotFire_OnDeliberateDetach(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)

	require.NoError(t, sess.DetachSafely())
	time.Sleep(20 * time.Millisecond) // let any (incorrect) reconnect attempt start

	assert.EqualValues(t, 1, transport.attachCalls, "DetachSafely must not trigger ReconnectLoop — no second Attach call")
}

func TestReconnectLoop_Fires_OnTransportErrorNotPrecededByDetach(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 5*time.Millisecond, 3)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
		}})
		return s
	}

	close(stream.events) // drop, not preceded by DetachSafely/Close

	wait.RequireEventually(t, func() bool {
		return atomic.LoadInt32(&transport.attachCalls) >= 2
	}, time.Second, time.Millisecond, "a non-deliberate stream end must trigger ReconnectLoop (a second Attach call)")
}

// --- Story 2.5.2 ---

func TestReconnectLoop_TransparentlyReattaches_OnTransientDrop_WithoutClosingAttachChannel(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 5*time.Millisecond, 4)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{
				Liveness: v1.Liveness_LIVENESS_LIVE,
				Grid:     []*v1.Row{row(cell("R", 0, 0, 0), cell("E", 0, 0, 0), cell("D", 0, 0, 0))},
			},
		}})
		return s
	}

	done, err := sess.Attach()
	require.NoError(t, err)
	_, subCh := sess.SubscribeToControlModeUpdates()

	close(stream.events) // drop

	select {
	case <-done:
		t.Fatal("Attach's channel closed on a transient drop that should have reconnected transparently")
	case <-time.After(200 * time.Millisecond):
	}

	select {
	case got := <-subCh:
		assert.Contains(t, string(got), "RED", "expected the post-reconnect resync redraw")
	case <-time.After(time.Second):
		t.Fatal("subscriber never received the post-reconnect resync redraw")
	}

	reconnecting, _, _ := sess.ReconnectState()
	assert.False(t, reconnecting, "ReconnectState must clear once the reconnect succeeds")
}

func TestReconnectLoop_GivesUp_AfterMaxAttempts_ClosesAttachChannel_AndFiresDistinctExitReason(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 2)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.sendFn = func(*v1.AttachRequest) error { return errors.New("boom: unreachable") }
		return s
	}

	reasons := make(chan string, 1)
	sess.SetOnExitCallback(func(reason string) { reasons <- reason })

	before := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "reconnect_exhausted")

	done, err := sess.Attach()
	require.NoError(t, err)

	close(stream.events) // drop

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Attach's channel never closed after ReconnectLoop exhausted its attempts")
	}

	select {
	case reason := <-reasons:
		assert.Contains(t, reason, "reconnect failed", "the give-up reason must be distinguishable from an ordinary process exit")
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired after ReconnectLoop exhaustion")
	}

	// REQ-10: true exhaustion (closing never observed) must be recorded
	// distinctly from a deliberate-close interruption.
	after := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "reconnect_exhausted")
	assert.Equal(t, before+1, after, "session_lifecycle_ends_total{subsystem=tymux_reconnect,reason=reconnect_exhausted} must increment by exactly 1")
}

// TestReconnectLoop_NoDuplicateRenderedOutput_AcrossReconnectBoundary is
// Task 2.5.2d's regression test: force a drop while output is actively
// streaming, let ReconnectLoop reconnect, and assert ClientFanout's
// subscribers never receive the same content twice across the reconnect
// boundary — closing adversarial-review.md's subscribe-then-snapshot
// Blocker for the client-triggered-reconnect case specifically.
func TestReconnectLoop_NoDuplicateRenderedOutput_AcrossReconnectBoundary(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)
	_, subCh := sess.SubscribeToControlModeUpdates()

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("before-drop")}})
	select {
	case got := <-subCh:
		assert.Equal(t, []byte("before-drop"), got)
	case <-time.After(time.Second):
		t.Fatal("never received pre-drop output")
	}

	var second *fakeAttachStream
	var secondMu sync.Mutex
	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{
				Liveness: v1.Liveness_LIVENESS_LIVE,
				Grid:     []*v1.Row{row(cell("R", 0, 0, 0), cell("W", 0, 0, 0))},
			},
		}})
		secondMu.Lock()
		second = s
		secondMu.Unlock()
		return s
	}

	close(stream.events) // drop mid-stream

	var redraw []byte
	select {
	case redraw = <-subCh:
	case <-time.After(time.Second):
		t.Fatal("never received the post-reconnect resync redraw")
	}
	assert.Contains(t, string(redraw), "RW")

	wait.RequireEventually(t, func() bool {
		secondMu.Lock()
		defer secondMu.Unlock()
		return second != nil
	}, time.Second, time.Millisecond)
	secondMu.Lock()
	second.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("after-reconnect")}})
	secondMu.Unlock()

	select {
	case got := <-subCh:
		assert.Equal(t, []byte("after-reconnect"), got)
	case <-time.After(time.Second):
		t.Fatal("never received post-reconnect live output")
	}

	select {
	case extra := <-subCh:
		t.Fatalf("received an unexpected extra broadcast (possible duplicate across the reconnect boundary): %q", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

// --- Story 2.5.3 ---

func TestReconnectLoop_DetectsDaemonRestart_RevivesSession_AndSurfacesDistinctState(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)

	var revived int32
	transport.reviveSessionFn = func(_ context.Context, _ *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
		atomic.AddInt32(&revived, 1)
		return connect.NewResponse(&v1.ReviveSessionResponse{}), nil
	}

	var calls int32
	transport.attachFn = func(ctx context.Context) attachStream {
		n := atomic.AddInt32(&calls, 1)
		s := newFakeAttachStream(ctx)
		if n == 1 {
			s.recvErr = connect.NewError(connect.CodeFailedPrecondition, errors.New("pane exited"))
		} else {
			s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
				Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
			}})
		}
		return s
	}

	done, err := sess.Attach()
	require.NoError(t, err)

	close(stream.events) // drop

	select {
	case <-done:
		t.Fatal("Attach's channel closed; a daemon-restart reconnect should still succeed transparently")
	case <-time.After(300 * time.Millisecond):
	}

	concrete := sess.(*tymuxGRPCSession)
	wait.RequireEventually(t, func() bool {
		restarted, _ := concrete.BackendRestarted()
		return restarted
	}, time.Second, time.Millisecond, "BackendRestarted should report true after a FailedPrecondition-detected daemon restart")

	assert.EqualValues(t, 1, atomic.LoadInt32(&revived), "ReviveSession must be called exactly once for the daemon-restart case")
	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2), "expected at least 2 Attach dial attempts (dead, then revived)")
}

// TestReadAttachLoop_CleanExitThenStreamEnd_DoesNotReconnectOrRevive is the
// BLOCKER regression test (Phase 6 architecture review): tymuxd closes the
// gRPC stream itself right after sending Exited (crates/tymuxd/src/main.rs),
// so readAttachLoop's next Receive() call always errors (io.EOF in
// practice) even though the pane already exited cleanly and deliberately —
// no Close()/DetachSafely() involved. Before the fix, readAttachLoop only
// checked s.closing here, so this fell through to ReconnectLoop, whose
// reattach dial fails with errPaneDead (the pane really is dead — just not
// because tymuxd restarted), daemonRestarted evaluated true, and
// ReviveSession spawned a needless replacement process while falsely
// reporting BackendRestarted() == true. Asserts none of that happens: no
// second Attach call, ReviveSession never invoked, BackendRestarted stays
// false, and Attach()'s done channel still closes (a clean exit's stream
// end is a known, not unexpected, terminus).
func TestReadAttachLoop_CleanExitThenStreamEnd_DoesNotReconnectOrRevive(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)

	var revived int32
	transport.reviveSessionFn = func(_ context.Context, _ *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
		atomic.AddInt32(&revived, 1)
		return connect.NewResponse(&v1.ReviveSessionResponse{}), nil
	}

	done, err := sess.Attach()
	require.NoError(t, err)

	code := int32(0)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	concrete := sess.(*tymuxGRPCSession)
	wait.RequireEventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.exited
	}, time.Second, time.Millisecond, "Exited event should be observed before the stream ends")

	// Task 2.4.1b: tie the end-to-end behavioral proof above to the new
	// classifier directly — classifyStreamEnd must already report
	// ReasonCleanExit for this exact exited-before-Receive()-error
	// sequence, not just "the end-to-end behavior happens to be correct."
	liveCtx, cancelLiveCtx := context.WithCancel(context.Background())
	defer cancelLiveCtx()
	assert.Equal(t, lifecycle.ReasonCleanExit, concrete.classifyStreamEnd(liveCtx),
		"classifyStreamEnd must report ReasonCleanExit once Exited has been observed, even with a live (non-canceled) ctx")

	close(stream.events) // server closes the stream right after Exited (io.EOF)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Attach's channel never closed after a clean exit followed by stream end")
	}

	assert.EqualValues(t, 1, transport.attachCalls, "a clean exit followed by stream end must not trigger ReconnectLoop (no second Attach call)")
	assert.EqualValues(t, 0, atomic.LoadInt32(&revived), "ReviveSession must never be called after a clean exit")

	restarted, _ := concrete.BackendRestarted()
	assert.False(t, restarted, "BackendRestarted must stay false after a clean exit — tymuxd never restarted")
}

func TestReconnectLoop_OrdinaryDrop_DoesNotSetBackendRestarted(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 4)

	transport.attachFn = func(ctx context.Context) attachStream {
		s := newFakeAttachStream(ctx)
		s.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{
			Snapshot: &v1.PaneSnapshot{Liveness: v1.Liveness_LIVENESS_LIVE},
		}})
		return s
	}

	close(stream.events) // ordinary transport drop, pane stays live

	wait.RequireEventually(t, func() bool {
		return atomic.LoadInt32(&transport.attachCalls) >= 2
	}, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	concrete := sess.(*tymuxGRPCSession)
	restarted, _ := concrete.BackendRestarted()
	assert.False(t, restarted, "an ordinary transport blip must not be surfaced as a daemon restart")
}

// --- Phase 2 (session-lifecycle-state-machine): classifyStreamEnd, lifecycle wiring ---

// TestClassifyStreamEnd_AllFourOrigins is Story 2.1.1/2.4.2's direct
// table-driven unit test of classifyStreamEnd in isolation, independent of
// any readAttachLoop/RPC-mock setup.
func TestClassifyStreamEnd_AllFourOrigins(t *testing.T) {
	tests := []struct {
		name        string
		closing     bool
		exited      bool
		ctxCanceled bool
		want        lifecycle.Reason
	}{
		{
			name:        "closing wins over exited and a canceled ctx",
			closing:     true,
			exited:      true,
			ctxCanceled: true,
			want:        lifecycle.ReasonDeliberateClose,
		},
		{
			name:   "exited wins over ctx when not closing",
			exited: true,
			want:   lifecycle.ReasonCleanExit,
		},
		{
			name:        "a canceled ctx alone is a deliberate supersede",
			ctxCanceled: true,
			want:        lifecycle.ReasonDeliberateSupersede,
		},
		{
			name: "none of the three signals fired is a transport drop",
			want: lifecycle.ReasonTransportDrop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := NewTymuxGRPCSession(&fakeTransport{})
			concrete := sess.(*tymuxGRPCSession)
			concrete.closing.Store(tt.closing)
			concrete.mu.Lock()
			concrete.exited = tt.exited
			concrete.mu.Unlock()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.ctxCanceled {
				cancel()
			}

			assert.Equal(t, tt.want, concrete.classifyStreamEnd(ctx))
		})
	}
}

// TestOpenStandingStream_ReadAttachLoop_CleanExit_IncrementsEndsTotalWithCleanExitReason
// is REQ-10's happy-path observability assertion: a clean pane exit followed
// by stream end must increment session_lifecycle_ends_total with
// reason=clean_exit for subsystem=tymux_stream exactly once.
func TestOpenStandingStream_ReadAttachLoop_CleanExit_IncrementsEndsTotalWithCleanExitReason(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)

	before := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_stream", "clean_exit")

	done, err := sess.Attach()
	require.NoError(t, err)

	code := int32(0)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})
	close(stream.events) // server closes the stream right after Exited (io.EOF)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Attach's channel never closed after a clean exit followed by stream end")
	}

	wait.RequireEventually(t, func() bool {
		after := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_stream", "clean_exit")
		return after == before+1
	}, time.Second, time.Millisecond,
		"session_lifecycle_ends_total{subsystem=tymux_stream,reason=clean_exit} must increment by exactly 1")
}

// TestReconnectLoop_ClosingDuringExhaustion_RecordsDeliberateCloseNotExhausted
// is REQ-10's branch-distinction assertion: when ReconnectLoop observes
// s.closing.Load() == true (a deliberate Close()/DetachSafely()), it must
// record reason=deliberate_close, never reason=reconnect_exhausted, even
// though both share the same "gave up" outward return value.
func TestReconnectLoop_ClosingDuringExhaustion_RecordsDeliberateCloseNotExhausted(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	concrete := setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 3)
	concrete.beginClosing()

	beforeClose := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "deliberate_close")
	beforeExhausted := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "reconnect_exhausted")

	_, _, ok, reason := concrete.ReconnectLoop(context.Background(), "pane-1", "error")
	assert.False(t, ok)
	assert.Equal(t, lifecycle.ReasonDeliberateClose, reason, "ReconnectLoop must return the same Reason it recorded")

	afterClose := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "deliberate_close")
	afterExhausted := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), "tymux_reconnect", "reconnect_exhausted")

	assert.Equal(t, beforeClose+1, afterClose, "closing observed mid-loop must record deliberate_close")
	assert.Equal(t, beforeExhausted, afterExhausted, "closing observed mid-loop must not also record reconnect_exhausted")
}

// TestOpenStandingStream_TearDownForReopen_DoesNotDeadlock_WhenOldStreamWouldOtherwiseReconnect
// is incident #2's regression test: openStandingStream's own tear-down-
// before-reopen (a plain reopen, not a Close()/DetachSafely()) cancels the
// old stream's ctx without ever setting s.closing. Before classifyStreamEnd
// (Task 2.1.1a) added the ctx.Err() check, the old reader's Receive() error
// fell through to ReconnectLoop instead of exiting, and since nothing
// closes s.abortReconnect for a mere reopen, ReconnectLoop's redial blocks
// forever on its own Receive() — wedging teardownStandingStream's wait on
// the old generation's `done` and, transitively, the reopening
// Start()/RestoreWithWorkDir() call itself.
func TestOpenStandingStream_TearDownForReopen_DoesNotDeadlock_WhenOldStreamWouldOtherwiseReconnect(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)
	setReconnectBackoff(sess, time.Millisecond, 2*time.Millisecond, 50)

	dir, err := sess.GetCurrentWorkingDirectory()
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.Start(dir) // reopen: tears down the old stream first
	}()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("reopen deadlocked waiting for the old reader goroutine to exit — its ctx cancellation was misclassified as a reconnectable drop")
	}

	assert.EqualValues(t, 2, atomic.LoadInt32(&transport.attachCalls),
		"reopen must open exactly one new Attach stream, not trigger the old reader's ReconnectLoop")
}

// TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged
// is incident #3's regression test: a reader goroutine whose Receive()
// never returns, even once its stream's ctx is canceled (a transport that
// doesn't honor context cancellation) — the case classifyStreamEnd's
// ctx.Err() check cannot help with, since the old reader never gets back
// into readAttachLoop's error-handling branch at all. teardownStandingStream
// must still abandon it and let the reopen proceed, bounded by
// s.teardownWait via lifecycle.AwaitBounded (Task 2.3.1a) — and the
// abandoned generation's session_lifecycle_active_generations slot must
// stay elevated, since its EndGeneration is never reached (Task 2.2.2a,
// REQ-11).
func TestOpenStandingStream_TearDownForReopen_ProceedsAnyway_WhenOldReaderIsWedged(t *testing.T) {
	dir := t.TempDir()
	wedged := &wedgedAttachStream{sent: make(chan *v1.AttachRequest, 4)}
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	transport.attachFn = func(ctx context.Context) attachStream { return wedged }

	sess := NewTymuxGRPCSession(transport)
	setTeardownWait(sess, 50*time.Millisecond)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })

	before := sumForSubsystem(t, collectMetric(t, "session_lifecycle_active_generations"), "tymux_stream", "")

	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.Start(dir) // reopen: old reader is wedged in wedged.Receive()
	}()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("reopen did not proceed within 2s despite teardownWait bounding the wedged old reader's wait")
	}

	assert.EqualValues(t, 2, atomic.LoadInt32(&transport.attachCalls),
		"reopen must still open a fresh Attach stream despite the old reader being wedged")

	wait.RequireEventually(t, func() bool {
		after := sumForSubsystem(t, collectMetric(t, "session_lifecycle_active_generations"), "tymux_stream", "")
		return after == before+1
	}, time.Second, time.Millisecond,
		"the abandoned generation must stay counted as active — its EndGeneration is never reached")
}

// TestClampResizeDim is the regression test for the gosec G115 fix.
// clampResizeDim floors negative input at 0 (unlike
// session/tmux.ClampWinsizeDim, which floors at 1 -- a resize dimension of 0
// is a legitimate signal here, not an invalid pty size) and caps at
// math.MaxUint32 rather than wrapping through the int -> uint32 narrowing
// conversion.
func TestClampResizeDim(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want uint32
	}{
		{name: "negative floors to zero, not one", in: -5, want: 0},
		{name: "zero passes through", in: 0, want: 0},
		{name: "normal value passes through", in: 80, want: 80},
		{name: "MaxUint32 boundary passes through", in: math.MaxUint32, want: math.MaxUint32},
		{name: "above MaxUint32 clamps to MaxUint32", in: math.MaxUint32 + 1, want: math.MaxUint32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, clampResizeDim(tt.in))
		})
	}
}
