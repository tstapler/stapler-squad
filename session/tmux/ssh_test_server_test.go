package tmux

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

// testSSHServer is a minimal in-process SSH server backing every SSH test
// in this package -- github.com/gliderlabs/ssh, this repo's chosen test SSH
// server per project_plans/ssh-remote-workspaces/research/stack.md. It
// accepts any client public key (real identity resolution is Phase 3's
// concern, out of scope for Epic 2.1) and dispatches exec requests to a
// small set of builtin verbs implemented directly in Go (see
// testSSHHandler), so these tests never depend on real binaries (echo,
// sleep, cat, ...) being installed on the host running `go test`.
type testSSHServer struct {
	Addr    string
	HostKey ssh.PublicKey

	server   *gliderssh.Server
	listener net.Listener
}

// startTestSSHServer starts a testSSHServer listening on an OS-assigned
// loopback port, torn down automatically via t.Cleanup.
func startTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	return startTestSSHServerOnListener(t, ln)
}

// startTestSSHServerOnListener is startTestSSHServer's building block: it
// builds and serves a gliderlabs/ssh server on an already-constructed
// listener, letting a test wrap the listener first (e.g.
// ssh_pool_test.go's load test, which needs a MaxStartups-throttled
// listener in front of the real test sshd).
func startTestSSHServerOnListener(t *testing.T, ln net.Listener) *testSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to build host signer: %v", err)
	}

	srv := &gliderssh.Server{
		Handler: testSSHHandler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(hostSigner)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &testSSHServer{
		Addr:     ln.Addr().String(),
		HostKey:  hostSigner.PublicKey(),
		server:   srv,
		listener: ln,
	}
}

// startMaxSessionsTestServer starts a testSSHServer that accepts at most
// maxSessions concurrently-open "session" channels: once that many are
// open, further channel-open requests are protocol-level rejected
// (gossh.ResourceShortage) rather than accepted -- a faithful simulation of
// OpenSSH's MaxSessions throttle, rejecting at the SSH_MSG_CHANNEL_OPEN
// step itself. This is deliberately a different failure point than
// SessionRequestCallback (which only fires for requests *within* an
// already-open session channel, i.e. after client.NewSession() has already
// succeeded) -- MaxSessions-class rejections fail NewSession() itself
// while leaving the underlying connection completely healthy, which is
// exactly the scenario ssh_runner_test.go's eviction-avoidance test needs.
func startMaxSessionsTestServer(t *testing.T, maxSessions int) *testSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to build host signer: %v", err)
	}

	var open atomic.Int64
	srv := &gliderssh.Server{
		Handler: testSSHHandler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"session": func(srv *gliderssh.Server, conn *ssh.ServerConn, newChan ssh.NewChannel, ctx gliderssh.Context) {
				for {
					cur := open.Load()
					if cur >= int64(maxSessions) {
						_ = newChan.Reject(ssh.ResourceShortage, "MaxSessions exceeded (test)")
						return
					}
					if open.CompareAndSwap(cur, cur+1) {
						break
					}
				}
				defer open.Add(-1)
				gliderssh.DefaultSessionHandler(srv, conn, newChan, ctx)
			},
		},
	}
	srv.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &testSSHServer{
		Addr:     ln.Addr().String(),
		HostKey:  hostSigner.PublicKey(),
		server:   srv,
		listener: ln,
	}
}

// testClientAuth returns an ssh.AuthMethod for a throwaway client keypair.
// The test server's PublicKeyHandler accepts any key, so the key itself is
// never verified -- it exists only so the client completes SSH's
// publickey auth flow rather than relying on "none" auth edge-case
// behavior.
func testClientAuth(t *testing.T) ssh.AuthMethod {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}
	_ = pub
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to build client signer: %v", err)
	}
	return ssh.PublicKeys(signer)
}

// testSSHHandler dispatches an exec session's command to a small set of
// builtin verbs. Unrecognized commands exit 127 with a message on stderr.
func testSSHHandler(s gliderssh.Session) {
	cmd := s.Command()
	if len(cmd) == 0 {
		_ = s.Exit(0)
		return
	}

	switch cmd[0] {
	case "echo":
		args := cmd[1:]
		newline := true
		if len(args) > 0 && args[0] == "-n" {
			newline = false
			args = args[1:]
		}
		out := strings.Join(args, " ")
		if newline {
			out += "\n"
		}
		_, _ = io.WriteString(s, out)
		_ = s.Exit(0)
	case "cat":
		// Echoes stdin back to stdout until EOF -- used for Start's
		// round-trip test.
		_, _ = io.Copy(s, s)
		_ = s.Exit(0)
	case "sleep":
		seconds := 5
		if len(cmd) > 1 {
			if n, err := strconv.Atoi(cmd[1]); err == nil {
				seconds = n
			}
		}
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-s.Context().Done():
		}
		_ = s.Exit(0)
	case "false":
		_ = s.Exit(1)
	default:
		_, _ = fmt.Fprintf(s.Stderr(), "unknown test command: %v\n", cmd)
		_ = s.Exit(127)
	}
}

// startRealExecTestSSHServer starts a testSSHServer whose Handler actually
// executes each exec request's raw command line via the host's real shell
// (realExecSSHHandler), rather than testSSHHandler's small closed set of
// builtin verbs (echo/cat/sleep/false). tmux_remote_test.go's integration
// test needs this: SSHRunner sends real "tmux has-session"/"tmux
// new-session -A"/"tmux list-sessions" invocations (wrapped in "env -u TMUX
// TERM=..." by wrapRemoteCommand) and the test must prove the
// existence-check-then-create logic behaves correctly against a real tmux
// server, not a canned fake. Safe despite executing real, unsandboxed shell
// commands: this listens on loopback and only the test process itself ever
// connects to it, so there is no untrusted remote party (unlike a
// production sshd, which must never do this).
func startRealExecTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to build host signer: %v", err)
	}

	srv := &gliderssh.Server{
		Handler: realExecSSHHandler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(hostSigner)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &testSSHServer{
		Addr:     ln.Addr().String(),
		HostKey:  hostSigner.PublicKey(),
		server:   srv,
		listener: ln,
	}
}

// realExecSSHHandler runs s.RawCommand() (the exact, unparsed command line
// the client sent -- not s.Command()'s shlex-split view, which would need
// lossy reassembly) through the host's real shell, wiring stdin/stdout/
// stderr directly to the SSH session. This mirrors the exec-request
// semantics a real sshd provides, which is exactly what SSHRunner's
// buildRemoteCommand output (a single shell-quoted command-line string,
// optionally "cd <dir> && "-prefixed) expects on the other end.
func realExecSSHHandler(s gliderssh.Session) {
	raw := s.RawCommand()
	if raw == "" {
		_ = s.Exit(0)
		return
	}

	cmd := exec.CommandContext(s.Context(), "sh", "-c", raw)
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

// stallingListener accepts TCP connections and never speaks SSH on them --
// simulating a peer that completes the TCP handshake but stalls the SSH
// banner/handshake exchange forever. Used to test Dial's ctx-bound timeout.
// The returned live func reports the number of currently-open accepted
// connections, so a test can confirm dialSSHContext actually force-closed
// the connection on ctx expiry rather than merely returning early while
// leaking it.
func startStallingListener(t *testing.T) (addr string, live func() int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var liveCount atomic.Int64

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			liveCount.Add(1)
			// Accept and hold the connection open without ever writing an
			// SSH banner, so the client's handshake blocks indefinitely,
			// until the client force-closes it on ctx expiry. Must keep
			// draining reads in a loop (not a single Read call) -- the
			// client sends its own SSH identification string immediately
			// per RFC 4253, so a single Read returns as soon as that
			// arrives; closing right after would leave unread data in the
			// kernel receive buffer, which causes Go/the kernel to send a
			// TCP RST instead of stalling. Only the terminal read error
			// (the client closing its end on ctx expiry) should end this.
			go func(c net.Conn) {
				buf := make([]byte, 4096)
				for {
					if _, err := c.Read(buf); err != nil {
						break
					}
				}
				liveCount.Add(-1)
				_ = c.Close()
			}(conn)
		}
	}()

	return ln.Addr().String(), liveCount.Load
}

// maxStartupsListener wraps a net.Listener, allowing at most max concurrent
// open connections -- a simplified stand-in for sshd's MaxStartups throttle
// (which limits concurrent pre-auth connections; this limits concurrent
// open connections outright, which is at least as strict and is what makes
// ssh_pool_test.go's load test a meaningful proof that pooling avoids the
// throttle rather than happening to dodge it).
type maxStartupsListener struct {
	net.Listener
	sem      chan struct{}
	accepted atomic.Int64
	rejected atomic.Int64
}

func newMaxStartupsListener(ln net.Listener, max int) *maxStartupsListener {
	return &maxStartupsListener{Listener: ln, sem: make(chan struct{}, max)}
}

func (l *maxStartupsListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.sem <- struct{}{}:
			l.accepted.Add(1)
			return &releasingConn{Conn: conn, release: func() { <-l.sem }}, nil
		default:
			l.rejected.Add(1)
			_ = conn.Close()
		}
	}
}

// releasingConn frees its maxStartupsListener slot exactly once, on Close.
type releasingConn struct {
	net.Conn
	release func()
	closed  atomic.Bool
}

func (c *releasingConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		c.release()
	}
	return c.Conn.Close()
}
