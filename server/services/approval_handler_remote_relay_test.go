package services

// approval_handler_remote_relay_test.go is Part B's real end-to-end proof
// (ssh-remote-workspaces Phase 5 correction, ADR-003's addendum) that
// *ApprovalHandler genuinely satisfies session/sshremote.
// PermissionRequestHandler and works correctly when driven through a real
// session/sshremote.RemoteApprovalRelay over a real (in-process) SSH
// connection -- not just that the two compile against the interface in
// isolation. session/sshremote's own tests (Part A) can only prove this
// with a FAKE PermissionRequestHandler, since session/sshremote can't
// import server/services (that would be the reverse of the real import
// direction and, worse, a cycle: server/services already imports
// session/sshremote for KeyStore/KnownHostsStore).
//
// This starts a real in-process SSH server (github.com/gliderlabs/ssh)
// supporting direct-streamlocal@openssh.com channel opens -- duplicated
// from session/sshremote/approval_relay_test.go's own test server rather
// than imported, for the same reason session_service_remote_test.go's own
// test SSH server is duplicated instead of shared: _test.go files aren't
// importable across package boundaries.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// remoteRelayStreamLocalOpenMsg mirrors golang.org/x/crypto/ssh's unexported
// streamLocalChannelOpenDirectMsg wire layout -- see
// session/sshremote/approval_relay_test.go's identical type for the full
// explanation of why field order (not names) is what matters here.
type remoteRelayStreamLocalOpenMsg struct {
	SocketPath string
	Reserved0  string
	Reserved1  uint32
}

// remoteRelayDirectStreamlocalHandler is a gliderssh.ChannelHandler for
// "direct-streamlocal@openssh.com", faithfully simulating a real sshd's
// handling of that channel type by connecting (as a client) to the named
// Unix socket and relaying bytes bidirectionally. Each direction propagates
// EOF as a half-close (CloseWrite) rather than fully closing the channel
// the moment either direction finishes -- required so the relay's response
// write (after the request has been fully read) still gets through. See
// session/sshremote/approval_relay_test.go's identical helper for the full
// rationale; duplicated here per this file's package doc comment.
func remoteRelayDirectStreamlocalHandler(_ *gliderssh.Server, _ *ssh.ServerConn, newChan ssh.NewChannel, _ gliderssh.Context) {
	var msg remoteRelayStreamLocalOpenMsg
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

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(ch, dconn)
		_ = ch.CloseWrite()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(dconn, ch)
		if cw, ok := dconn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		<-done
		<-done
		_ = ch.Close()
		_ = dconn.Close()
	}()
}

// remoteRelayTestServer is a minimal in-process SSH server supporting
// direct-streamlocal channel opens, standing in for a real sshd.
type remoteRelayTestServer struct {
	Addr    string
	HostKey ssh.PublicKey
}

func startRemoteRelayTestServer(t *testing.T) *remoteRelayTestServer {
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
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool { return true },
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"direct-streamlocal@openssh.com": remoteRelayDirectStreamlocalHandler,
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

	return &remoteRelayTestServer{Addr: ln.Addr().String(), HostKey: hostSigner.PublicKey()}
}

func remoteRelayTestClientConfig(t *testing.T, hostKey ssh.PublicKey) ssh.ClientConfig {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("build client signer: %v", err)
	}
	return ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}
}

// autoAllowClassifier is a fake classifier.Classifier that always returns
// AutoAllow -- "configured to auto-classify predictably for the test" per
// this feature's design brief, avoiding any need to wait on (or fake) a
// human decision through ApprovalStore/decisionCh for this end-to-end proof.
type autoAllowClassifier struct{}

func (autoAllowClassifier) BuildContext(cwd string) classifier.ClassificationContext {
	return classifier.ClassificationContext{Cwd: cwd}
}

func (autoAllowClassifier) Classify(classifier.PermissionRequestPayload, classifier.ClassificationContext) classifier.ClassificationResult {
	return classifier.ClassificationResult{Decision: classifier.AutoAllow, RuleID: "test-auto-allow", RuleName: "test auto-allow"}
}

// remoteRelayWirePayload mirrors session/sshremote's unexported
// relayedApprovalPayload JSON shape ({"token":...,"request":...}) -- this
// package can't reference that type directly (unexported, different
// package), so this is a same-wire-format stand-in built purely for
// encoding the test payload, exactly as a real remote hook script's socat
// pipeline would.
type remoteRelayWirePayload struct {
	Token   string          `json:"token"`
	Request json.RawMessage `json:"request"`
}

// TestApprovalHandler_SatisfiesRelayHandlerEndToEnd is Part B's required
// integration test: a raw classifier.PermissionRequestPayload, written to a
// real (test-sshd-backed) remote Unix socket, relayed through a real
// *sshremote.RemoteApprovalRelay wired to a real *ApprovalHandler
// (classifier auto-configured to allow, so the round trip completes without
// waiting on a human decision), must produce a hookDecisionResponse-shaped
// JSON response written back over the same connection -- proving the two
// packages' pieces genuinely fit together, not just that ApprovalHandler
// compiles against the sshremote.PermissionRequestHandler interface in
// isolation.
func TestApprovalHandler_SatisfiesRelayHandlerEndToEnd(t *testing.T) {
	srv := startRemoteRelayTestServer(t)
	pool := tmux.NewSSHClientPool()
	target := tmux.SSHTarget{Name: "approval-e2e-remote", Addr: srv.Addr}
	cfg := remoteRelayTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	// storage=nil is deliberate: ApprovalHandler.resolveSessionID falls back
	// to returning the X-CS-Session-ID header verbatim when h.storage is nil
	// (see its doc comment), so this test needs no real session.Storage/
	// session.Instance fixture to prove the round trip -- the relay's
	// StableSessionID is what ends up in that header regardless.
	handler := NewApprovalHandler(NewApprovalStore(""), nil, events.NewEventBus(4))
	handler.SetClassifier(autoAllowClassifier{})

	// *ApprovalHandler satisfying sshremote.PermissionRequestHandler
	// structurally (no explicit "implements" declaration anywhere) is itself
	// part of what this test proves -- if the method signature ever drifts,
	// this line fails to compile.
	var _ sshremote.PermissionRequestHandler = handler

	basePath := t.TempDir()
	relay, err := sshremote.NewRemoteApprovalRelay(pool, handler, sshremote.RemoteApprovalRelayTarget{
		RemoteName:      target.Name,
		BasePath:        basePath,
		StableSessionID: "e2e-stable-session-id",
		Title:           "E2E Test Session",
	})
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()

	reqPayload := classifier.PermissionRequestPayload{
		ToolName:      "Bash",
		ToolInput:     map[string]interface{}{"command": "echo hi"},
		Cwd:           "/home/agent/work",
		HookEventName: "PermissionRequest",
	}
	reqJSON, err := json.Marshal(reqPayload)
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}

	socketPath := sshremote.RemoteApprovalSocketPath(basePath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", socketPath, err)
	}

	type exchangeResult struct {
		response []byte
		err      error
	}
	resultCh := make(chan exchangeResult, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			resultCh <- exchangeResult{err: err}
			return
		}
		defer conn.Close()

		wire := remoteRelayWirePayload{Token: token, Request: reqJSON}
		if err := json.NewEncoder(conn).Encode(wire); err != nil {
			resultCh <- exchangeResult{err: err}
			return
		}
		if cw, ok := conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}

		resp, err := io.ReadAll(conn)
		resultCh <- exchangeResult{response: resp, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("exchange with the relay failed: %v", result.err)
		}
		var decoded hookDecisionResponse
		if err := json.Unmarshal(result.response, &decoded); err != nil {
			t.Fatalf("response %s did not decode as hookDecisionResponse: %v", result.response, err)
		}
		if decoded.HookSpecificOutput.HookEventName != "PermissionRequest" {
			t.Errorf("HookEventName = %q, want %q", decoded.HookSpecificOutput.HookEventName, "PermissionRequest")
		}
		if decoded.HookSpecificOutput.Decision.Behavior != "allow" {
			t.Errorf("Decision.Behavior = %q, want %q (from autoAllowClassifier)", decoded.HookSpecificOutput.Decision.Behavior, "allow")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("never received a response back through the relay")
	}
}
