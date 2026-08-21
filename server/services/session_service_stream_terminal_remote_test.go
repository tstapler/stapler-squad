package services

// session_service_stream_terminal_remote_test.go is the regression test for
// the review-found VIOLATION in StreamTerminal's raw-PTY-fallback path
// (ssh-remote-workspaces Phase 4, Task 4.4.1d): the input-forwarding
// goroutine used to call instance.WriteToPTY(data.Input.Data) unconditionally
// for both local and remote sessions. instance.WriteToPTY routes to
// TmuxSession.SendKeys -> t.lockedPTMX(), which is only ever populated by a
// LOCAL raw-PTY attach -- never for a remote session -- so the very first
// client keystroke sent through this path against a remote session returned
// "PTY not initialized", the handler sent a WRITE_ERROR, and the whole
// stream tore down. This proves the fix (branch on remotePTY != nil and call
// remotePTY.Write instead) against a real remote session: real in-process
// test sshd + real remote tmux server, same conventions as
// session_service_remote_test.go.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/creack/pty"
	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
)

// realExecWithPtySessionSSHHandler is realExecSessionSSHHandler
// (session_service_remote_test.go) extended to actually honor a client's
// pty-req: when one is present, it allocates a REAL OS pty (github.com/
// creack/pty, the same library session/tmux/pty.go's local PtyFactory uses)
// and runs the exec'd command attached to it, copying bytes between the SSH
// channel and the pty master in both directions and propagating
// window-change notifications via pty.Setsize. realExecSessionSSHHandler
// itself never does this (CreateSession's own remote commands -- git,
// SSHRunner.Run/Start -- never request a pty, per
// session/tmux/tmux_remote_test.go's TestNewSessionA_AgainstExistingSession_
// FailsOverNonPTYChannel), so a real "tmux attach-session" (no -C) run
// against a plain-piped, non-tty stdin/stdout would fail termios/isatty
// checks and never forward keystrokes into the pane -- exactly the gap this
// handler closes, so this file's PTY round-trip test exercises genuine PTY
// semantics, not just a channel-open no-op.
func realExecWithPtySessionSSHHandler(s gliderssh.Session) {
	raw := s.RawCommand()
	if raw == "" {
		_ = s.Exit(0)
		return
	}

	cmd := safeexec.CommandContext(s.Context(), "sh", "-c", raw)

	if ptyReq, winCh, isPty := s.Pty(); isPty {
		cmd.Env = append(cmd.Environ(), "TERM="+ptyReq.Term)
		f, err := pty.StartWithSize(cmd, &pty.Winsize{
			Rows: uint16(ptyReq.Window.Height),
			Cols: uint16(ptyReq.Window.Width),
		})
		if err != nil {
			_, _ = fmt.Fprintf(s.Stderr(), "pty start error: %v\n", err)
			_ = s.Exit(1)
			return
		}

		// resizeDone signals that the window-change-handling goroutine below
		// has stopped touching f, so f.Close() below never races
		// pty.Setsize's read of f's fd against os.File.Close()'s write to it
		// (caught by -race in an earlier version of this handler that used a
		// deferred Close alongside an unbounded "for win := range winCh"
		// goroutine with no join point).
		resizeDone := make(chan struct{})
		go func() {
			defer close(resizeDone)
			for {
				select {
				case win, ok := <-winCh:
					if !ok {
						return
					}
					_ = pty.Setsize(f, &pty.Winsize{Rows: uint16(win.Height), Cols: uint16(win.Width)})
				case <-s.Context().Done():
					return
				}
			}
		}()

		go func() { _, _ = io.Copy(f, s) }()
		_, _ = io.Copy(s, f)
		_ = cmd.Wait()
		<-resizeDone
		_ = f.Close()
		_ = s.Exit(0)
		return
	}

	cmd.Stdin = s
	cmd.Stdout = s
	cmd.Stderr = s.Stderr()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_ = s.Exit(exitErr.ExitCode())
			return
		}
		_, _ = fmt.Fprintf(s.Stderr(), "exec error: %v\n", err)
		_ = s.Exit(1)
		return
	}
	_ = s.Exit(0)
}

// newBidiStreamTestServerForSvc is session_service_stream_terminal_test.go's
// newBidiStreamTestServer, parameterized on an already-constructed
// *SessionService instead of building its own -- this test needs
// remoteSessionFixture's svc (wired with SetRemoteDeps), not a bare
// NewSessionService(storage, bus).
func newBidiStreamTestServerForSvc(t *testing.T, svc *SessionService) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)

	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// remoteCapturePaneContains runs "tmux capture-pane" on srv over a SECOND,
// independent SSH dial -- mirrors remoteHasSessionViaIndependentDial's own
// "verified via a second, independent SSH dial" convention -- and reports
// whether the captured pane text contains marker.
func remoteCapturePaneContains(t *testing.T, srv *remoteSessionTestSSHServer, sessionName, serverSocket, marker string) bool {
	t.Helper()
	client, err := ssh.Dial("tcp", srv.Addr, &ssh.ClientConfig{
		User:            "testuser",
		Auth:            []ssh.AuthMethod{remoteSessionTestClientAuth(t)},
		HostKeyCallback: ssh.FixedHostKey(srv.HostKey),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		return false
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return false
	}
	defer sess.Close()

	cmd := "tmux capture-pane -p -t " + sessionName
	if serverSocket != "" {
		cmd = "tmux -L " + serverSocket + " capture-pane -p -t " + sessionName
	}
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), marker)
}

func TestStreamTerminal_RemoteSession_InputReachesRemotePTY(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts a real remote tmux session")
	}
	srv := startRemoteSessionTestSSHServerWithHandler(t, realExecWithPtySessionSSHHandler, nil)
	fix := newRemoteSessionFixture(t, srv)

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "remote-stream-input",
		Path:  fix.repoPath,
		// "cat" echoes stdin straight back to the pane, so a successful write
		// is guaranteed to also produce visible output, independent of any
		// real shell's own line-editing/echo behavior.
		Branch:      "feature-input",
		Program:     "cat",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	sessionID := resp.Msg.Session.Id

	sessionName := "staplersquad_remote-stream-input"
	require.Eventually(t, func() bool {
		return remoteHasSessionViaIndependentDial(t, srv, sessionName, fix.svc.testTmuxServerSocket)
	}, 10*time.Second, 200*time.Millisecond, "remote tmux session must exist before streaming")

	var found *session.Instance
	require.Eventually(t, func() bool {
		for _, inst := range fix.poller.GetInstances() {
			if inst.Title == "remote-stream-input" {
				found = inst
				return inst.Started()
			}
		}
		return false
	}, 15*time.Second, 100*time.Millisecond, "created remote instance must appear in the poller and report Started()")
	require.True(t, found.ExecutionTarget != nil && found.ExecutionTarget.IsRemote(),
		"sanity check: this test only proves the fix if the instance is actually remote")

	httpSrv := newBidiStreamTestServerForSvc(t, fix.svc)
	client := sessionv1connect.NewSessionServiceClient(httpSrv.Client(), httpSrv.URL)

	streamCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := client.StreamTerminal(streamCtx)
	require.NoError(t, stream.Send(&sessionv1.TerminalData{SessionId: sessionID}))
	t.Cleanup(func() { _ = stream.CloseRequest() })

	// Drain Receive() in the background so a WRITE_ERROR (the pre-fix
	// symptom: instance.WriteToPTY failing with "PTY not initialized" for a
	// remote session) fails the test loudly and immediately, instead of only
	// being inferred from capture-pane never seeing the marker.
	recvErrCh := make(chan string, 1)
	go func() {
		for {
			msg, recvErr := stream.Receive()
			if recvErr != nil {
				return
			}
			if errData, ok := msg.Data.(*sessionv1.TerminalData_Error); ok {
				select {
				case recvErrCh <- errData.Error.Code + ": " + errData.Error.Message:
				default:
				}
			}
		}
	}()

	const marker = "REMOTE_INPUT_MARKER"
	require.NoError(t, stream.Send(&sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_Input{
			Input: &sessionv1.TerminalInput{Data: []byte(marker)},
		},
	}))

	select {
	case errMsg := <-recvErrCh:
		t.Fatalf("StreamTerminal returned an error for remote input (this is the pre-fix symptom -- instance.WriteToPTY called unconditionally instead of remotePTY.Write): %s", errMsg)
	case <-time.After(500 * time.Millisecond):
		// No immediate error; proceed to the authoritative check below.
	}

	require.Eventually(t, func() bool {
		return remoteCapturePaneContains(t, srv, sessionName, fix.svc.testTmuxServerSocket, marker)
	}, 10*time.Second, 200*time.Millisecond,
		"input sent via StreamTerminal must reach the remote tmux pane (proving remotePTY.Write was used, not the local-only instance.WriteToPTY path)")
}

// TestStreamTerminal_RemoteSession_RecoversAfterConnectionKilledMidStream
// covers the gap flagged in review of ssh-remote-workspaces AC3 ("including
// recovery after a network partition"): no prior test (unit or e2e) ever
// killed the underlying SSH connection while a remote terminal stream was
// actively flowing and then verified recovery -- this repo's e2e spec
// (tests/e2e/remote-workspaces.spec.ts) explicitly disclaims that as out of
// its budget, and this file's other tests never disconnect mid-stream.
//
// "Recovery" here mirrors how a LOCAL session's terminal recovers from any
// disruption too: StreamTerminal has no server-side auto-reconnect of its
// own (see the output goroutine's remote branch above -- a read error just
// ends the goroutine); recovery is the client noticing its stream ended and
// calling StreamTerminal again. This test proves the piece that's actually
// new/remote-specific: a FRESH StreamTerminal call after the SSH connection
// dies redials through SSHClientPool (session/tmux/ssh_pool.go's
// GetOrDial/evictIfCurrent) and reaches the SAME still-running remote tmux
// session again, rather than getting permanently stuck handing back a dead
// connection.
func TestStreamTerminal_RemoteSession_RecoversAfterConnectionKilledMidStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts a real remote tmux session")
	}
	srv := startRemoteSessionTestSSHServerWithHandler(t, realExecWithPtySessionSSHHandler, nil)
	fix := newRemoteSessionFixture(t, srv)

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "remote-stream-recovery",
		Path:        fix.repoPath,
		Branch:      "feature-recovery",
		Program:     "cat",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Remote:      &sessionv1.RemoteTarget{RemoteName: "test-remote"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	sessionID := resp.Msg.Session.Id

	sessionName := "staplersquad_remote-stream-recovery"
	require.Eventually(t, func() bool {
		return remoteHasSessionViaIndependentDial(t, srv, sessionName, fix.svc.testTmuxServerSocket)
	}, 10*time.Second, 200*time.Millisecond, "remote tmux session must exist before streaming")

	require.Eventually(t, func() bool {
		for _, inst := range fix.poller.GetInstances() {
			if inst.Title == "remote-stream-recovery" {
				return inst.Started()
			}
		}
		return false
	}, 15*time.Second, 100*time.Millisecond, "created remote instance must appear in the poller and report Started()")

	httpSrv := newBidiStreamTestServerForSvc(t, fix.svc)
	client := sessionv1connect.NewSessionServiceClient(httpSrv.Client(), httpSrv.URL)

	// First stream: prove it is actively flowing before the connection dies.
	streamCtx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	stream1 := client.StreamTerminal(streamCtx1)
	t.Cleanup(func() { _ = stream1.CloseRequest() })
	// Drain Receive() in the background for the rest of the test -- mirrors
	// TestStreamTerminal_RemoteSession_InputReachesRemotePTY's recvErrCh
	// pattern above. Required, not cosmetic: without a goroutine actually
	// reading stream1 to its end, the underlying HTTP/2 stream/connection
	// never fully releases on the connect-go client side even after
	// cancel1() -- confirmed the hard way: an earlier version of this test
	// only called cancel1() and left stream1 completely undrained, which
	// left its TLS connection reporting "active" to httptest.Server.Close()
	// in this test's own t.Cleanup for the rest of the process's life,
	// wedging the whole test past go test's timeout despite every actual
	// assertion in the test body having already passed.
	go func() {
		for {
			if _, recvErr := stream1.Receive(); recvErr != nil {
				return
			}
		}
	}()
	require.NoError(t, stream1.Send(&sessionv1.TerminalData{SessionId: sessionID}))

	const markerBeforeKill = "REMOTE_RECOVERY_BEFORE_KILL"
	require.NoError(t, stream1.Send(&sessionv1.TerminalData{
		SessionId: sessionID,
		Data:      &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte(markerBeforeKill)}},
	}))
	require.Eventually(t, func() bool {
		return remoteCapturePaneContains(t, srv, sessionName, fix.svc.testTmuxServerSocket, markerBeforeKill)
	}, 10*time.Second, 200*time.Millisecond, "stream must be actively flowing before the connection is killed")

	// Kill the underlying SSH connection out from under the active stream --
	// the "network partition" this AC calls for. Peek/Close (rather than
	// e.g. stopping the test sshd server) targets exactly the shared
	// *ssh.Client this session's SSHRunner/pool entry holds, without tearing
	// down the test server itself (a later reconnect must succeed against
	// the SAME still-running remote tmux session).
	pool := fix.svc.sshClientPool()
	deadClient, ok := pool.Peek("test-remote")
	require.True(t, ok, "the remote's SSH connection must be pooled at this point")
	require.NoError(t, deadClient.Close())

	// stream1 is now attached to a dead connection; a real client would
	// notice (Send/Receive erroring, or its own liveness signal) and give
	// up rather than wait forever -- mirrored here by cancelling it
	// directly instead of asserting on the exact error golang.org/x/crypto/
	// ssh surfaces for a force-closed channel (EOF vs. a wrapped net error
	// is transport-internal, not part of this AC's contract).
	cancel1()

	// Wait for the pool's own internal Client.Wait() watcher
	// (session/tmux/ssh_pool.go's register/evictIfCurrent) to actually
	// evict deadClient BEFORE attempting reconnection. Skipping this and
	// racing a reconnect attempt against the not-yet-evicted entry is not
	// merely flaky: SSHRunner.newSession's own doc comment records that a
	// channel-open request in flight when its caller's ctx expires is NOT
	// forcibly unblocked server-side -- it keeps running "until it
	// completes or the underlying connection dies on its own" -- so a
	// channel-open attempted against an already-dead-but-not-yet-evicted
	// client can leave a permanently blocked server goroutine holding its
	// HTTP/2 connection "active" for the rest of the process's life. That
	// is exactly what an earlier version of this test hit: repeated
	// pre-eviction reconnect attempts each leaked one such goroutine,
	// which then wedged httptest.Server.Close() in this test's own
	// t.Cleanup indefinitely ("httptest.Server blocked in Close... TLS
	// conn active"), which in turn wedged the whole test past go test's
	// timeout. Waiting for eviction first removes the race by
	// construction: every reconnect attempt below is guaranteed to see an
	// empty pool entry and therefore genuinely redial.
	require.Eventually(t, func() bool {
		_, stillPooled := pool.Peek("test-remote")
		return !stillPooled
	}, 10*time.Second, 50*time.Millisecond, "pool must evict the dead client before a reconnect can succeed")

	// Recovery: a FRESH StreamTerminal call must eventually succeed again --
	// proving GetPTYSession/SSHRunner redial through the pool now that the
	// dead entry is gone, rather than a fresh call also getting stuck on
	// the same dead client forever.
	const markerAfterRecovery = "REMOTE_RECOVERY_AFTER_KILL"
	require.Eventually(t, func() bool {
		streamCtx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		stream2 := client.StreamTerminal(streamCtx2)
		// Drain in the background so this attempt's connection resources
		// release properly regardless of outcome -- not just the winning
		// attempt needs this: stream1's identical requirement above (an
		// undrained stream leaves its TLS connection permanently "active"
		// from httptest.Server.Close()'s perspective) applies to every
		// attempt this loop makes, not only the last one.
		go func() {
			for {
				if _, recvErr := stream2.Receive(); recvErr != nil {
					return
				}
			}
		}()
		if sendErr := stream2.Send(&sessionv1.TerminalData{SessionId: sessionID}); sendErr != nil {
			return false
		}
		if sendErr := stream2.Send(&sessionv1.TerminalData{
			SessionId: sessionID,
			Data:      &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte(markerAfterRecovery)}},
		}); sendErr != nil {
			return false
		}
		_ = stream2.CloseRequest()
		time.Sleep(300 * time.Millisecond) // let the write land before checking the pane
		return remoteCapturePaneContains(t, srv, sessionName, fix.svc.testTmuxServerSocket, markerAfterRecovery)
	}, 20*time.Second, 500*time.Millisecond,
		"a fresh StreamTerminal call after the connection was killed mid-stream must eventually reach the remote pane again (recovery)")

	// The pool must be pointing at a genuinely new connection, not just
	// coincidentally succeeding against the same (already-dead) client
	// object.
	newClient, ok := pool.Peek("test-remote")
	require.True(t, ok, "pool must hold a freshly redialed client after recovery")
	require.NotSame(t, deadClient, newClient, "recovery must have redialed a NEW *ssh.Client, not reused the killed one")
}
