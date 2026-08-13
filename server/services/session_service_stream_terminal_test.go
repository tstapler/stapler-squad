package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// newBidiStreamTestServer creates a TLS+HTTP/2 httptest server. Connect's
// bidirectional streaming (used by StreamTerminal) requires full-duplex
// HTTP/2; the plain HTTP/1.1 server used by newNotificationTestServer only
// supports unary and server-streaming RPCs.
func newBidiStreamTestServer(t *testing.T) (*SessionService, *httptest.Server) {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })

	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return svc, srv
}

// TestStreamTerminal_SendsRawOutput exercises the simplified StreamTerminal
// output path (server/services/session_service.go) end-to-end against a real
// tmux-backed session, and asserts that bytes read from the PTY arrive at the
// client as a TerminalData_Output message (never any other oneof variant).
// This closes the coverage gap identified in the BUG-025 review: the only
// prior tests touching StreamTerminal
// (BenchmarkSessionService_StreamTerminal_NotFound/NotStarted) short-circuit
// before reaching the PTY-forwarding goroutine.
//
// The PTY handle StreamTerminal reads from (instance.GetPTYReader) is shared
// with the instance's own internal consumers (response stream, command
// executor, claude controller), which are already reading it concurrently by
// the time this RPC attaches. That makes the exact bytes/timing of any given
// read nondeterministic, so this test asserts only on the message shape
// (always Output, never any other variant) rather than specific echoed
// content.
func TestStreamTerminal_SendsRawOutput(t *testing.T) {
	svc, srv := newBidiStreamTestServer(t)

	statusMgr := session.NewInstanceStatusManager()
	queue := session.NewReviewQueue()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)
	svc.SetStatusManager(statusMgr)

	// Wire the actor registry exactly as production does (server/dependencies.go):
	// without it, CreateSession never wraps the new Instance in a LiveInstance, so
	// every actor-routed mutation (transitionToLocked, UpdateTerminalTimestamps,
	// etc.) silently falls back to running synchronously on the calling goroutine
	// with no cross-goroutine synchronization at all — a real -race finding in
	// this test, caused by the test omitting production wiring rather than by any
	// bug in the actor itself.
	registry := session.NewRegistry(nil, svc.WireInstanceCallbacks)
	svc.SetRegistry(registry)

	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	resp, err := client.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "stream-term-" + uuid.New().String()[:8],
		Path:    t.TempDir(),
		Program: "bash",
	}))
	require.NoError(t, err)
	sessionID := resp.Msg.Session.Id
	t.Cleanup(func() { destroyCreatedSession(t, svc, sessionID) })

	inst := svc.FindLiveInstance(sessionID)
	require.NotNil(t, inst, "instance must appear in live poller immediately after CreateSession")

	// Poll until the session has actually started a real tmux PTY, or skip if
	// tmux is unavailable in this environment (mirrors
	// TestCreateSession_StatusManagerWiredBeforeDriver's convention).
	var started bool
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if inst.Started() {
			started = true
			break
		}
		if session.Status(inst.GetStatus()) == session.Stopped {
			t.Skip("tmux not available; skipping StreamTerminal raw-output assertion")
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, started, "session never started within 60s")

	// 120s, not 60s: every tmux subprocess this test's stimulus goroutine and
	// the instance's own internal consumers spawn queues behind the same
	// 8-slot exec gate (session/tmux/exec_gate.go's AcquireExecSlot) shared
	// by every other tmux-backed test in this package. Under full-suite load
	// (dozens of concurrent real-tmux tests) that queueing can push PTY
	// output well past a 60s budget even though nothing is actually stuck —
	// see the exec-gate flake this widened window is meant to absorb.
	streamCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	stream := client.StreamTerminal(streamCtx)
	require.NoError(t, stream.Send(&sessionv1.TerminalData{SessionId: sessionID}))
	t.Cleanup(func() { _ = stream.CloseRequest() })

	// Actively provoke fresh PTY output instead of passively waiting for
	// whatever's left of bash's one-time startup burst (motd/prompt). The PTY
	// fd this goroutine reads from is shared with the instance's own internal
	// consumers (response stream, command executor, claude controller), which
	// are already reading it concurrently -- a real fd-level race for who
	// gets each byte, not a broadcast. Under a fast/idle CI runner there's
	// normally still some of that startup burst in flight by the time this
	// stream attaches, so the passive wait usually won by luck; but under
	// heavy load CreateSession's own polling loop above can take much longer,
	// during which the other consumers fully drain that one-time burst,
	// leaving a bare interactive bash shell that emits nothing further on its
	// own. That's what produced CI run 29549848133's exact failure: 60.15s
	// (the streamCtx deadline) with zero Output frames ever received.
	// Sending a newline repeatedly makes bash re-prompt for as long as it
	// takes, giving this reader many independent chances to win the race
	// instead of exactly one (the leftover startup burst, or nothing).
	stimulusDone := make(chan struct{})
	var stimulusWG sync.WaitGroup
	stimulusWG.Add(1)
	go func() {
		defer stimulusWG.Done()
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-streamCtx.Done():
				return
			case <-stimulusDone:
				return
			case <-ticker.C:
				_ = stream.Send(&sessionv1.TerminalData{
					SessionId: sessionID,
					Data: &sessionv1.TerminalData_Input{
						Input: &sessionv1.TerminalInput{Data: []byte("\n")},
					},
				})
			}
		}
	}()

	var gotOutput bool
	for {
		msg, recvErr := stream.Receive()
		if recvErr != nil {
			break
		}
		switch data := msg.Data.(type) {
		case *sessionv1.TerminalData_Output:
			require.NotEmpty(t, data.Output.Data, "TerminalData_Output must carry the raw PTY bytes that were read")
			gotOutput = true
		default:
			t.Fatalf("StreamTerminal's simplified raw-output path must only ever emit TerminalData_Output, got %T", data)
		}
		if gotOutput {
			break
		}
	}
	close(stimulusDone)
	stimulusWG.Wait()

	require.True(t, gotOutput, "expected at least one TerminalData_Output frame from the live PTY")
}

// TestWaitWithTimeout pins waitWithTimeout's two branches directly, without
// depending on tmux or the e2e StreamTerminal path above.
func TestWaitWithTimeout(t *testing.T) {
	t.Run("returns true when goroutines finish in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done() }()
		require.True(t, waitWithTimeout(&wg, time.Second))
	})

	t.Run("returns false when goroutines don't finish in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1) // deliberately never Done()
		require.False(t, waitWithTimeout(&wg, 10*time.Millisecond))
	})
}
