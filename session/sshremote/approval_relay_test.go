package sshremote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// streamLocalOpenMsg mirrors golang.org/x/crypto/ssh's unexported
// streamLocalChannelOpenDirectMsg wire layout -- an SSH string (the target
// socket path) followed by two reserved fields, per
// openssh-portable/PROTOCOL §2.4 -- so this test server's channel handler
// can decode what a real client.Dial("unix", path) encodes into a
// direct-streamlocal@openssh.com channel-open request's ExtraData. Field
// NAMES don't matter for the wire format (ssh.Marshal/Unmarshal encode by
// field order and Go kind via reflection); they only need to be exported so
// ssh.Unmarshal (called from this different package) can set them.
type streamLocalOpenMsg struct {
	SocketPath string
	Reserved0  string
	Reserved1  uint32
}

// directStreamlocalHandler is a gliderssh.ChannelHandler for
// "direct-streamlocal@openssh.com". gliderlabs/ssh only ships a built-in
// handler for direct-tcpip (DirectTCPIPHandler); this mirrors that
// handler's exact accept-then-bidirectional-copy shape
// (github.com/gliderlabs/ssh@v0.3.8/tcpip.go) but dials a Unix domain
// socket instead of TCP -- faithfully simulating what a real sshd does for
// direct-streamlocal: connect, as a client, to the socket path the
// channel-open request names, then relay bytes between that connection and
// the new SSH channel. This is what makes RemoteApprovalRelay's
// client.Dial("unix", socketPath) actually reach a real net.Listener at
// socketPath in these tests, exactly as it would reach a real sshd-side
// Unix socket connect() on an actual remote host.
func directStreamlocalHandler(_ *gliderssh.Server, _ *ssh.ServerConn, newChan ssh.NewChannel, _ gliderssh.Context) {
	var msg streamLocalOpenMsg
	if err := ssh.Unmarshal(newChan.ExtraData(), &msg); err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, "error parsing streamlocal data: "+err.Error())
		return
	}

	var dialer net.Dialer
	dconn, err := dialer.Dial("unix", msg.SocketPath)
	if err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		_ = dconn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)

	go func() {
		defer ch.Close()
		defer dconn.Close()
		_, _ = io.Copy(ch, dconn)
	}()
	go func() {
		defer ch.Close()
		defer dconn.Close()
		_, _ = io.Copy(dconn, ch)
	}()
}

// approvalRelayTestServer is a minimal in-process SSH server supporting
// direct-streamlocal channel opens, standing in for a real sshd on the
// "remote host" side of RemoteApprovalRelay's tests. Deliberately a
// separate, trimmed-down helper rather than a reuse of
// session/tmux/ssh_test_server_test.go's testSSHServer: that file's helpers
// are unexported and live in a _test.go file in a different package
// (tmux), so this package can't import them.
type approvalRelayTestServer struct {
	Addr    string
	HostKey ssh.PublicKey
}

// startApprovalRelayTestServer starts a testSSHServer listening on an
// OS-assigned loopback port, torn down automatically via t.Cleanup.
func startApprovalRelayTestServer(t *testing.T) *approvalRelayTestServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("build host signer: %v", err)
	}

	srv := &gliderssh.Server{
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"direct-streamlocal@openssh.com": directStreamlocalHandler,
		},
	}
	srv.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &approvalRelayTestServer{Addr: ln.Addr().String(), HostKey: hostSigner.PublicKey()}
}

// testClientAuth returns an ssh.AuthMethod for a throwaway client keypair --
// the test server's PublicKeyHandler accepts any key, so the key itself is
// never verified; it exists only so the client completes SSH's publickey
// auth flow.
func testClientAuth(t *testing.T) ssh.AuthMethod {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("build client signer: %v", err)
	}
	return ssh.PublicKeys(signer)
}

func testClientConfig(t *testing.T, hostKey ssh.PublicKey) ssh.ClientConfig {
	t.Helper()
	return ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}
}

// writeApprovalOnce simulates the remote agent hook script (Epic 5.2, not
// this package): it listens on socketPath, accepts exactly one connection,
// writes payload as JSON, and closes. Returns a channel closed once the
// write attempt (successful or not) completes, and the error (if any).
func writeApprovalOnce(t *testing.T, socketPath string, payload relayedApprovalPayload) <-chan error {
	t.Helper()
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", socketPath, err)
	}
	resultCh := make(chan error, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			resultCh <- err
			return
		}
		defer conn.Close()
		resultCh <- json.NewEncoder(conn).Encode(payload)
	}()
	return resultCh
}

func newTestPool(t *testing.T) *tmux.SSHClientPool {
	t.Helper()
	return tmux.NewSSHClientPool()
}

func poolTargetFor(t *testing.T, srv *approvalRelayTestServer, name string) (tmux.SSHTarget, ssh.ClientConfig) {
	t.Helper()
	return tmux.SSHTarget{Name: name, Addr: srv.Addr}, testClientConfig(t, srv.HostKey)
}

// waitForPendingApproval polls monitor.GetPendingApprovals(sessionKey)
// until it contains an entry with the given requestID or the deadline
// elapses, returning the matching request (or nil on timeout).
func waitForPendingApproval(t *testing.T, monitor *session.ExternalApprovalMonitor, sessionKey, requestID string, timeout time.Duration) *detection.ApprovalRequest {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, req := range monitor.GetPendingApprovals(sessionKey) {
			if req.ID == requestID {
				return req
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func TestRemoteApprovalSocketPath(t *testing.T) {
	tests := []struct {
		basePath string
		want     string
	}{
		{"/home/agent/work", "/home/agent/work/.stapler-squad-approval.sock"},
		{"/home/agent/work/", "/home/agent/work/.stapler-squad-approval.sock"},
	}
	for _, tt := range tests {
		if got := RemoteApprovalSocketPath(tt.basePath); got != tt.want {
			t.Errorf("RemoteApprovalSocketPath(%q) = %q, want %q", tt.basePath, got, tt.want)
		}
	}
}

func TestNewRemoteApprovalRelay_ValidatesArgs(t *testing.T) {
	pool := newTestPool(t)
	monitor := session.NewExternalApprovalMonitor()

	tests := []struct {
		name       string
		pool       *tmux.SSHClientPool
		monitor    *session.ExternalApprovalMonitor
		remoteName string
		basePath   string
		sessionKey string
	}{
		{"nil pool", nil, monitor, "r", "/base", "k"},
		{"nil monitor", pool, nil, "r", "/base", "k"},
		{"empty remoteName", pool, monitor, "", "/base", "k"},
		{"empty basePath", pool, monitor, "r", "", "k"},
		{"empty sessionKey", pool, monitor, "r", "/base", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := RemoteApprovalRelayTarget{RemoteName: tt.remoteName, BasePath: tt.basePath, SessionKey: tt.sessionKey, Title: "title"}
			if _, err := NewRemoteApprovalRelay(tt.pool, tt.monitor, target); err == nil {
				t.Error("NewRemoteApprovalRelay() error = nil, want a validation error")
			}
		})
	}
}

// TestRemoteApprovalRelay_ForwardsPayloadToMonitor is Task 5.1.1e's
// integration test: a test payload matching detection.ApprovalRequest's
// shape, written to the remote Unix socket, must reach
// ExternalApprovalMonitor.GetPendingApprovals(sessionKey) within a bounded
// poll interval.
func TestRemoteApprovalRelay_ForwardsPayloadToMonitor(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "forward-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	monitor := session.NewExternalApprovalMonitor()
	monitor.Start()
	defer monitor.Stop()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, monitor, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, SessionKey: "session-key", Title: "Test Remote Session"}, withPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, expiresAt := relay.BearerToken()
	if token == "" {
		t.Fatal("BearerToken() returned an empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("BearerToken() returned an already-expired credential")
	}

	socketPath := RemoteApprovalSocketPath(basePath)
	payload := relayedApprovalPayload{
		Token: token,
		Request: detection.ApprovalRequest{
			ID:           "req-1",
			Type:         detection.ApprovalCommand,
			DetectedText: "rm -rf /",
			Confidence:   0.87,
		},
	}
	writeErrCh := writeApprovalOnce(t, socketPath, payload)

	got := waitForPendingApproval(t, monitor, "session-key", "req-1", 5*time.Second)
	if got == nil {
		t.Fatal("relayed approval request never became visible via GetPendingApprovals")
	}
	if got.DetectedText != "rm -rf /" {
		t.Errorf("DetectedText = %q, want %q", got.DetectedText, "rm -rf /")
	}
	if got.Confidence != 0.87 {
		t.Errorf("Confidence = %v, want 0.87", got.Confidence)
	}

	select {
	case err := <-writeErrCh:
		if err != nil {
			t.Fatalf("writing the test payload failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("test payload write never completed")
	}
}

// TestRemoteApprovalRelay_RejectsWrongBearerToken verifies a payload with
// the wrong token is dropped without ever reaching the monitor -- Task
// 5.1.1d's requirement that the relay not act as an open channel any
// process on the remote host could forge traffic into.
func TestRemoteApprovalRelay_RejectsWrongBearerToken(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "reject-token-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	monitor := session.NewExternalApprovalMonitor()
	monitor.Start()
	defer monitor.Stop()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, monitor, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, SessionKey: "session-key", Title: "Test"}, withPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	socketPath := RemoteApprovalSocketPath(basePath)
	payload := relayedApprovalPayload{
		Token:   "not-the-real-token",
		Request: detection.ApprovalRequest{ID: "req-forged", Type: detection.ApprovalCommand, DetectedText: "evil"},
	}
	<-writeApprovalOnce(t, socketPath, payload)

	if got := waitForPendingApproval(t, monitor, "session-key", "req-forged", 500*time.Millisecond); got != nil {
		t.Fatal("a payload with the wrong bearer token was forwarded into the monitor")
	}
}

// TestRemoteApprovalRelay_RejectsExpiredBearerToken verifies a payload
// bearing an expired token is rejected even though the token VALUE is
// correct.
func TestRemoteApprovalRelay_RejectsExpiredBearerToken(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "reject-expired-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	monitor := session.NewExternalApprovalMonitor()
	monitor.Start()
	defer monitor.Stop()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(
		pool, monitor,
		RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, SessionKey: "session-key", Title: "Test"},
		withPollInterval(20*time.Millisecond),
		withBearerTokenTTL(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	time.Sleep(50 * time.Millisecond) // let the short TTL elapse

	socketPath := RemoteApprovalSocketPath(basePath)
	payload := relayedApprovalPayload{
		Token:   token,
		Request: detection.ApprovalRequest{ID: "req-expired", Type: detection.ApprovalCommand, DetectedText: "cmd"},
	}
	<-writeApprovalOnce(t, socketPath, payload)

	if got := waitForPendingApproval(t, monitor, "session-key", "req-expired", 500*time.Millisecond); got != nil {
		t.Fatal("a payload with an expired bearer token was forwarded into the monitor")
	}
}

// TestRemoteApprovalRelay_DialTimeout_DoesNotWedgePollLoop exercises
// withDialTimeout: a remote-side connection that accepts but never writes
// (simulating a stalled handshake/hung write on the real remote host) must
// be abandoned within the configured timeout rather than blocking the
// relay's single poll goroutine forever -- proven here not by inspecting
// decodePayload's internals directly, but by observing the poll loop's
// externally-visible behavior: a second, well-formed payload sent shortly
// after the hung one still reaches the monitor well within the test's
// deadline, which is only possible if the first (hung) connection's
// handleConnection call actually returned instead of blocking the loop
// indefinitely.
func TestRemoteApprovalRelay_DialTimeout_DoesNotWedgePollLoop(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "dial-timeout-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	monitor := session.NewExternalApprovalMonitor()
	monitor.Start()
	defer monitor.Stop()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(
		pool, monitor,
		RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, SessionKey: "session-key", Title: "Test"},
		withPollInterval(20*time.Millisecond),
		withDialTimeout(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := RemoteApprovalSocketPath(basePath)

	// Accept a connection and then hang -- never write a single byte -- to
	// simulate a stalled remote-side writer. decodePayload's read must be
	// abandoned at withDialTimeout's 100ms bound, not block forever.
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", socketPath, err)
	}
	hungConnCh := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			hungConnCh <- conn
		}
	}()

	select {
	case conn := <-hungConnCh:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("relay never dialed the hung listener")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	// A fresh listener (same socket path) for the follow-up payload -- the
	// relay's poll loop re-dials the socket path fresh on every iteration,
	// so this is a distinct connection/listener from the hung one above.
	payload := relayedApprovalPayload{
		Token:   token,
		Request: detection.ApprovalRequest{ID: "req-after-hang", Type: detection.ApprovalCommand, DetectedText: "still works"},
	}
	writeErrCh := writeApprovalOnce(t, socketPath, payload)

	got := waitForPendingApproval(t, monitor, "session-key", "req-after-hang", 5*time.Second)
	if got == nil {
		t.Fatal("poll loop appears wedged: a payload sent after the hung connection never reached the monitor")
	}

	select {
	case writeErr := <-writeErrCh:
		if writeErr != nil {
			t.Fatalf("writing the follow-up payload failed: %v", writeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up payload write never completed")
	}
}

// TestRemoteApprovalRelay_ReopensChannelAfterReconnect is Task 5.1.2d's
// integration test: the underlying pooled *ssh.Client is killed (simulating
// a connection drop) and redialed (simulating reconnect), and a request
// delivered only AFTER that redial -- standing in for the remote agent's
// documented retry-with-the-same-ID behavior (RemoteApprovalRelay's type
// doc comment) -- must still reach the monitor without recreating the
// relay or the session.
func TestRemoteApprovalRelay_ReopensChannelAfterReconnect(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "reconnect-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client1, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	monitor := session.NewExternalApprovalMonitor()
	monitor.Start()
	defer monitor.Stop()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, monitor, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, SessionKey: "session-key", Title: "Test"}, withPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := RemoteApprovalSocketPath(basePath)

	// Baseline: prove the relay works before any disruption.
	<-writeApprovalOnce(t, socketPath, relayedApprovalPayload{
		Token:   token,
		Request: detection.ApprovalRequest{ID: "req-before", Type: detection.ApprovalCommand, DetectedText: "before"},
	})
	if got := waitForPendingApproval(t, monitor, "session-key", "req-before", 5*time.Second); got == nil {
		t.Fatal("baseline request (pre-drop) never reached the monitor")
	}

	// Simulate the connection dropping.
	_ = client1.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pool.Peek(target.Name); !ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := pool.Peek(target.Name); ok {
		t.Fatal("pool entry was not evicted after the connection died")
	}

	// Simulate the agent script retrying: it keeps re-listening/re-writing
	// the SAME request (same ID) on the fixed socket path until it gets
	// through -- the documented agent-side-retry constraint Story 5.1.2
	// relies on (no relay-side buffering).
	retryPayload := relayedApprovalPayload{
		Token:   token,
		Request: detection.ApprovalRequest{ID: "req-inflight", Type: detection.ApprovalCommand, DetectedText: "in-flight"},
	}
	stopRetry := make(chan struct{})
	retryFinished := make(chan struct{})
	go func() {
		defer close(retryFinished)
		for {
			select {
			case <-stopRetry:
				return
			default:
			}
			errCh := writeApprovalOnce(t, socketPath, retryPayload)
			select {
			case <-errCh:
			case <-time.After(500 * time.Millisecond):
			}
			if waitForPendingApproval(t, monitor, "session-key", "req-inflight", 200*time.Millisecond) != nil {
				return
			}
		}
	}()

	// Simulate reconnect: redial the same remote name against the still-
	// running test server.
	client2, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() after simulated drop: %v", err)
	}
	if client2 == client1 {
		t.Fatal("GetOrDial() returned the dead client instead of redialing")
	}

	got := waitForPendingApproval(t, monitor, "session-key", "req-inflight", 10*time.Second)
	close(stopRetry)
	<-retryFinished
	if got == nil {
		t.Fatal("the in-flight request was never delivered after reconnect -- it was silently lost")
	}

	// Both the pre-drop and post-reconnect requests must be visible --
	// reconnecting must not have dropped or replaced the session's other
	// pending state.
	pending := monitor.GetPendingApprovals("session-key")
	if len(pending) != 2 {
		t.Errorf("GetPendingApprovals() after reconnect = %d entries, want 2 (req-before, req-inflight)", len(pending))
	}
}

// TestWriteApprovalOnce_SmokeTestsTheTestHelper is a narrow sanity check on
// this file's own writeApprovalOnce helper (not RemoteApprovalRelay
// itself): if the Unix-socket test double is broken, every other test in
// this file would fail for the wrong reason, so this pins its basic
// behavior directly.
func TestWriteApprovalOnce_SmokeTestsTheTestHelper(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "smoke.sock")
	payload := relayedApprovalPayload{Token: "t", Request: detection.ApprovalRequest{ID: "smoke"}}
	errCh := writeApprovalOnce(t, socketPath, payload)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial unix %s: %v", socketPath, err)
	}
	defer conn.Close()

	var got relayedApprovalPayload
	if err := json.NewDecoder(conn).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Request.ID != "smoke" {
		t.Errorf("Request.ID = %q, want %q", got.Request.ID, "smoke")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("writeApprovalOnce reported an error: %v", err)
	}
	_ = os.Remove(socketPath)
}
