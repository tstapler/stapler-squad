package tmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Per-operation-class timeout budgets, per research/pitfalls.md §4. These
// bound SSHRunner's own dial step internally (DialTimeout); the remaining
// two are named constants for Epic 2.3's call sites (session/tmux/tmux.go,
// session/git/worktree_git.go) to build their per-RPC ctx from when they
// migrate onto SSHRunner -- SSHRunner.Run/Start themselves race every
// blocking SSH call against whatever ctx the caller supplies rather than
// imposing a second, redundant timeout of their own (see Run/Start's doc
// comments).
const (
	// DialTimeout bounds a single SSH dial-and-handshake attempt (TCP
	// connect + SSH handshake, including host-key verification).
	DialTimeout = 10 * time.Second
	// ExistenceCheckTimeout bounds low-latency, existence-check-class
	// remote commands (e.g. "tmux has-session").
	ExistenceCheckTimeout = 3 * time.Second
	// LongRunningCommandTimeout bounds longer remote operations (e.g. "git
	// worktree add" against a fresh clone).
	LongRunningCommandTimeout = 60 * time.Second
)

// HostKeyFingerprint returns the SSH host key fingerprint (SHA256, OpenSSH's
// default presentation format, e.g. "SHA256:abcd...") for key, suitable for
// display to a user deciding whether to trust a previously-unseen host.
func HostKeyFingerprint(key ssh.PublicKey) string {
	return ssh.FingerprintSHA256(key)
}

// ErrUnknownHostKey is returned by SSHRunner.Dial (and, transitively,
// Run/Start's first call) when the configured HostKeyCallback reports the
// remote host's key as unknown -- never previously trusted -- rather than
// mismatched. SSHRunner never falls back to ssh.InsecureIgnoreHostKey(): an
// unknown host key stops the handshake and surfaces this typed error
// instead of silently connecting. Wraps the underlying HostKeyCallback
// error (e.g. a *knownhosts.KeyError) via Unwrap.
type ErrUnknownHostKey struct {
	Host        string
	Fingerprint string
	Err         error
}

func (e *ErrUnknownHostKey) Error() string {
	return fmt.Sprintf("ssh: unknown host key for %s (fingerprint %s): %v", e.Host, e.Fingerprint, e.Err)
}

func (e *ErrUnknownHostKey) Unwrap() error { return e.Err }

// wrapHostKeyCallback wraps cb so that an "unknown host" outcome (a
// *knownhosts.KeyError with no Want entries -- i.e. the host has no known
// key at all, as opposed to a mismatched one) is translated into
// ErrUnknownHostKey. A key *mismatch* (Want non-empty, which knownhosts
// treats as a possible MITM signal) is intentionally left as the
// underlying error, unlaundered, rather than being folded into "unknown."
// A nil cb is treated the same as an unknown host key for every connection
// attempt -- refusing to connect rather than defaulting to
// ssh.InsecureIgnoreHostKey()'s silent-accept behavior, which this type
// exists to prevent.
func wrapHostKeyCallback(cb ssh.HostKeyCallback) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if cb == nil {
			return &ErrUnknownHostKey{
				Host:        hostname,
				Fingerprint: HostKeyFingerprint(key),
				Err:         errors.New("no HostKeyCallback configured"),
			}
		}
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			return &ErrUnknownHostKey{
				Host:        hostname,
				Fingerprint: HostKeyFingerprint(key),
				Err:         err,
			}
		}
		return err
	}
}

// Modern algorithm allowlist pinned explicitly per research/pitfalls.md §2:
// golang.org/x/crypto/ssh's package-default Config (KeyExchanges, Ciphers,
// MACs) has, across versions, included algorithms later deprecated for
// weakness, and its defaults have shifted silently across minor version
// bumps. SSHRunner pins its own allowlist rather than trusting whatever
// ssh.Config{} defaults to at whatever x/crypto/ssh version this repo is
// pinned to at build time.
//
// No Compression entry appears in this allowlist, and ssh.Config has no
// Compression field to pin -- Task 4.4.1f (disable SSH compression on the
// terminal data channel, per research/pitfalls.md §1's escape-sequence-
// boundary-corruption risk) is satisfied by construction, not by an
// explicit setting: golang.org/x/crypto/ssh never implements a compression
// algorithm at all. Its own supportedCompressions list
// (golang.org/x/crypto/ssh/common.go) is exactly []string{"none"} in every
// version this repo has depended on (verified against v0.35.0 through
// v0.53.0 in the local module cache), so "none" is the only outcome the
// handshake can ever negotiate regardless of what a remote sshd offers.
// zlib@openssh.com is simply not reachable through this client.
var (
	allowedKeyExchanges = []string{
		"curve25519-sha256",
		"curve25519-sha256@libssh.org",
	}
	allowedCiphers = []string{
		"chacha20-poly1305@openssh.com",
		"aes256-gcm@openssh.com",
		"aes128-gcm@openssh.com",
	}
	allowedMACs = []string{
		"hmac-sha2-256-etm@openssh.com",
		"hmac-sha2-512-etm@openssh.com",
	}
)

func pinModernAlgorithms(cfg *ssh.ClientConfig) {
	cfg.KeyExchanges = allowedKeyExchanges
	cfg.Ciphers = allowedCiphers
	cfg.MACs = allowedMACs
}

// defaultSSHClientPool is the process-wide shared pool every SSHRunner uses
// unless constructed with WithSSHClientPool -- the production default that
// gives "one shared *ssh.Client per remote name" for free across every
// SSHRunner instance in the process, per the Design Decision.
var defaultSSHClientPool = NewSSHClientPool()

// SSHRunner implements CommandRunner over a persistent, pooled *ssh.Client:
// Run and Start execute remote commands as new SSH channels on the shared
// connection for Target.Name (session/tmux/ssh_pool.go), rather than
// dialing a fresh TCP+SSH connection per call. It never uses
// ssh.InsecureIgnoreHostKey() -- callers supply a pre-built ssh.ClientConfig
// (Phase 3 constructs this from IdentityRef-resolved signer +
// KnownHostsStore; this package accepts it directly for testability, e.g.
// a knownhosts.New()-backed or fixed-key HostKeyCallback in tests).
type SSHRunner struct {
	target  SSHTarget
	config  ssh.ClientConfig
	pool    *SSHClientPool
	backoff *sshBackoff
}

// SSHRunnerOption configures an SSHRunner at construction time.
type SSHRunnerOption func(*SSHRunner)

// WithSSHClientPool overrides the pool an SSHRunner dials/shares connections
// through. Defaults to the process-wide defaultSSHClientPool; tests use
// this to inject an isolated pool per test.
func WithSSHClientPool(pool *SSHClientPool) SSHRunnerOption {
	return func(r *SSHRunner) { r.pool = pool }
}

// withSSHBackoffConfig overrides the reconnect backoff/circuit-open
// schedule. Unexported: only same-package tests need a faster-than-
// production schedule to keep test runtime bounded.
func withSSHBackoffConfig(config sshBackoffConfig) SSHRunnerOption {
	return func(r *SSHRunner) { r.backoff = newSSHBackoff(config) }
}

// NewSSHRunner constructs an SSHRunner for target, using config as the base
// SSH client configuration. config.HostKeyCallback is wrapped to translate
// an unknown-host outcome into ErrUnknownHostKey (see wrapHostKeyCallback);
// config.Config's KeyExchanges/Ciphers/MACs are overwritten with SSHRunner's
// pinned modern allowlist regardless of what config supplies, since pinning
// them explicitly -- not trusting either the package default or a caller's
// possibly-stale choice -- is the whole point of Task 2.1.1d.
func NewSSHRunner(target SSHTarget, config ssh.ClientConfig, opts ...SSHRunnerOption) *SSHRunner {
	cfg := config
	cfg.HostKeyCallback = wrapHostKeyCallback(config.HostKeyCallback)
	pinModernAlgorithms(&cfg)

	r := &SSHRunner{
		target:  target,
		config:  cfg,
		pool:    defaultSSHClientPool,
		backoff: newSSHBackoff(defaultSSHBackoffConfig()),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

var _ CommandRunner = (*SSHRunner)(nil)

// IsRemote implements CommandRunner. SSHRunner always runs commands on a
// different host than this process (paired with LocalRunner.IsRemote()
// returning false -- the single mechanism every "is this remote" check
// reads, per architecture-review.md Blocker 1 / command_runner.go's doc
// comment).
func (r *SSHRunner) IsRemote() bool { return true }

// Dial establishes (or reuses, via the shared pool) the SSH connection for
// this remote, bounded by DialTimeout nested inside ctx. Run and Start dial
// lazily on first use, so most callers never need this directly; it's
// exposed for callers that want to eagerly validate connectivity and
// host-key trust before doing real work (e.g. RemoteHealthProber, Epic
// 6.4, reusing this same pooled client for its liveness checks rather than
// opening a dedicated connection).
func (r *SSHRunner) Dial(ctx context.Context) error {
	_, err := r.client(ctx)
	return err
}

// client acquires a reference to the shared *ssh.Client for this runner's
// target (dialing through the pool if not already pooled) and returns it.
// Every successful call increments the pool's reference count for
// target.Name by one -- callers own exactly one reference per successful
// client() call and must pool.Release(r.target.Name) exactly once when done
// with it (Run/Start do this around each SSH channel they open), matching
// the Design Decision's "reference-counted... last channel closing does not
// tear down the client" framing: one reference per open channel, not one
// per SSHRunner instance.
//
// The backoff/circuit-open gate (allowAttempt) only governs new dial
// *attempts* -- it is skipped (and never blocks) when the pool already has
// a live client for this target, so an open circuit can never prevent a
// caller from using an already-live pooled connection, only from triggering
// a fresh dial attempt against a host that's currently failing.
func (r *SSHRunner) client(ctx context.Context) (*ssh.Client, error) {
	if _, ok := r.pool.Peek(r.target.Name); !ok {
		if err := r.backoff.allowAttempt(); err != nil {
			return nil, err
		}
	}

	dialCtx, cancel := context.WithTimeout(ctx, DialTimeout)
	defer cancel()

	client, err := r.pool.GetOrDial(dialCtx, r.target, &r.config)
	r.backoff.recordResult(err == nil)
	if err != nil {
		return nil, fmt.Errorf("ssh: dial %s (%s): %w", r.target.Name, r.target.Addr, err)
	}
	return client, nil
}

// newSession opens a new SSH session (channel) on client, bounded by ctx.
//
// NOTE (documented, deliberate gap): unlike the dial step, we cannot
// force-close an in-flight NewSession() call on ctx expiry without closing
// the shared *ssh.Client itself -- which would break every other concurrent
// user of this pooled connection, exactly the MaxStartups-class failure the
// pool exists to prevent (see the Design Decision in
// project_plans/ssh-remote-workspaces/implementation/plan.md). So on ctx
// expiry here, the caller is never blocked past ctx's deadline, but the
// background goroutine calling client.NewSession() may continue running
// until it completes or the underlying connection dies on its own. If it
// completes successfully *after* the caller has already given up, the
// resulting session is self-closed here rather than left open toward the
// remote's MaxSessions ceiling -- there is no one left to hand it to. This
// hand-off is race-free: ch is unbuffered, so the goroutine's send can only
// complete in rendezvous with a still-waiting receiver below; if ctx has
// already fired by the time the goroutine's own select runs, the send
// branch is (by construction) not ready and the close branch is chosen
// deterministically instead -- there is no window where a session is
// dropped without being closed.
func (r *SSHRunner) newSession(ctx context.Context, client *ssh.Client) (*ssh.Session, error) {
	type outcome struct {
		session *ssh.Session
		err     error
	}
	ch := make(chan outcome)
	go func() {
		s, err := client.NewSession()
		select {
		case ch <- outcome{session: s, err: err}:
			// Handed off to the still-waiting caller below.
		case <-ctx.Done():
			if err == nil {
				_ = s.Close()
			}
		}
	}()

	select {
	case res := <-ch:
		return res.session, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("ssh: new session on %s: %w", r.target.Name, ctx.Err())
	}
}

// Run implements CommandRunner.Run: opens a new session on the shared
// client and returns its combined stdout+stderr, matching
// CommandRunner.Run's contract. Every blocking SSH call along the way
// (client acquisition/dial, session open, CombinedOutput) is raced against
// ctx.Done(); on expiry the session is force-closed (safe -- sessions are
// per-call, never shared) and a context-deadline error is returned rather
// than blocking indefinitely.
//
// A newSession failure here is NOT treated as connection death and does
// NOT evict the shared client: client.NewSession() can fail for reasons
// that say nothing about the underlying connection's health -- most
// importantly a per-connection channel-limit rejection (OpenSSH's default
// MaxSessions 10), which is an expected, benign condition at exactly the
// concurrency level ssh_pool_test.go's load test targets (15-20 concurrent
// sessions sharing one connection). Evicting on every such error would
// force-close the shared client out from under every other concurrently-
// active caller -- the connection-layer cascade failure the pool exists to
// prevent, reintroduced one layer up at the channel layer. Dead-connection
// detection has exactly one source of truth: the pool's own Client.Wait()
// background watcher (session/tmux/ssh_pool.go), which is unaffected by
// (and doesn't need help from) a single failed channel-open.
func (r *SSHRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	client, err := r.client(ctx)
	if err != nil {
		return nil, err
	}
	defer r.pool.Release(r.target.Name)

	session, err := r.newSession(ctx, client)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()

	cmd := buildRemoteCommand(dir, name, args)

	type outcome struct {
		out []byte
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		out, runErr := session.CombinedOutput(cmd)
		ch <- outcome{out: out, err: runErr}
	}()

	select {
	case res := <-ch:
		return res.out, res.err
	case <-ctx.Done():
		_ = session.Close() // force-close to unblock the goroutine and abort the remote command
		return nil, fmt.Errorf("ssh: run %q on %s: %w", cmd, r.target.Name, ctx.Err())
	}
}

// Start implements CommandRunner.Start: opens a new session on the shared
// client, wires its stdin/stdout as pipes, and starts cmd without waiting
// for it to complete, returning a wait func mirroring (*exec.Cmd).Wait().
// session.Setenv is deliberately not used (most sshd AcceptEnv is disabled
// by default); environment normalization is the caller's job via the
// command line itself, not this method.
//
// Only the session-open and the Start() call itself are raced against
// ctx.Done() -- once Start() has returned successfully, the process is
// running and wait() is meant to be called out-of-band by the caller
// later, exactly like LocalRunner.Start's cmd.Wait, so wait() is
// intentionally not ctx-bound here.
//
// The pool reference client() acquires is released exactly once: on every
// early-error return path here, or -- once Start succeeds -- inside the
// returned wait func, the first time it's called. A caller that never
// calls wait() after a successful Start leaks that one reference (the
// same caveat LocalRunner.Start's cmd.Wait carries for reaping the OS
// process); there is currently no production caller of CommandRunner.Start
// to observe this against (control_mode.go, the one long-lived piped use
// case in this package, still talks to *exec.Cmd directly and explicitly
// rejects remote CommandRunners -- see its IsRemote check -- pending
// Epic 2.3's remote control-mode wiring).
//
// Like Run, a newSession failure here does not evict the shared client or
// count against the reconnect backoff -- see Run's doc comment for why (a
// channel-open failure, e.g. a MaxSessions rejection, is not evidence the
// connection itself is dead; the pool's Client.Wait() watcher is the sole
// dead-connection-detection path).
func (r *SSHRunner) Start(ctx context.Context, dir, name string, args ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	client, err := r.client(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	release := func() { r.pool.Release(r.target.Name) }

	session, err := r.newSession(ctx, client)
	if err != nil {
		release()
		return nil, nil, nil, err
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		release()
		_ = session.Close()
		return nil, nil, nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		release()
		_ = session.Close()
		return nil, nil, nil, err
	}

	cmd := buildRemoteCommand(dir, name, args)

	startCh := make(chan error, 1)
	go func() { startCh <- session.Start(cmd) }()

	select {
	case startErr := <-startCh:
		if startErr != nil {
			release()
			_ = session.Close()
			return nil, nil, nil, startErr
		}
	case <-ctx.Done():
		release()
		_ = session.Close()
		return nil, nil, nil, fmt.Errorf("ssh: start %q on %s: %w", cmd, r.target.Name, ctx.Err())
	}

	var releaseOnce sync.Once
	wait := func() error {
		defer releaseOnce.Do(release)
		return session.Wait()
	}
	return stdin, &sshSessionStdout{Reader: stdout, session: session}, wait, nil
}

// sshSessionStdout adapts ssh.Session.StdoutPipe()'s io.Reader into the
// io.ReadCloser CommandRunner.Start's contract requires. There is no way to
// half-close just the stdout side of an SSH channel, so Close closes the
// whole underlying session.
type sshSessionStdout struct {
	io.Reader
	session *ssh.Session
}

func (s *sshSessionStdout) Close() error {
	return s.session.Close()
}

// SSHPtyFactory implements RemotePtyFactory (session/tmux/pty.go, Task
// 4.4.1a) using an SSHRunner's pooled *ssh.Client: RequestPty followed by
// Start(cmd) gets a genuine remote pseudo-terminal running a specific
// command (session/tmux/tmux.go's local buildAttachCommand()+PtyFactory
// equivalent for "tmux attach-session -t name", not a bare login shell --
// plan.md's Task 4.4.1b sketch names session.Shell(), but Shell() takes no
// command argument; Start(cmd) is the ssh package's primitive for "run this
// exact command with a PTY attached," which is what the raw-PTY-attach path
// needs). Used by server/services/session_service.go's StreamTerminal
// raw-PTY fallback for a remote session (control-mode's remote path,
// control_mode.go's StartControlMode, does NOT use this -- tmux control
// mode's protocol is plain text over stdin/stdout and needs no PTY, so it
// goes through CommandRunner.Start directly, same as this type's
// non-PTY sibling).
type SSHPtyFactory struct {
	runner *SSHRunner
}

// NewSSHPtyFactory constructs an SSHPtyFactory over runner's pooled connection.
func NewSSHPtyFactory(runner *SSHRunner) *SSHPtyFactory {
	return &SSHPtyFactory{runner: runner}
}

var _ RemotePtyFactory = (*SSHPtyFactory)(nil)

// StartPty implements RemotePtyFactory. Mirrors SSHRunner.Run/Start's own
// dial/session/release bookkeeping (see those methods' doc comments for why
// a newSession failure doesn't evict the shared client or count against the
// reconnect backoff) but additionally requests a PTY before starting cmd,
// and sizes it BEFORE the command starts (RequestPty's rows/cols arguments,
// taken from ws -- the same *pty.Winsize type PtyFactory.StartWithSize
// takes, rather than adjacent cols/rows ints), matching
// PtyFactory.StartWithSize's "size set before the child is forked" contract
// so a remote tmux attach-session never briefly sees a 0x0 terminal and
// self-disconnects.
func (f *SSHPtyFactory) StartPty(ctx context.Context, ws *pty.Winsize, dir, name string, args ...string) (PtySession, error) {
	client, err := f.runner.client(ctx)
	if err != nil {
		return nil, err
	}
	release := func() { f.runner.pool.Release(f.runner.target.Name) }

	sshSession, err := f.runner.newSession(ctx, client)
	if err != nil {
		release()
		return nil, err
	}

	// Matches the local attach path's terminal type (session/tmux/tmux.go's
	// buildAttachCommand sets TERM=xterm-256color when unset) and a
	// conservative fixed baud rate -- ttyname/ioctl baud settings have no
	// meaning over SSH; RequestPty requires nonzero values regardless.
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sshSession.RequestPty("xterm-256color", int(ws.Rows), int(ws.Cols), modes); err != nil {
		release()
		_ = sshSession.Close()
		return nil, fmt.Errorf("ssh: request pty on %s: %w", f.runner.target.Name, err)
	}

	stdin, err := sshSession.StdinPipe()
	if err != nil {
		release()
		_ = sshSession.Close()
		return nil, fmt.Errorf("ssh: stdin pipe on %s: %w", f.runner.target.Name, err)
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		release()
		_ = sshSession.Close()
		return nil, fmt.Errorf("ssh: stdout pipe on %s: %w", f.runner.target.Name, err)
	}

	cmd := buildRemoteCommand(dir, name, args)

	startCh := make(chan error, 1)
	go func() { startCh <- sshSession.Start(cmd) }()
	select {
	case startErr := <-startCh:
		if startErr != nil {
			release()
			_ = sshSession.Close()
			return nil, fmt.Errorf("ssh: start pty %q on %s: %w", cmd, f.runner.target.Name, startErr)
		}
	case <-ctx.Done():
		release()
		_ = sshSession.Close()
		return nil, fmt.Errorf("ssh: start pty %q on %s: %w", cmd, f.runner.target.Name, ctx.Err())
	}

	var releaseOnce sync.Once
	return &sshPtySession{
		session: sshSession,
		stdin:   stdin,
		stdout:  stdout,
		release: func() { releaseOnce.Do(release) },
	}, nil
}

// sshPtySession implements PtySession over a PTY-attached *ssh.Session.
// There is no way to half-close just one direction of an SSH channel, so
// Close tears down the whole session (mirroring sshSessionStdout.Close()'s
// same constraint for the non-PTY CommandRunner.Start path above).
type sshPtySession struct {
	session   *ssh.Session
	stdin     io.WriteCloser
	stdout    io.Reader
	release   func()
	closeOnce sync.Once
	closeErr  error
}

func (s *sshPtySession) Read(p []byte) (int, error)  { return s.stdout.Read(p) }
func (s *sshPtySession) Write(p []byte) (int, error) { return s.stdin.Write(p) }

// Resize implements PtySession via SSH's native window-change request --
// the remote-transport analog of pty.Setsize, per Task 4.4.1e and
// research/pitfalls.md §1's "SSH channel window-change request -> remote
// tmux resize-window" description of this 3-hop path.
func (s *sshPtySession) Resize(cols, rows int) error {
	return s.session.WindowChange(rows, cols)
}

func (s *sshPtySession) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.session.Close()
		s.release()
	})
	return s.closeErr
}

// shellQuote POSIX-single-quotes s so it round-trips through a remote shell
// unmodified regardless of embedded spaces or shell metacharacters,
// escaping embedded single quotes as '"'"' (close quote, literal quote,
// reopen quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// buildRemoteCommand builds the single command-line string passed to
// ssh.Session.Start/CombinedOutput, matching CommandRunner's dir semantics:
// dir == "" runs name/args in the remote login's default directory (no cd
// prefix); a non-empty dir is prefixed as "cd <dir> && ...".
func buildRemoteCommand(dir, name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(name))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	cmd := strings.Join(parts, " ")
	if dir != "" {
		cmd = "cd " + shellQuote(dir) + " && " + cmd
	}
	return cmd
}
