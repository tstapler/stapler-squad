package sshremote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

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
// handler's general accept-then-bidirectional-copy shape
// (github.com/gliderlabs/ssh@v0.3.8/tcpip.go) but dials a Unix domain
// socket instead of TCP -- faithfully simulating what a real sshd does for
// direct-streamlocal: connect, as a client, to the socket path the
// channel-open request names, then relay bytes between that connection and
// the new SSH channel. This is what makes RemoteApprovalRelay's
// client.Dial("unix", socketPath) actually reach a real net.Listener at
// socketPath in these tests, exactly as it would reach a real sshd-side
// Unix socket connect() on an actual remote host.
//
// Unlike the original tcpip.go shape this mirrors (which is fine for a
// pure one-shot forward with no response), each copy direction here
// propagates EOF as a HALF-close (ssh.Channel.CloseWrite / UnixConn.
// CloseWrite) rather than fully closing both ends the moment either
// direction finishes -- required once Part A's request/response round trip
// needs the channel to survive a write-side half-close on one side (the
// test payload writer calling CloseWrite after writing its JSON) long
// enough for the OTHER direction (the relay's response bytes) to still get
// through. Both ends are only fully closed once BOTH copy directions have
// finished.
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

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(ch, dconn)
		_ = ch.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(dconn, ch)
		if cw, ok := dconn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	go func() {
		wg.Wait()
		_ = ch.Close()
		_ = dconn.Close()
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

// writeApprovalAndReadResponse simulates the remote agent hook script
// (server/services.remoteApprovalHookCommand's socat pipeline, not this
// package): it listens on socketPath, accepts exactly one connection,
// writes payload as JSON, half-closes its write side (via CloseWrite, the
// same "printf/cat's stdin EOF becomes an SSH channel EOF" mechanic socat
// produces in production), reads back whatever bytes the relay writes as
// the response, and closes. Returns a channel delivering the response bytes
// (and any error) once the whole exchange completes.
type approvalExchangeResult struct {
	response []byte
	err      error
}

func writeApprovalAndReadResponse(t *testing.T, socketPath string, payload relayedApprovalPayload) <-chan approvalExchangeResult {
	t.Helper()
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", socketPath, err)
	}
	resultCh := make(chan approvalExchangeResult, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			resultCh <- approvalExchangeResult{err: err}
			return
		}
		defer conn.Close()

		if err := json.NewEncoder(conn).Encode(payload); err != nil {
			resultCh <- approvalExchangeResult{err: err}
			return
		}
		// Half-close the write side so the relay's json.Decoder sees a clean
		// end of the JSON value's stream, mirroring what socat does once
		// printf/cat's stdin reaches EOF in production.
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}

		resp, err := io.ReadAll(conn)
		resultCh <- approvalExchangeResult{response: resp, err: err}
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

// fakePermissionRequestHandler is a same-package test double for
// PermissionRequestHandler. Every test in this file uses a fake rather than
// a real *server/services.ApprovalHandler: constructing one here would
// require this package to import server/services, which itself imports
// session/sshremote -- an import cycle. The real end-to-end proof that
// *server/services.ApprovalHandler actually satisfies this interface and
// works correctly through a real RemoteApprovalRelay belongs in
// server/services's own package tests (Part B).
type fakePermissionRequestHandler struct {
	// handle is invoked synchronously with the decoded request body and
	// X-CS-Session-ID header value; its return value is written verbatim to
	// the ResponseWriter as the "response". nil defaults to echoing back a
	// fixed allow decision.
	handle func(bodyBytes []byte, sessionIDHeader string, r *http.Request) []byte

	callsCh  chan struct{}
	lastBody []byte
	lastHdr  string
}

func newFakePermissionRequestHandler() *fakePermissionRequestHandler {
	return &fakePermissionRequestHandler{callsCh: make(chan struct{}, 16)}
}

func (f *fakePermissionRequestHandler) HandlePermissionRequest(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.lastBody = body
	f.lastHdr = r.Header.Get("X-CS-Session-ID")

	var out []byte
	if f.handle != nil {
		out = f.handle(body, f.lastHdr, r)
	} else {
		out = []byte(`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`)
	}
	_, _ = w.Write(out)

	select {
	case f.callsCh <- struct{}{}:
	default:
	}
}

// waitForCall blocks until HandlePermissionRequest has been invoked at
// least once, or timeout elapses (in which case it fails the test).
func (f *fakePermissionRequestHandler) waitForCall(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-f.callsCh:
	case <-time.After(timeout):
		t.Fatal("PermissionRequestHandler.HandlePermissionRequest was never called")
	}
}

func TestRemoteApprovalSocketPath(t *testing.T) {
	tests := []struct {
		basePath        string
		stableSessionID string
		want            string
	}{
		{"/home/agent/work", "session-a", "/home/agent/work/.ssq-appr-" + hashSessionIDForTest("session-a") + ".sock"},
		{"/home/agent/work/", "session-a", "/home/agent/work/.ssq-appr-" + hashSessionIDForTest("session-a") + ".sock"},
	}
	for _, tt := range tests {
		if got := RemoteApprovalSocketPath(tt.basePath, tt.stableSessionID); got != tt.want {
			t.Errorf("RemoteApprovalSocketPath(%q, %q) = %q, want %q", tt.basePath, tt.stableSessionID, got, tt.want)
		}
	}
}

// TestRemoteApprovalSocketPath_DifferentSessionIDsDisambiguateSharedBasePath proves the
// collision fix: two sessions rooted at the identical remote directory (e.g. both
// SessionTypeDirectory sessions pointed at the same path) must not resolve to the same
// socket, or the second session's hook's `unlink-early` bind would silently steal the
// first session's listener out from under it.
func TestRemoteApprovalSocketPath_DifferentSessionIDsDisambiguateSharedBasePath(t *testing.T) {
	const sharedBasePath = "/home/agent/shared-dir"
	a := RemoteApprovalSocketPath(sharedBasePath, "session-a")
	b := RemoteApprovalSocketPath(sharedBasePath, "session-b")
	if a == b {
		t.Fatalf("expected distinct socket paths for distinct session IDs sharing a base path, got %q for both", a)
	}
}

func hashSessionIDForTest(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", sum[:remoteApprovalSocketIDHashBytes])
}

func TestNewRemoteApprovalRelay_ValidatesArgs(t *testing.T) {
	pool := newTestPool(t)
	handler := newFakePermissionRequestHandler()

	tests := []struct {
		name            string
		pool            *tmux.SSHClientPool
		handler         PermissionRequestHandler
		remoteName      string
		basePath        string
		stableSessionID string
	}{
		{"nil pool", nil, handler, "r", "/base", "k"},
		{"nil handler", pool, nil, "r", "/base", "k"},
		{"empty remoteName", pool, handler, "", "/base", "k"},
		{"empty basePath", pool, handler, "r", "", "k"},
		{"empty stableSessionID", pool, handler, "r", "/base", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := RemoteApprovalRelayTarget{RemoteName: tt.remoteName, BasePath: tt.basePath, StableSessionID: tt.stableSessionID, Title: "title"}
			if _, err := NewRemoteApprovalRelay(tt.pool, tt.handler, target); err == nil {
				t.Error("NewRemoteApprovalRelay() error = nil, want a validation error")
			}
		})
	}
}

// TestRemoteApprovalRelay_DrivesRequestThroughHandler is
// this file's core Part A integration test: a raw JSON payload written to
// the remote Unix socket must be decoded, its bearer token verified, and
// then driven through PermissionRequestHandler as a synthetic HTTP request
// carrying the raw bytes as its body and X-CS-Session-ID set to the
// relay's configured StableSessionID -- with the handler's written response
// bytes relayed back over the SAME connection.
func TestRemoteApprovalRelay_DrivesRequestThroughHandler(t *testing.T) {
	srv := startApprovalRelayTestServer(t)
	pool := newTestPool(t)
	target, cfg := poolTargetFor(t, srv, "forward-remote")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	handler := newFakePermissionRequestHandler()
	wantResponse := []byte(`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"no"}}}`)
	handler.handle = func([]byte, string, *http.Request) []byte { return wantResponse }

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, handler, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, StableSessionID: "stable-session-1", Title: "Test Remote Session"}, withPollInterval(20*time.Millisecond))
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

	rawRequest := json.RawMessage(`{"tool_name":"Bash","tool_input":{"command":"rm -rf /"},"cwd":"/home/agent/work"}`)
	socketPath := RemoteApprovalSocketPath(basePath, "stable-session-1")
	payload := relayedApprovalPayload{Token: token, Request: rawRequest}
	resultCh := writeApprovalAndReadResponse(t, socketPath, payload)

	handler.waitForCall(t, 5*time.Second)

	if handler.lastHdr != "stable-session-1" {
		t.Errorf("X-CS-Session-ID header = %q, want %q", handler.lastHdr, "stable-session-1")
	}
	if string(handler.lastBody) != string(rawRequest) {
		t.Errorf("request body = %s, want %s (raw passthrough)", handler.lastBody, rawRequest)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("exchange failed: %v", result.err)
		}
		if string(result.response) != string(wantResponse) {
			t.Errorf("response written back to remote = %s, want %s", result.response, wantResponse)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never received a response back from the relay")
	}
}

// TestRemoteApprovalRelay_RejectsWrongBearerToken verifies a payload with
// the wrong token is dropped without ever reaching the handler -- Task
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

	handler := newFakePermissionRequestHandler()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, handler, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, StableSessionID: "session-key", Title: "Test"}, withPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	socketPath := RemoteApprovalSocketPath(basePath, "session-key")
	payload := relayedApprovalPayload{Token: "not-the-real-token", Request: json.RawMessage(`{"tool_name":"Bash"}`)}
	<-writeApprovalAndReadResponse(t, socketPath, payload)

	select {
	case <-handler.callsCh:
		t.Fatal("a payload with the wrong bearer token was forwarded to the handler")
	case <-time.After(500 * time.Millisecond):
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

	handler := newFakePermissionRequestHandler()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(
		pool, handler,
		RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, StableSessionID: "session-key", Title: "Test"},
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

	socketPath := RemoteApprovalSocketPath(basePath, "session-key")
	payload := relayedApprovalPayload{Token: token, Request: json.RawMessage(`{"tool_name":"Bash"}`)}
	<-writeApprovalAndReadResponse(t, socketPath, payload)

	select {
	case <-handler.callsCh:
		t.Fatal("a payload with an expired bearer token was forwarded to the handler")
	case <-time.After(500 * time.Millisecond):
	}
}

// TestRemoteApprovalRelay_DialTimeout_DoesNotWedgePollLoop exercises
// withDialTimeout: a remote-side connection that accepts but never writes
// (simulating a stalled handshake/hung write on the real remote host) must
// be abandoned within the configured timeout rather than blocking the
// relay's single poll goroutine forever -- proven here not by inspecting
// decodePayload's internals directly, but by observing the poll loop's
// externally-visible behavior: a second, well-formed payload sent shortly
// after the hung one still reaches the handler well within the test's
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

	handler := newFakePermissionRequestHandler()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(
		pool, handler,
		RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, StableSessionID: "session-key", Title: "Test"},
		withPollInterval(20*time.Millisecond),
		withDialTimeout(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := RemoteApprovalSocketPath(basePath, "session-key")

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
	payload := relayedApprovalPayload{Token: token, Request: json.RawMessage(`{"tool_name":"Bash"}`)}
	resultCh := writeApprovalAndReadResponse(t, socketPath, payload)

	handler.waitForCall(t, 5*time.Second)

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("writing/reading the follow-up payload failed: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up payload exchange never completed")
	}
}

// TestRemoteApprovalRelay_ReopensChannelAfterReconnect is Task 5.1.2d's
// integration test: the underlying pooled *ssh.Client is killed (simulating
// a connection drop) and redialed (simulating reconnect), and a request
// delivered only AFTER that redial -- standing in for the remote agent's
// documented retry-with-the-same-payload behavior -- must still reach the
// handler without recreating the relay or the session.
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

	handler := newFakePermissionRequestHandler()

	basePath := t.TempDir()
	relay, err := NewRemoteApprovalRelay(pool, handler, RemoteApprovalRelayTarget{RemoteName: target.Name, BasePath: basePath, StableSessionID: "session-key", Title: "Test"}, withPollInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := RemoteApprovalSocketPath(basePath, "session-key")

	// Baseline: prove the relay works before any disruption.
	<-writeApprovalAndReadResponse(t, socketPath, relayedApprovalPayload{Token: token, Request: json.RawMessage(`{"tool_name":"before"}`)})
	handler.waitForCall(t, 5*time.Second)

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
	// the same request on the fixed socket path until it gets through -- the
	// documented agent-side-retry constraint Story 5.1.2 relies on (see
	// server/services.remoteApprovalHookCommand's retry loop, the real
	// production mechanism this simulates). This scenario has no undelivered
	// decision to buffer/redeliver (the connection died before the FIRST
	// attempt's request even reached the relay, unlike
	// TestRemoteApprovalRelay_RedeliversAfterDrop's write-failure-after-
	// decision case in this same file), so it exercises reconnect/redial
	// alone, independent of the redelivery-buffering path.
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
			resultCh := writeApprovalAndReadResponse(t, socketPath, relayedApprovalPayload{Token: token, Request: json.RawMessage(`{"tool_name":"in-flight"}`)})
			select {
			case <-resultCh:
				return
			case <-time.After(500 * time.Millisecond):
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

	select {
	case <-retryFinished:
	case <-time.After(10 * time.Second):
		close(stopRetry)
		t.Fatal("the in-flight request was never delivered after reconnect -- it was silently lost")
	}
	close(stopRetry)
	// retryFinished closing (awaited above) already proves the post-reconnect
	// request reached the handler -- writeApprovalAndReadResponse's resultCh
	// only fires once a response comes back over the connection, which only
	// happens after handleConnection's r.handler.HandlePermissionRequest call
	// returns. Combined with the baseline handler.waitForCall above (the
	// pre-drop request), both requests are proven delivered.
}

// TestWriteApprovalAndReadResponse_SmokeTestsTheTestHelper is a narrow
// sanity check on this file's own writeApprovalAndReadResponse helper (not
// RemoteApprovalRelay itself): if the Unix-socket test double is broken,
// every other test in this file would fail for the wrong reason, so this
// pins its basic behavior directly.
func TestWriteApprovalAndReadResponse_SmokeTestsTheTestHelper(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "smoke.sock")
	payload := relayedApprovalPayload{Token: "t", Request: json.RawMessage(`{"tool_name":"smoke"}`)}
	resultCh := writeApprovalAndReadResponse(t, socketPath, payload)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial unix %s: %v", socketPath, err)
	}

	var got relayedApprovalPayload
	if err := json.NewDecoder(conn).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got.Request) != `{"tool_name":"smoke"}` {
		t.Errorf("Request = %s, want %s", got.Request, `{"tool_name":"smoke"}`)
	}

	if _, err := conn.Write([]byte("fake-response")); err != nil {
		t.Fatalf("write fake response: %v", err)
	}
	_ = conn.Close()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("writeApprovalAndReadResponse reported an error: %v", result.err)
		}
		if string(result.response) != "fake-response" {
			t.Errorf("response = %q, want %q", result.response, "fake-response")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writeApprovalAndReadResponse never completed")
	}
	_ = os.Remove(socketPath)
}

// writeFailConn wraps a real net.Conn (for Read/Close/deadlines, so
// decodePayload can still successfully read a request through it) but
// forces every Write to fail. Used instead of actually killing a real
// direct-streamlocal-forwarded connection: an SSH channel's Write() can
// succeed at the local send-buffering/flow-control layer even when the
// downstream forwarded Unix-socket peer is already dead (the fake sshd's
// own io.Copy(dconn, ch) only notices and closes the channel
// asymmetrically later) -- confirmed the hard way, an earlier version of
// this test closed the real forwarded connection first and observed the
// relay's subsequent conn.Write() succeed anyway, non-deterministically.
// Forcing the failure directly at this boundary tests handleConnection's
// buffering logic itself, deterministically, independent of that transport
// race.
type writeFailConn struct {
	net.Conn
	writeErr error
}

func (c *writeFailConn) Write([]byte) (int, error) {
	return 0, c.writeErr
}

// TestRemoteApprovalRelay_RedeliversAfterDrop is the core regression test
// for the review-found AC4 gap: "a network drop while a request is blocked
// waiting on a human decision is not implemented (no buffering/redelivery)
// or tested." The first connection delivers a request and computes a
// decision, but its Write back to the remote side is forced to fail
// (writeFailConn) -- simulating the connection having died in the interval
// between the human deciding and the response reaching the wire. A second
// connection resending the SAME request bytes (the hook script's own retry,
// server/services.remoteApprovalHookCommand) must receive that SAME decision
// without r.handler being invoked a second time -- re-invoking it would
// create a second pending-approval record and could re-prompt a human who
// already decided once.
func TestRemoteApprovalRelay_RedeliversAfterDrop(t *testing.T) {
	pool := newTestPool(t)

	var handlerCalls int32
	handler := newFakePermissionRequestHandler()
	wantResponse := []byte(`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`)
	handler.handle = func([]byte, string, *http.Request) []byte {
		atomic.AddInt32(&handlerCalls, 1)
		return wantResponse
	}

	relay, err := NewRemoteApprovalRelay(pool, handler, RemoteApprovalRelayTarget{RemoteName: "redeliver-remote", BasePath: "/base", StableSessionID: "redeliver-session", Title: "Test"})
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Start only to populate r.ctx (handleConnection's synthetic request is
	// bound to it) -- its poll loop stays idle for this test since nothing
	// is ever pooled; handleConnection is invoked directly below, bypassing
	// dial/accept entirely.
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	sameRequest := json.RawMessage(`{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
	payloadBytes, err := json.Marshal(relayedApprovalPayload{Token: token, Request: sameRequest})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// First attempt: a real net.Pipe delivers the request bytes; the write
	// back is forced to fail.
	serverSide, clientSide := net.Pipe()
	go func() { _, _ = clientSide.Write(payloadBytes) }()
	relay.handleConnection(&writeFailConn{Conn: serverSide, writeErr: errors.New("simulated connection drop mid-request")})
	_ = clientSide.Close()

	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Fatalf("handler call count after first (failed-write) attempt = %d, want 1", got)
	}

	// Retry: the SAME request bytes over a genuinely working connection must
	// get the buffered decision without the handler running again.
	serverSide2, clientSide2 := net.Pipe()
	respCh := make(chan []byte, 1)
	go func() {
		_, _ = clientSide2.Write(payloadBytes)
		buf := make([]byte, len(wantResponse))
		n, _ := io.ReadFull(clientSide2, buf)
		respCh <- buf[:n]
		_ = clientSide2.Close()
	}()
	relay.handleConnection(serverSide2)

	select {
	case resp := <-respCh:
		if string(resp) != string(wantResponse) {
			t.Errorf("retry response = %s, want the SAME buffered decision %s", resp, wantResponse)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry never received the buffered decision -- it was lost, not redelivered")
	}

	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Errorf("handler call count after retry = %d, want still 1 (redelivery must not re-invoke the handler and re-prompt a human)", got)
	}
}

// TestRemoteApprovalRelay_ExpiredBufferedDecisionIsNotRedelivered is the
// regression test for the review-found gap in the redelivery buffer itself
// (found independently by two reviewers): without a TTL, a buffered decision
// stays redeliverable forever, so a genuinely later, byte-identical request
// (classifier.PermissionRequestPayload carries no nonce/request ID) would
// silently get the earlier human's stale answer with no prompt. Directly
// backdates r.pendingAt (same package, unexported field) rather than
// sleeping pendingRedeliveryTTL in a test.
func TestRemoteApprovalRelay_ExpiredBufferedDecisionIsNotRedelivered(t *testing.T) {
	pool := newTestPool(t)

	var handlerCalls int32
	handler := newFakePermissionRequestHandler()
	handler.handle = func([]byte, string, *http.Request) []byte {
		atomic.AddInt32(&handlerCalls, 1)
		return []byte(`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`)
	}

	relay, err := NewRemoteApprovalRelay(pool, handler, RemoteApprovalRelayTarget{RemoteName: "expiry-remote", BasePath: "/base", StableSessionID: "expiry-session", Title: "Test"})
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	sameRequest := json.RawMessage(`{"tool_name":"Bash","tool_input":{"command":"git push --force"}}`)
	payloadBytes, err := json.Marshal(relayedApprovalPayload{Token: token, Request: sameRequest})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// First attempt: buffer a decision via a forced write failure.
	serverSide, clientSide := net.Pipe()
	go func() { _, _ = clientSide.Write(payloadBytes) }()
	relay.handleConnection(&writeFailConn{Conn: serverSide, writeErr: errors.New("simulated connection drop")})
	_ = clientSide.Close()
	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Fatalf("handler call count after first attempt = %d, want 1", got)
	}

	// Simulate the TTL having elapsed long ago.
	relay.pendingAt = time.Now().Add(-2 * pendingRedeliveryTTL)

	// A "retry" with the identical request bytes now, after expiry, must be
	// treated as a genuinely new request -- the handler runs again rather
	// than silently replaying the stale decision.
	serverSide2, clientSide2 := net.Pipe()
	go func() { _, _ = clientSide2.Write(payloadBytes) }()
	relay.handleConnection(&writeFailConn{Conn: serverSide2, writeErr: errors.New("simulated connection drop")})
	_ = clientSide2.Close()

	if got := atomic.LoadInt32(&handlerCalls); got != 2 {
		t.Errorf("handler call count after the expired-buffer retry = %d, want 2 (an expired buffer must not be redelivered -- the request must be re-decided)", got)
	}
}
