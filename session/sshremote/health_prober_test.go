package sshremote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// healthProberTestServer is a minimal in-process SSH server backing every
// RemoteHealthProber test in this file. Deliberately a separate,
// trimmed-down helper rather than a reuse of
// session/tmux/ssh_test_server_test.go's testSSHServer or this file's own
// sibling approvalRelayTestServer: both live in different packages/files
// with unexported helpers this file can't reach for free, and this prober
// only ever needs to run a single command ("true", via
// SSHRunner.Run(ctx, "", "true")) -- see startHealthProberTestServer's
// rejectSessions toggle for how the "reconnecting" soft-degradation state
// is simulated (reject new session channels while leaving the underlying
// transport connection alive, so Client.Wait() never fires).
type healthProberTestServer struct {
	Addr    string
	HostKey ssh.PublicKey

	rejectSessions atomic.Bool
}

func startHealthProberTestServer(t *testing.T) *healthProberTestServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("build host signer: %v", err)
	}

	hts := &healthProberTestServer{HostKey: hostSigner.PublicKey()}

	srv := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {
			cmd := s.Command()
			if len(cmd) > 0 && cmd[0] == "true" {
				_ = s.Exit(0)
				return
			}
			_ = s.Exit(0)
		},
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"session": func(srv *gliderssh.Server, conn *ssh.ServerConn, newChan ssh.NewChannel, ctx gliderssh.Context) {
				if hts.rejectSessions.Load() {
					_ = newChan.Reject(ssh.ResourceShortage, "session channels rejected (test)")
					return
				}
				gliderssh.DefaultSessionHandler(srv, conn, newChan, ctx)
			},
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

	hts.Addr = ln.Addr().String()
	return hts
}

func healthProberTestClientConfig(t *testing.T, hostKey ssh.PublicKey) ssh.ClientConfig {
	t.Helper()
	return ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}
}

// fakeHealthPublisher is a same-package test double for HealthEventPublisher.
type fakeHealthPublisher struct {
	mu     sync.Mutex
	events []healthEvent

	notifyCh chan healthEvent
}

type healthEvent struct {
	remoteName    string
	state         RemoteConnectionState
	previousState RemoteConnectionState
}

func newFakeHealthPublisher() *fakeHealthPublisher {
	return &fakeHealthPublisher{notifyCh: make(chan healthEvent, 32)}
}

func (f *fakeHealthPublisher) PublishRemoteHealthChanged(remoteName string, state, previousState RemoteConnectionState) {
	f.mu.Lock()
	f.events = append(f.events, healthEvent{remoteName: remoteName, state: state, previousState: previousState})
	f.mu.Unlock()

	select {
	case f.notifyCh <- healthEvent{remoteName: remoteName, state: state, previousState: previousState}:
	default:
	}
}

func (f *fakeHealthPublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

// eventsSnapshot returns a copy of every event recorded so far.
func (f *fakeHealthPublisher) eventsSnapshot() []healthEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]healthEvent, len(f.events))
	copy(out, f.events)
	return out
}

// waitForState blocks until a healthEvent with the given state arrives, or
// timeout elapses (in which case it fails the test).
func (f *fakeHealthPublisher) waitForState(t *testing.T, want RemoteConnectionState, timeout time.Duration) healthEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-f.notifyCh:
			if ev.state == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("never observed a transition to state %q within %s", want, timeout)
		}
	}
}

func newHealthProberTestPool(t *testing.T) *tmux.SSHClientPool {
	t.Helper()
	return tmux.NewSSHClientPool()
}

func TestNewRemoteHealthProber_ValidatesArgs(t *testing.T) {
	pool := newHealthProberTestPool(t)
	runner := tmux.NewSSHRunner(tmux.SSHTarget{Name: "r", Addr: "unused:22"}, ssh.ClientConfig{}, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()

	tests := []struct {
		name       string
		pool       *tmux.SSHClientPool
		runner     *tmux.SSHRunner
		remoteName string
		publisher  HealthEventPublisher
	}{
		{"nil pool", nil, runner, "r", publisher},
		{"nil runner", pool, nil, "r", publisher},
		{"empty remoteName", pool, runner, "", publisher},
		{"nil publisher", pool, runner, "r", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewRemoteHealthProber(tt.pool, tt.runner, tt.remoteName, tt.publisher); err == nil {
				t.Error("NewRemoteHealthProber() error = nil, want a validation error")
			}
		})
	}
}

// TestRemoteHealthProber_PublishesConnectedWhenAlreadyPooled verifies the
// baseline transition: if a client is already pooled for remoteName before
// Start is called, Subscribe's priming value drives an immediate
// disconnected -> connected publish, without waiting out a liveness-check
// tick.
func TestRemoteHealthProber_PublishesConnectedWhenAlreadyPooled(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "connected-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(pool, runner, target.Name, publisher, withLivenessCheckInterval(time.Hour))
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	defer prober.Stop()

	ev := publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)
	if ev.remoteName != target.Name {
		t.Errorf("remoteName = %q, want %q", ev.remoteName, target.Name)
	}
	if ev.previousState != RemoteConnectionStateDisconnected {
		t.Errorf("previousState = %q, want %q", ev.previousState, RemoteConnectionStateDisconnected)
	}
	if got := prober.State(); got != RemoteConnectionStateConnected {
		t.Errorf("State() = %q, want %q", got, RemoteConnectionStateConnected)
	}
}

// TestRemoteHealthProber_DisconnectTriggersEventWithinOneTick is this
// file's core acceptance-criteria test (Story 6.4.1 / Task 6.4.1d): closing
// the pooled *ssh.Client out from under the prober must produce a
// connected -> disconnected publish driven by Client.Wait() returning, not
// by waiting out a liveness-check tick -- proven by setting
// livenessInterval far longer than the test's own timeout, so ONLY the
// Wait()-based push path could possibly deliver the event in time.
func TestRemoteHealthProber_DisconnectTriggersEventWithinOneTick(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "disconnect-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	// A liveness interval far longer than this test's own timeout budget:
	// if a disconnected event arrives anyway, it can only have come from
	// the Wait()-driven watchClientDeath path, not runLivenessLoop's ticker.
	prober, err := NewRemoteHealthProber(pool, runner, target.Name, publisher, withLivenessCheckInterval(time.Hour))
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	defer prober.Stop()

	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	// Simulate the connection dropping.
	_ = client.Close()

	ev := publisher.waitForState(t, RemoteConnectionStateDisconnected, 5*time.Second)
	if ev.previousState != RemoteConnectionStateConnected {
		t.Errorf("previousState = %q, want %q", ev.previousState, RemoteConnectionStateConnected)
	}
	if got := prober.State(); got != RemoteConnectionStateDisconnected {
		t.Errorf("State() = %q, want %q", got, RemoteConnectionStateDisconnected)
	}
}

// TestRemoteHealthProber_ReconnectAfterHardDisconnectRestoresConnected
// proves the full connected -> disconnected -> connected cycle: after a
// hard disconnect, a fresh dial for the same remote name (delivered via
// SSHClientPool.Subscribe's reconnect notification -- Story 5.1.2's
// mechanism, reused here) drives the prober back to connected.
func TestRemoteHealthProber_ReconnectAfterHardDisconnectRestoresConnected(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "reconnect-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client1, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(pool, runner, target.Name, publisher, withLivenessCheckInterval(time.Hour))
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	defer prober.Stop()

	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	_ = client1.Close()
	publisher.waitForState(t, RemoteConnectionStateDisconnected, 5*time.Second)

	client2, err := pool.GetOrDial(ctx, target, &cfg)
	if err != nil {
		t.Fatalf("GetOrDial() after simulated drop: %v", err)
	}
	if client2 == client1 {
		t.Fatal("GetOrDial() returned the dead client instead of redialing")
	}

	ev := publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)
	if ev.previousState != RemoteConnectionStateDisconnected {
		t.Errorf("previousState = %q, want %q", ev.previousState, RemoteConnectionStateDisconnected)
	}
}

// TestRemoteHealthProber_LivenessFailureTransitionsToReconnecting proves
// the soft-degradation path Task 6.4.1b calls for: the remote host
// rejecting new session channels (simulating a stalled/degraded remote)
// while the underlying transport connection stays fully alive -- so
// Client.Wait() never fires -- must still surface as
// RemoteConnectionStateReconnecting via the periodic liveness check, and
// recover back to connected once the remote starts accepting sessions
// again.
func TestRemoteHealthProber_LivenessFailureTransitionsToReconnecting(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "flaky-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(
		pool, runner, target.Name, publisher,
		withLivenessCheckInterval(20*time.Millisecond),
		withLivenessCheckTimeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	defer prober.Stop()

	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	srv.rejectSessions.Store(true)
	publisher.waitForState(t, RemoteConnectionStateReconnecting, 5*time.Second)

	srv.rejectSessions.Store(false)
	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)
}

// TestRemoteHealthProber_StopIsIdempotentAndReturnsPromptly verifies Stop
// can be called multiple times and doesn't hang waiting on the (untracked)
// watchClientDeath goroutine's blocking Wait() call for a still-live
// client.
func TestRemoteHealthProber_StopIsIdempotentAndReturnsPromptly(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "stop-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(pool, runner, target.Name, publisher, withLivenessCheckInterval(time.Hour))
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	done := make(chan struct{})
	go func() {
		prober.Stop()
		prober.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return promptly")
	}
}

// TestRemoteHealthProber_NoOpTransitionDoesNotRepublish verifies transition
// only publishes on an ACTUAL state change: two consecutive successful
// liveness checks while already connected must not produce a second
// "connected" event.
func TestRemoteHealthProber_NoOpTransitionDoesNotRepublish(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "no-flood-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.GetOrDial(ctx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(
		pool, runner, target.Name, publisher,
		withLivenessCheckInterval(10*time.Millisecond),
		withLivenessCheckTimeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(ctx)
	defer prober.Stop()

	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	// Let several liveness-check ticks elapse while nothing changes.
	time.Sleep(200 * time.Millisecond)

	if got := publisher.count(); got != 1 {
		t.Errorf("publisher recorded %d events, want exactly 1 (no repeated connected publishes across ticks)", got)
	}
}

// TestRemoteHealthProber_CheckLiveness_NoOpAfterCtxCancelled is a deterministic regression
// guard (no timing race, unlike relying on Stop() happening to land mid-check): a
// context.Canceled-wrapping error from runner.Run -- exactly what a liveness check in flight
// when Stop() cancels p.ctx would see -- must not be treated as a genuine liveness failure.
// Without checkLiveness's ctx.Done() guard, this would publish a spurious
// connected->reconnecting transition, which in production happens on essentially every
// Stop() call (a liveness check is in flight roughly timeout/interval of the time at any
// given moment), directly undermining "reflects reality in real time." This test cancels the
// prober's internal context directly (same-package access to the unexported cancel field) and
// then calls checkLiveness() synchronously exactly once, rather than starting the background
// loop and hoping Stop() races it -- deterministic, not probabilistic.
func TestRemoteHealthProber_CheckLiveness_NoOpAfterCtxCancelled(t *testing.T) {
	srv := startHealthProberTestServer(t)
	pool := newHealthProberTestPool(t)
	target := tmux.SSHTarget{Name: "cancelled-ctx-remote", Addr: srv.Addr}
	cfg := healthProberTestClientConfig(t, srv.HostKey)

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dialCancel()
	if _, err := pool.GetOrDial(dialCtx, target, &cfg); err != nil {
		t.Fatalf("GetOrDial() error: %v", err)
	}

	runner := tmux.NewSSHRunner(target, cfg, tmux.WithSSHClientPool(pool))
	publisher := newFakeHealthPublisher()
	prober, err := NewRemoteHealthProber(
		pool, runner, target.Name, publisher,
		// Long enough that the loop never fires on its own during this test.
		withLivenessCheckInterval(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("NewRemoteHealthProber() error: %v", err)
	}
	prober.Start(context.Background())
	defer prober.Stop()
	publisher.waitForState(t, RemoteConnectionStateConnected, 5*time.Second)

	before := publisher.count()

	// Simulate exactly what Stop() does to p.ctx (without waiting on p.wg, so this doesn't
	// race the background loop goroutines at all), then drive checkLiveness synchronously
	// with that already-cancelled context -- runner.Run must fail with a context-cancellation
	// error immediately, and checkLiveness must recognize that as "we're shutting down," not
	// "the remote is unreachable."
	prober.cancel()
	prober.checkLiveness()

	after := publisher.eventsSnapshot()
	if len(after) != before {
		t.Fatalf("checkLiveness() published %d new event(s) after ctx cancellation, want 0: %+v", len(after)-before, after[before:])
	}
	for _, ev := range after {
		if ev.state == RemoteConnectionStateReconnecting {
			t.Fatalf("checkLiveness() produced a spurious connected->reconnecting publish (%+v) after ctx cancellation -- the ctx.Done() guard did not fire", ev)
		}
	}
}
