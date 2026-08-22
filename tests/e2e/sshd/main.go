// Command sshd is a standalone, minimal SSH server used ONLY as a real SSH target for
// tests/e2e/remote-workspaces.spec.ts (ssh-remote-workspaces Phase 6 Epic 6.3, Story 6.3.1).
//
// Playwright specs run in Node and cannot reach into this repo's existing Go-only test SSH
// server helpers (session/tmux/ssh_test_server_test.go, session/git/ssh_test_server_test.go) --
// those are t.Cleanup-scoped to a single `go test` process. This binary is the standalone
// equivalent, spawned and torn down by remote-workspaces.spec.ts's own test.beforeAll/afterAll
// (tests/e2e/helpers/test-sshd.ts) -- NOT by global-setup.ts/global-teardown.ts, deliberately:
// a single shared instance across the whole Playwright run would let one project's
// TestRemoteConnection call trust its host key, leaving every subsequent project's TOFU-flow
// assertions racing against an already-trusted host instead of a genuinely unknown one. Each
// spec-owned instance gets an OS-assigned port and a fresh host key, so every project sees its
// own independent TOFU state.
//
// It reuses this repo's existing gliderlabs/ssh dependency and mirrors
// ssh_test_server_test.go's realExecSSHHandler pattern: every exec request runs a REAL local
// shell command via `sh -c`. That's what makes this a genuine SSH round-trip target rather
// than a canned fake -- TestRemoteConnection/TrustRemoteHostKey/GenerateRemoteIdentity dial it
// for real, and a session created "against" it drives real git/tmux commands over a real SSH
// exec channel, exactly like the production SSHRunner does against a real remote box. The
// "remote" happens to be this same host (loopback-only, matching
// startRealExecTestSSHServer's doc comment on why that's safe: only the test process itself
// ever connects).
//
// A fresh Ed25519 host key is generated on every start, so its fingerprint is unknown to any
// app instance's known_hosts on every e2e run -- exercising the real TOFU
// (trust-on-first-use) flow without needing a separate "unknown host key" fixture.
//
// PublicKeyHandler accepts any client key (same as the Go test helpers), so
// GenerateRemoteIdentity's freshly-minted keypair is accepted without the spec having to copy
// an authorized_keys line onto this server first.
//
// Usage: go run ./tests/e2e/sshd [-port N]
// Prints a single line "READY <port>" to stdout once listening (port 0 means OS-assigned), then
// blocks until SIGINT/SIGTERM.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func main() {
	port := flag.Int("port", 0, "TCP port to listen on (0 = OS-assigned)")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshd: listen failed: %v\n", err)
		os.Exit(1)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshd: host key generation failed: %v\n", err)
		os.Exit(1)
	}
	hostSigner, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshd: host signer failed: %v\n", err)
		os.Exit(1)
	}

	srv := &gliderssh.Server{
		Handler: execHandler,
		PublicKeyHandler: func(gliderssh.Context, gliderssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(hostSigner)

	actualPort := ln.Addr().(*net.TCPAddr).Port
	// Single machine-readable line the Node side parses (helpers/test-server.ts convention:
	// print readiness rather than poll a port file). Flush via stdout's line-buffering on a
	// TTY-less pipe -- fmt.Println already writes a full line + newline in one Write call.
	fmt.Printf("READY %d\n", actualPort)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		_ = srv.Close()
		_ = ln.Close()
	case err := <-errCh:
		if err != nil && !errors.Is(err, gliderssh.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "sshd: serve failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// execHandler runs every exec request's raw command line via the host's real shell, mirroring
// session/tmux/ssh_test_server_test.go's realExecSSHHandler.
func execHandler(s gliderssh.Session) {
	raw := s.RawCommand()
	if raw == "" {
		_ = s.Exit(0)
		return
	}

	cmd := safeexec.CommandContext(s.Context(), "sh", "-c", raw)
	cmd.Stdin = s
	cmd.Stdout = s
	cmd.Stderr = s.Stderr()

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			_ = s.Exit(exitErr.ExitCode())
			return
		}
		_, _ = io.WriteString(s.Stderr(), fmt.Sprintf("exec error: %v\n", err))
		_ = s.Exit(1)
		return
	}
	_ = s.Exit(0)
}
