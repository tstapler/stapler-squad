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
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"os/exec"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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

// delayingClassifier stands in for a human taking real time to decide --
// review found (verified empirically) that socat's own default half-close
// wait (0.5s) silently drops the connection and hands the hook an empty,
// exit-0 response long before any real decision could complete. delay only
// needs to exceed that old default, not a full human-length wait, to prove
// the fix (remoteApprovalHookAttemptTimeoutSeconds's -t bound) without
// making the test itself slow.
type delayingClassifier struct{ delay time.Duration }

func (d delayingClassifier) BuildContext(cwd string) classifier.ClassificationContext {
	return classifier.ClassificationContext{Cwd: cwd}
}

func (d delayingClassifier) Classify(classifier.PermissionRequestPayload, classifier.ClassificationContext) classifier.ClassificationResult {
	time.Sleep(d.delay)
	return classifier.ClassificationResult{Decision: classifier.AutoAllow, RuleID: "test-delayed-allow", RuleName: "test delayed allow"}
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

// TestRemoteApprovalHookCommand_RealShellDeliversToRelay is the
// missing end-to-end proof this review found: every existing test in this
// package and in session/sshremote (including
// TestApprovalHandler_SatisfiesRelayHandlerEndToEnd just above) simulates
// the remote hook script's wire behavior directly in Go -- none of them ever
// executes remoteApprovalHookCommand's ACTUAL generated shell string through
// a real shell. That gap hid a real production bug: the command used to
// target UNIX-CONNECT (connect, as a client) when RemoteApprovalRelay's own
// direct-streamlocal dial requires sshd to CONNECT to the remote-side
// socket, meaning something there must LISTEN -- with both sides only ever
// connecting, no approval request could ever reach the relay in a real
// deployment, verified empirically (an actual `sh -c` run of the old command
// against a real relay produced socat's "No such file or directory" and the
// handler was never invoked). This test runs the real generated command
// (now UNIX-LISTEN,unlink-early) through a real shell, exactly as Claude
// Code's hook runner would, and proves it reaches *ApprovalHandler through a
// real RemoteApprovalRelay over a real (in-process) SSH connection.
//
// Skips if socat is not on PATH -- this exercises the real remote-host
// dependency remoteApprovalWriteTool's doc comment already flags, which this
// test environment may or may not have installed.
func TestRemoteApprovalHookCommand_RealShellDeliversToRelay(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not on PATH -- skipping the real-shell end-to-end proof")
	}

	srv := startRemoteRelayTestServer(t)
	pool := tmux.NewSSHClientPool()
	target := tmux.SSHTarget{Name: "hook-command-e2e-remote", Addr: srv.Addr}
	cfg := remoteRelayTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	handler := NewApprovalHandler(NewApprovalStore(""), nil, events.NewEventBus(4))
	handler.SetClassifier(autoAllowClassifier{})

	basePath := t.TempDir()
	relay, err := sshremote.NewRemoteApprovalRelay(pool, handler, sshremote.RemoteApprovalRelayTarget{
		RemoteName:      target.Name,
		BasePath:        basePath,
		StableSessionID: "hook-command-e2e-session",
		Title:           "Hook Command E2E Test Session",
	})
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := sshremote.RemoteApprovalSocketPath(basePath)

	// The REAL production command -- byte-for-byte what InjectHooksConfig
	// with WithRemoteHookTarget would write into a session's
	// .claude/settings.local.json, per InjectHookConfigRemote's own call to
	// remoteApprovalHookCommand.
	hookCmd := remoteApprovalHookCommand(RemoteHookTarget{SocketPath: socketPath, BearerToken: token})

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

	// Exactly how Claude Code invokes a hook command: pipe the hook-event
	// JSON to its stdin, capture stdout as the response.
	cmd := safeexec.CommandContext(ctx, "sh", "-c", hookCmd)
	cmd.Stdin = bytes.NewReader(reqJSON)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("real hook command failed: %v (output: %s)", runErr, out)
	}

	var decoded hookDecisionResponse
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("hook command output %s did not decode as hookDecisionResponse: %v", out, err)
	}
	if decoded.HookSpecificOutput.HookEventName != "PermissionRequest" {
		t.Errorf("HookEventName = %q, want %q", decoded.HookSpecificOutput.HookEventName, "PermissionRequest")
	}
	if decoded.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("Decision.Behavior = %q, want %q (from autoAllowClassifier)", decoded.HookSpecificOutput.Decision.Behavior, "allow")
	}
}

// TestRemoteApprovalHookCommand_SurvivesSlowHumanDecision is the regression
// test for the review-found timing bug: socat's own default half-close wait
// (0.5s) silently dropped the connection -- with exit 0 and empty output --
// long before any real human could decide, and the retry loop couldn't help
// because socat never signaled failure. delayingClassifier's 2s delay
// exceeds that old 0.5s default by 4x while staying fast enough for a unit
// test; it does not attempt the full 4-minute production bound directly
// (remoteApprovalHookAttemptTimeoutSeconds), only proves the mechanism that
// bound depends on -- socat's -t flag actually widening the wait -- works.
func TestRemoteApprovalHookCommand_SurvivesSlowHumanDecision(t *testing.T) {
	if _, err := exec.LookPath("socat"); err != nil {
		t.Skip("socat not on PATH -- skipping the real-shell timing proof")
	}

	srv := startRemoteRelayTestServer(t)
	pool := tmux.NewSSHClientPool()
	target := tmux.SSHTarget{Name: "hook-command-timing-remote", Addr: srv.Addr}
	cfg := remoteRelayTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	handler := NewApprovalHandler(NewApprovalStore(""), nil, events.NewEventBus(4))
	handler.SetClassifier(delayingClassifier{delay: 2 * time.Second})

	basePath := t.TempDir()
	relay, err := sshremote.NewRemoteApprovalRelay(pool, handler, sshremote.RemoteApprovalRelayTarget{
		RemoteName:      target.Name,
		BasePath:        basePath,
		StableSessionID: "hook-command-timing-session",
		Title:           "Hook Command Timing Test Session",
	})
	if err != nil {
		t.Fatalf("NewRemoteApprovalRelay() error: %v", err)
	}
	relay.Start(ctx)
	defer relay.Stop()

	token, _ := relay.BearerToken()
	socketPath := sshremote.RemoteApprovalSocketPath(basePath)
	hookCmd := remoteApprovalHookCommand(RemoteHookTarget{SocketPath: socketPath, BearerToken: token})

	reqJSON, err := json.Marshal(classifier.PermissionRequestPayload{
		ToolName: "Bash", ToolInput: map[string]interface{}{"command": "echo hi"},
		Cwd: "/home/agent/work", HookEventName: "PermissionRequest",
	})
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}

	cmd := safeexec.CommandContext(ctx, "sh", "-c", hookCmd)
	cmd.Stdin = bytes.NewReader(reqJSON)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("real hook command failed: %v (output: %s)", runErr, out)
	}

	var decoded hookDecisionResponse
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("hook command output %s did not decode as hookDecisionResponse (a pre-fix run would produce empty output here, since socat's default -t drops the connection before the 2s delayed decision arrives): %v", out, err)
	}
	if decoded.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("Decision.Behavior = %q, want %q (from delayingClassifier)", decoded.HookSpecificOutput.Decision.Behavior, "allow")
	}
}
