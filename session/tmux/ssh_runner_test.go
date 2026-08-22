package tmux

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// newTestClientConfig builds an ssh.ClientConfig trusting exactly hostKey
// (via ssh.FixedHostKey) -- the "fixed-key-shaped HostKeyCallback" the plan
// text names as acceptable test plumbing ahead of Phase 3's real
// KnownHostsStore.
func newTestClientConfig(t *testing.T, hostKey ssh.PublicKey) ssh.ClientConfig {
	t.Helper()
	return ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	}
}

func newTestSSHRunner(t *testing.T, name, addr string, config ssh.ClientConfig, opts ...SSHRunnerOption) *SSHRunner {
	t.Helper()
	allOpts := append([]SSHRunnerOption{WithSSHClientPool(NewSSHClientPool())}, opts...)
	return NewSSHRunner(SSHTarget{Name: name, Addr: addr}, config, allOpts...)
}

// TestSSHRunner_IsRemote verifies SSHRunner always reports remote execution,
// paired with TestLocalRunner_IsRemote.
func TestSSHRunner_IsRemote(t *testing.T) {
	r := newTestSSHRunner(t, "remote1", "unused:22", ssh.ClientConfig{})
	if !r.IsRemote() {
		t.Error("SSHRunner.IsRemote() = false, want true")
	}
}

// TestSSHRunner_Run_TableDriven verifies Run round-trips combined
// stdout+stderr correctly for a handful of remote commands against a real
// in-process test sshd (github.com/gliderlabs/ssh), including the exact
// Given/When/Then from the plan's acceptance criteria: Run(ctx, "echo",
// "hi") returns []byte("hi\n") with a nil error.
func TestSSHRunner_Run_TableDriven(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "table-remote", srv.Addr, cfg)

	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr bool
	}{
		{name: "echo appends newline", args: []string{"echo", "hi"}, wantOut: "hi\n"},
		{name: "echo -n suppresses newline", args: []string{"echo", "-n", "hi"}, wantOut: "hi"},
		{name: "false exits non-zero", args: []string{"false"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			out, err := runner.Run(ctx, "", tt.args[0], tt.args[1:]...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Run() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if string(out) != tt.wantOut {
				t.Errorf("Run() out = %q, want %q", out, tt.wantOut)
			}
		})
	}
}

// TestSSHRunner_Start_RoundTripsBytes verifies Start returns a live
// stdin/stdout pipe pair backed by a real remote session: bytes written to
// stdin are readable from stdout before wait() is called, mirroring
// TestLocalRunner_Start_RoundTripsBytesBeforeWait.
func TestSSHRunner_Start_RoundTripsBytes(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "start-remote", srv.Addr, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdin, stdout, wait, err := runner.Start(ctx, "", "cat")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	const payload = "round-trip-test\n"
	if _, err := stdin.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write to stdin: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stdout, buf); err != nil {
		t.Fatalf("failed to read from stdout: %v", err)
	}
	if string(buf) != payload {
		t.Errorf("stdout = %q, want %q", buf, payload)
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("failed to close stdin: %v", err)
	}
	if err := wait(); err != nil {
		t.Fatalf("wait() returned error: %v", err)
	}
}

// TestBuildRemoteCommand verifies the shell-quoting/cd-prefixing logic
// CommandRunner.Run/Start's dir parameter is translated into, as a pure
// function -- there is no real remote shell in the test sshd to exercise
// "cd" against (the test server dispatches on shlex-parsed argv, not real
// shell semantics), so this is the faithful way to test dir honoring.
func TestBuildRemoteCommand(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		cmd  string
		args []string
		want string
	}{
		{name: "no dir, no args", dir: "", cmd: "true", want: "'true'"},
		{name: "no dir, with args", dir: "", cmd: "echo", args: []string{"hi"}, want: "'echo' 'hi'"},
		{name: "with dir", dir: "/tmp/work tree", cmd: "git", args: []string{"status"}, want: "cd '/tmp/work tree' && 'git' 'status'"},
		{name: "arg with embedded single quote", dir: "", cmd: "echo", args: []string{"it's"}, want: `'echo' 'it'"'"'s'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRemoteCommand(tt.dir, tt.cmd, tt.args)
			if got != tt.want {
				t.Errorf("buildRemoteCommand(%q, %q, %v) = %q, want %q", tt.dir, tt.cmd, tt.args, got, tt.want)
			}
		})
	}
}

// TestSSHRunner_Dial_UnknownHostKey_ReturnsErrUnknownHostKey is Story
// 2.1.1's core host-key acceptance criterion: an SSHRunner configured with
// a KnownHostsStore-shaped callback (knownhosts.New against an empty file)
// that has no entry for the target host returns ErrUnknownHostKey, wrapping
// the computed fingerprint, and never completes the handshake.
func TestSSHRunner_Dial_UnknownHostKey_ReturnsErrUnknownHostKey(t *testing.T) {
	srv := startTestSSHServer(t)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHostsPath, nil, 0o600); err != nil {
		t.Fatalf("failed to create empty known_hosts file: %v", err)
	}
	cb, err := knownhosts.New(knownHostsPath)
	if err != nil {
		t.Fatalf("knownhosts.New failed: %v", err)
	}

	cfg := ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: cb,
	}
	runner := newTestSSHRunner(t, "unknown-remote", srv.Addr, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runner.Dial(ctx)
	if err == nil {
		t.Fatal("Dial() error = nil, want ErrUnknownHostKey")
	}
	var unknownErr *ErrUnknownHostKey
	if !errors.As(err, &unknownErr) {
		t.Fatalf("Dial() error = %v (%T), want *ErrUnknownHostKey", err, err)
	}
	wantFingerprint := HostKeyFingerprint(srv.HostKey)
	if unknownErr.Fingerprint != wantFingerprint {
		t.Errorf("ErrUnknownHostKey.Fingerprint = %q, want %q", unknownErr.Fingerprint, wantFingerprint)
	}
}

// TestSSHRunner_Dial_MismatchedHostKey_ReturnsUnderlyingErrorNotUnknownHostKey
// is WARNING 2's regression guard: wrapHostKeyCallback must distinguish a
// key *mismatch* (a host previously trusted under a different key -- the
// actual MITM-relevant case TOFU exists to catch) from a never-seen host.
// A pre-populated known_hosts entry for this host's address, but with a
// different (decoy) key than the server actually presents, must surface
// the underlying knownhosts.KeyError directly -- not get relabeled as
// *ErrUnknownHostKey, which would silently understate the severity of a
// possible active MITM as a routine first-connection prompt.
func TestSSHRunner_Dial_MismatchedHostKey_ReturnsUnderlyingErrorNotUnknownHostKey(t *testing.T) {
	srv := startTestSSHServer(t)

	_, decoyPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate decoy key: %v", err)
	}
	decoySigner, err := ssh.NewSignerFromSigner(decoyPriv)
	if err != nil {
		t.Fatalf("failed to build decoy signer: %v", err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(srv.Addr)}, decoySigner.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write known_hosts with a decoy key: %v", err)
	}
	cb, err := knownhosts.New(knownHostsPath)
	if err != nil {
		t.Fatalf("knownhosts.New failed: %v", err)
	}

	cfg := ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{testClientAuth(t)},
		HostKeyCallback: cb,
	}
	runner := newTestSSHRunner(t, "mismatched-remote", srv.Addr, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runner.Dial(ctx)
	if err == nil {
		t.Fatal("Dial() against a host with a mismatched known_hosts entry = nil error, want a rejection")
	}

	var unknownErr *ErrUnknownHostKey
	if errors.As(err, &unknownErr) {
		t.Fatalf("Dial() error = %v, want the underlying mismatch error surfaced directly -- not relabeled as *ErrUnknownHostKey (that would understate a possible MITM as a routine unknown-host prompt)", err)
	}

	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) {
		t.Fatalf("Dial() error = %v (%T), want it to wrap a *knownhosts.KeyError", err, err)
	}
	if len(keyErr.Want) == 0 {
		t.Error("knownhosts.KeyError.Want is empty, want at least one entry (this is a mismatch against a known decoy key, not an unknown host)")
	}
}

// TestSSHRunner_Dial_HungHandshake_TimesOutWithinBudget covers Task
// 2.1.1g's dial case: a listener that accepts the TCP connection but never
// speaks SSH must not block Dial past ctx's deadline, and the underlying
// TCP connection must be force-closed afterward (not leaked).
func TestSSHRunner_Dial_HungHandshake_TimesOutWithinBudget(t *testing.T) {
	addr, liveConns := startStallingListener(t)

	cfg := ssh.ClientConfig{
		User: "test",
		Auth: []ssh.AuthMethod{testClientAuth(t)},
		// The stalling peer never gets far enough for a host-key callback
		// to be invoked at all, so this deliberately fails loudly if it
		// ever is -- there is no reason to reach for
		// ssh.InsecureIgnoreHostKey() even in test scaffolding.
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error {
			return errors.New("unexpected: host key callback invoked during hung-handshake test")
		},
	}
	runner := newTestSSHRunner(t, "stalling-remote", addr, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	err := runner.Dial(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Dial() error = nil, want a context-deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Dial() error = %v, want wrapping context.DeadlineExceeded", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Dial() took %v, want well under ctx's 1s budget plus scheduling slack", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for liveConns() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := liveConns(); n != 0 {
		t.Errorf("live stalling connections = %d after Dial timeout, want 0 (connection not force-closed)", n)
	}
}

// TestSSHRunner_Run_HungCommand_TimesOutWithinBudget covers Task 2.1.1g's
// command case: a remote command that never terminates and never produces
// output must not block Run past ctx's deadline, and the shared pooled
// client must remain usable afterward (proving the timed-out session was
// closed independently rather than corrupting/leaking the connection).
func TestSSHRunner_Run_HungCommand_TimesOutWithinBudget(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "hung-command-remote", srv.Addr, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	_, err := runner.Run(ctx, "", "sleep", "30")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() error = nil, want a context-deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() error = %v, want wrapping context.DeadlineExceeded", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Run() took %v, want well under ctx's 1s budget plus scheduling slack", elapsed)
	}

	// The pooled connection must still be usable: a fresh, short call on
	// the same runner should succeed quickly, proving the earlier timeout
	// closed only its own session rather than the shared client.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	out, err := runner.Run(ctx2, "", "echo", "-n", "still-alive")
	if err != nil {
		t.Fatalf("Run() after prior timeout returned error: %v", err)
	}
	if string(out) != "still-alive" {
		t.Errorf("Run() after prior timeout = %q, want %q", out, "still-alive")
	}
}

// TestSSHRunner_Run_ReusesPooledClient verifies two SSHRunners sharing a
// pool and target name reuse the exact same *ssh.Client rather than each
// dialing independently -- a narrower, unit-level companion to
// ssh_pool_test.go's full load test.
func TestSSHRunner_Run_ReusesPooledClient(t *testing.T) {
	srv := startTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "shared-remote", Addr: srv.Addr}

	r1 := NewSSHRunner(target, cfg, WithSSHClientPool(pool))
	r2 := NewSSHRunner(target, cfg, WithSSHClientPool(pool))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := r1.Run(ctx, "", "echo", "-n", "one"); err != nil {
		t.Fatalf("r1.Run() error: %v", err)
	}
	if _, err := r2.Run(ctx, "", "echo", "-n", "two"); err != nil {
		t.Fatalf("r2.Run() error: %v", err)
	}

	c1, ok1 := pool.Peek(target.Name)
	c2, ok2 := pool.Peek(target.Name)
	if !ok1 || !ok2 {
		t.Fatal("expected a pooled client for target.Name after both runners' Run calls")
	}
	if c1 != c2 {
		t.Error("r1 and r2 did not share the same pooled *ssh.Client")
	}
	// Run releases its reference when its session closes (one reference per
	// open channel, not per runner -- see client()'s doc comment), so after
	// both completed Run calls the refcount is back to zero even though the
	// underlying client is still pooled.
	if got := pool.RefCount(target.Name); got != 0 {
		t.Errorf("pool.RefCount(%q) after both Run calls completed = %d, want 0 (each Run releases its reference when its channel closes)", target.Name, got)
	}
}

// TestSSHRunner_Run_MaxSessionsRejection_DoesNotEvictSharedClient is the
// VIOLATION regression guard: a client.NewSession() failure caused by a
// per-connection channel-limit rejection (OpenSSH's MaxSessions, simulated
// here via startMaxSessionsTestServer) is NOT connection death and must not
// evict the shared *ssh.Client -- doing so would force-close the pooled
// connection out from under every other concurrently-active caller, the
// exact cascade failure the pool exists to prevent, reintroduced at the
// channel layer. Sequence: hold the one available session slot open via
// Start, drive a concurrent Run into the rejection, assert the pool still
// holds the *same* client afterward, then free the slot and assert a
// subsequent Run on the very same client succeeds -- proving the
// connection was never touched, not just that Peek still returns something.
func TestSSHRunner_Run_MaxSessionsRejection_DoesNotEvictSharedClient(t *testing.T) {
	const maxSessions = 1
	srv := startMaxSessionsTestServer(t, maxSessions)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "max-sessions-remote", Addr: srv.Addr}
	runner := NewSSHRunner(target, cfg, WithSSHClientPool(pool))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Occupy the one available session slot with a long-lived command.
	stdin, stdout, wait, err := runner.Start(ctx, "", "sleep", "30")
	if err != nil {
		t.Fatalf("Start() (occupying the session slot) error: %v", err)
	}

	clientBefore, ok := pool.Peek(target.Name)
	if !ok {
		t.Fatal("expected a pooled client after Start()")
	}

	// The slot is held, so this Run must be rejected at the channel-open
	// step -- a MaxSessions-class failure, not a dead connection.
	if _, err := runner.Run(ctx, "", "echo", "-n", "rejected"); err == nil {
		t.Fatal("Run() while the session slot is held = nil error, want a rejection")
	}

	clientAfter, ok := pool.Peek(target.Name)
	if !ok {
		t.Fatal("shared client was evicted by a MaxSessions-class newSession failure, want it to survive")
	}
	if clientAfter != clientBefore {
		t.Fatal("pool holds a different *ssh.Client after the rejection -- the shared connection was evicted/redialed when it should not have been")
	}

	// Free the slot, then prove the *same* connection is still genuinely
	// usable -- not just that Peek happens to return a non-nil pointer.
	_ = stdin.Close()
	_ = stdout.Close()
	if err := wait(); err != nil {
		// "sleep 30" killed via the closed session is expected to report a
		// non-nil wait error; only the slot-freeing side effect matters here.
		t.Logf("wait() on the closed long-lived session returned: %v (expected)", err)
	}

	out, err := runner.Run(ctx, "", "echo", "-n", "still-works")
	if err != nil {
		t.Fatalf("Run() after freeing the slot on the same connection: %v", err)
	}
	if string(out) != "still-works" {
		t.Errorf("Run() after freeing the slot = %q, want %q", out, "still-works")
	}
	if got, _ := pool.Peek(target.Name); got != clientBefore {
		t.Error("the connection used for the final Run() is not the original client -- a redial happened somewhere it shouldn't have")
	}
}
