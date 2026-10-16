package git

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os/exec"
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// testSSHServer is a minimal in-process SSH server backing this package's
// remote_worktree_test.go tests. It is session/git's own equivalent of
// session/tmux's testSSHServer (session/tmux/ssh_test_server_test.go) rather
// than a reuse of it: that file is a _test.go file, so it is not importable
// across packages (Go test-file visibility), and its handler dispatches a
// small fixed set of builtin verbs (echo/cat/sleep/false) to avoid depending
// on host binaries for tmux's own tests — exactly the wrong shape here, since
// RemoteWorktreeOps's tests need real `git`, `mkdir`, `test`, and `sh`
// behavior, not a simulation of it. So this handler execs the client's raw
// command line through a real shell instead (testGitSSHHandler below), and
// this file duplicates only the handful of lines of test-server plumbing
// (host key generation, gliderssh.Server wiring, client auth) needed to make
// that possible — see the handler's own doc comment for why "run it for
// real" is the right tradeoff here specifically.
type testSSHServer struct {
	Addr    string
	HostKey ssh.PublicKey
}

// startTestSSHServer starts a testSSHServer listening on an OS-assigned
// loopback port, torn down automatically via t.Cleanup.
func startTestSSHServer(t *testing.T) *testSSHServer {
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
		Handler: testGitSSHHandler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			// Real identity resolution (Phase 3) is out of scope for these
			// tests -- mirrors session/tmux's test server, which accepts any
			// client public key for the same reason.
			return true
		},
	}
	srv.AddHostKey(hostSigner)

	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})

	return &testSSHServer{Addr: ln.Addr().String(), HostKey: hostSigner.PublicKey()}
}

// testGitSSHHandler executes the exact raw command line the client sent
// (session.RawCommand(), not the shlex-split session.Command() session/tmux's
// handler dispatches on) through a real `sh -c`, so RemoteWorktreeOps's
// `cd <dir> && 'git' 'worktree' 'add' ...`-shaped commands (buildRemoteCommand
// in session/tmux/ssh_runner.go) and its InitializeProjectDirectory
// `sh -c <script>` commands run exactly as they would against a real remote
// host's login shell, using the git/mkdir/test/sh binaries already on the
// machine running `go test` -- there is no meaningful way to unit-test real
// worktree/init behavior without a real shell and a real git binary on the
// other end.
func testGitSSHHandler(s gliderssh.Session) {
	cmd := safeexec.CommandContext(s.Context(), "sh", "-c", s.RawCommand())
	cmd.Stdout = s
	cmd.Stderr = s.Stderr()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_ = s.Exit(exitErr.ExitCode())
			return
		}
		_ = s.Exit(1)
		return
	}
	_ = s.Exit(0)
}

// testClientAuth returns an ssh.AuthMethod for a throwaway client keypair.
// The test server's PublicKeyHandler accepts any key, so the key itself is
// never verified -- it exists only so the client completes SSH's publickey
// auth flow. Mirrors session/tmux/ssh_test_server_test.go's helper of the
// same name (unexported, package-local, so no cross-package reuse is
// possible -- see this file's top-of-file doc comment).
func testClientAuth(t *testing.T) ssh.AuthMethod {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("failed to build client signer: %v", err)
	}
	return ssh.PublicKeys(signer)
}
