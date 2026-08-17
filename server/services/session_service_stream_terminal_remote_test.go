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

	cmd := exec.CommandContext(s.Context(), "sh", "-c", raw)

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
