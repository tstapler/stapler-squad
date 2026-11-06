package tymux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/tstapler/tymux/clients/go/liveness"
	"github.com/tstapler/tymux/clients/go/selecting"
	"github.com/tstapler/tymux/clients/go/udsdialer"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// tymuxdPIDFileName is the PID file this package writes when it spawns
// tymuxd, distinct from daemon/daemon.go's "daemon.pid" so the two
// supervised processes never collide on the same file.
const tymuxdPIDFileName = "tymuxd.pid"

// daemonHealthCheckTimeout bounds StopTymuxd's executable-identity lookup so a stuck `ps`
// cannot block shutdown. The liveness probe has its own, longer daemonProbeTimeout.
const daemonHealthCheckTimeout = 2 * time.Second

// daemonProbeTimeout bounds the GetServerInfo probe. It is deliberately generous: a tymuxd on a
// host at load average 150+ answers slowly, and "slow" must not be mistaken for "dead".
//
// Vars, not consts, so a test can shrink them.
var (
	daemonProbeTimeout = 10 * time.Second
	// daemonProbeDialTimeout bounds the probe's TCP dial; a timeout (full accept backlog) is
	// inconclusive, only a refusal means nothing is listening.
	daemonProbeDialTimeout = 5 * time.Second
)

// portDialTimeout bounds the raw TCP dial used to distinguish "nothing
// listening yet" from "something is squatting the port" (Task 2.1.2d).
const portDialTimeout = 500 * time.Millisecond

// daemonStartAttempts/daemonStartBackoffStart/daemonStartBackoffMax bound how
// long EnsureDaemonRunning retries checkDaemonHealthy after spawning tymuxd
// before giving up. These reuse session/tmux/tmux.go's
// serverStartAttempts/serverStartBackoffStart/serverStartBackoffMax values
// (8 attempts, 100ms-3s backoff, ~9.1s worst-case) as a documented starting
// point, not a tymuxd-specific incident-derived value the way tmux's were --
// no such incident history exists yet for tymuxd. Revisit if real-world
// startup proves slower/faster.
//
// Declared as vars, not consts, so unit tests can substitute tiny bounds
// (avoiding a multi-second real sleep in a fast unit test) instead of
// racing the production worst-case wait -- restored via t.Cleanup in every
// test that overrides them.
var (
	daemonStartAttempts     = 8
	daemonStartBackoffStart = 100 * time.Millisecond
	daemonStartBackoffMax   = 3 * time.Second
)

// checkDaemonHealthyFn/startDaemonAttemptFn/portListeningFn are the
// injection seams EnsureDaemonRunning calls through, so unit tests can
// substitute deterministic fakes instead of spawning a real tymuxd
// subprocess or making a real network call -- mirroring the
// startServer/isNotRunning closure-injection pattern
// session/tmux/tmux.go's EnsureServerRunning/ensureServerRunningWithRetry
// use (tmux.go:631,647), adapted to package-level vars here since
// EnsureDaemonRunning's exported signature is fixed to (ctx, cfg) and the
// singleflight-coalesced spawn (Task 2.1.2g) needs one seam reachable from
// both a direct call and a concurrent-caller test.
// stopTymuxdFn is the same injection-seam pattern applied to StopTymuxd:
// EnsureDaemonRunning's failure paths below call through this var (rather
// than StopTymuxd directly) so a unit test can assert cleanup fired without
// needing a real PID file or a real process to kill.
var (
	checkDaemonHealthyFn = checkDaemonHealthy
	startDaemonAttemptFn = startDaemonAttempt
	portListeningFn      = portListening
	stopTymuxdFn         = StopTymuxd
	unixListeningFn      = unixSocketListening
)

// spawnSF coalesces concurrent EnsureDaemonRunning callers that both observe
// a cold daemon for the same DaemonConfig.Addr onto exactly one spawn
// attempt, rather than racing to bind the port (Task 2.1.2g, ADR-004).
// Keyed by cfg.Addr -- not a single shared key -- so two DaemonConfigs with
// different addresses (e.g. two STAPLER_SQUAD_INSTANCEs) never coalesce with
// each other. Mirrors session/tmux/tmux.go's existsSF/noCacheSF precedent
// for this exact "coalesce concurrent callers onto one real attempt" shape.
var spawnSF singleflight.Group //nolint:exhaustruct

// TymuxdReady is a proof token returned by EnsureDaemonRunning, mirroring
// session/tmux/tmux.go's TmuxServerReady -- a caller holding one has
// confirmation that a healthy tymuxd was reachable (already running, or just
// spawned and verified) at the moment EnsureDaemonRunning returned.
//
// Spawned distinguishes which case it was: true only when THIS call actually
// started a new tymuxd process (the reuse path -- an already-healthy daemon
// answered checkDaemonHealthy before any spawn was attempted -- always
// leaves this false). This matters because StopTymuxd() kills whatever PID
// is recorded in the shared, per-configDir tymuxd.pid file: two processes
// sharing the same STAPLER_SQUAD_INSTANCE (including the default/"shared"
// instance, or a named instance started twice) can observe the SAME
// already-running daemon via the reuse path. A caller that registers a
// stop-on-shutdown hook unconditionally -- rather than gating it on
// Spawned -- risks killing a daemon a DIFFERENT, still-running process
// depends on. This is the exact "isolated by config dir but sharing the
// daemon/socket underneath" hazard config.IsNamedInstance's doc comment
// documents for tmux (an orphan sweep once killed 5 unrelated production
// tmux sessions); do not reintroduce it here unguarded (see
// session/backend_tymux.go's Story 2.1.3 callers and main.go's Epic 2.2
// OnStop registration, both of which must check Spawned before registering
// a stop hook).
type TymuxdReady struct {
	Spawned bool
}

// checkDaemonHealthy reports whether tymuxd is usable by squad's clients: true if its Unix control
// socket answers, or (degraded) only its TCP listener does, which the client falls back to. A
// daemon that accepts a connection but answers slowly is busy, not dead, and counts as alive so
// no caller kills it. The reason for a failed probe is logged at Warn.
func checkDaemonHealthy(ctx context.Context, cfg DaemonConfig) bool {
	return probeDaemon(ctx, cfg) >= daemonProbeBusy
}

// daemonProbeResult orders probe outcomes from worst to best.
type daemonProbeResult int

const (
	daemonProbeNotListening daemonProbeResult = iota // connection refused: definitively down
	daemonProbeNoSocketFile                          // unix endpoint only: no usable socket (ENOENT or an undialable path); probeDaemon maps it away and never returns it
	daemonProbeUnhealthy                             // something answered, but not as tymuxd
	daemonProbeBusy                                  // accepted the connection but did not answer in time: inconclusive
	daemonProbeAccessDenied                          // alive, but the kernel (EACCES/EPERM) or the daemon (Unauthenticated/PermissionDenied) refuses this user: never killed, never "wedged"
	daemonProbeDegraded                              // the Unix control socket is down but the deprecated TCP listener answers: usable via the fallback
	daemonProbeHealthy                               // answered GetServerInfo over the primary transport (the Unix socket; TCP when no socket is known)
)

// probeDaemon probes the Unix control socket first, the transport squad prefers. A socket that
// answers is Healthy: squad does not need TCP. Only when the socket is not listening (or answers
// as something else) does it try TCP, and a TCP listener that answers is reported as
// daemonProbeDegraded when the socket file exists but refuses (the control socket died) so the
// monitor surfaces that squad is on the unauthenticated fallback. When no socket file exists the
// daemon is older than the socket (or has not created it yet): TCP answering is plain Healthy.
// With no known socket for the address, TCP alone decides.
func probeDaemon(ctx context.Context, cfg DaemonConfig) daemonProbeResult {
	result, _, _ := probeDaemonWithTCP(ctx, cfg)
	return result
}

// probeDaemonWithTCP is probeDaemon that also returns the TCP probe's own result when TCP was
// consulted (tcpProbed), so the monitor does not probe TCP a second time.
func probeDaemonWithTCP(ctx context.Context, cfg DaemonConfig) (result, tcpResult daemonProbeResult, tcpProbed bool) {
	path := unixSocketPath(cfg)
	noSocketFile := false
	summary := ProbeSummary{TCP: probeNotProbed, Unix: probeNotProbed}
	defer func() {
		if tcpProbed {
			summary.TCP = tcpResult.String()
		}
		recordProbe(cfg.Addr, summary)
	}()
	if path != "" {
		unix, unixHTTP := newUnixTransport(path)
		defer unixHTTP.CloseIdleConnections()
		unixResult := probeEndpoint(ctx, "unix", path, unix)
		summary.Unix = unixResult.String()
		switch unixResult {
		case daemonProbeHealthy, daemonProbeBusy, daemonProbeAccessDenied:
			return unixResult, 0, false // a silent or forbidden socket is inconclusive: never killed, TCP not consulted
		case daemonProbeNoSocketFile:
			noSocketFile = true
		}
		if ctx.Err() != nil {
			return daemonProbeNotListening, 0, false
		}
	}
	tcpResult = probeTCP(ctx, cfg)
	if path == "" || tcpResult != daemonProbeHealthy {
		return tcpResult, tcpResult, true
	}
	if noSocketFile {
		logOnce("nosocket|"+path, log.Info, "[tymux] tymuxd has no Unix control socket (older daemon?); using TCP", "socket", path, "addr", cfg.Addr)
		return daemonProbeHealthy, tcpResult, true
	}
	return daemonProbeDegraded, tcpResult, true
}

// probeTCP probes the TCP listener only.
func probeTCP(ctx context.Context, cfg DaemonConfig) daemonProbeResult {
	addr := cfg.Addr
	if sharedDaemonBlockedInTest(addr) {
		addr = deadLoopbackAddr
	}
	hostPort, err := addrToHostPort(addr)
	if err != nil {
		return daemonProbeNotListening
	}
	tcp, tcpHTTP := newTCPTransport(addr)
	defer tcpHTTP.CloseIdleConnections()
	return probeEndpoint(ctx, "tcp", hostPort, tcp)
}

// onceLogged remembers which diagnoses were already logged this episode; a Healthy probe of the
// same endpoint clears its keys so a recurrence logs again.
var onceLogged sync.Map

func logOnce(key string, emit func(string, ...any), msg string, args ...any) {
	if _, dup := onceLogged.LoadOrStore(key, struct{}{}); dup {
		return
	}
	emit(msg, args...)
}

func clearOnce(address string) {
	for _, kind := range []string{"denied", "nosocket"} {
		onceLogged.Delete(kind + "|" + address)
	}
}

// dialFn is the seam probeEndpoint, portListening and unixSocketListening dial through.
var dialFn = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	if network == "unix" { // the probe sends an RPC next: apply the same trust check as the client
		return selecting.CheckedDial(address, nil)(ctx, network, address, timeout)
	}
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, network, address)
}

// dialRefused reports whether a dial error proves nothing is listening: only ECONNREFUSED (and
// ENOENT for a Unix socket path). Timeouts and resource errors (EMFILE, ENFILE, ENOBUFS, ENOMEM,
// EADDRNOTAVAIL, ENETUNREACH...) say nothing about the daemon, and mistaking them for "down"
// under fd exhaustion would get a live daemon holding every pane killed.
func dialRefused(network string, err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || (network == "unix" && errors.Is(err, syscall.ENOENT))
}

// probeLog records liveness.Probe's own diagnostics so probeEndpoint can emit them (or replace
// them, for access-denied, with squad's once-per-episode wording) after classification.
type probeLog struct {
	msg  string
	args []any
}

func (p *probeLog) Warn(msg string, args ...any) { p.msg, p.args = msg, args }

// probeEndpoint classifies one endpoint with upstream's liveness.Probe, then layers squad's
// refinements on it: a unix path that cannot exist is "no socket file" rather than "refusing",
// and access-denied is logged once per episode. transport must target exactly this endpoint.
func probeEndpoint(ctx context.Context, network, address string, transport rpcTransport) daemonProbeResult {
	var dialErr error
	var diag probeLog
	result := liveness.Probe(ctx, network, address, transport, liveness.Options{
		DialTimeout:  daemonProbeDialTimeout,
		ProbeTimeout: daemonProbeTimeout,
		Dial: func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
			conn, err := dialFn(ctx, network, address, timeout)
			dialErr = markAccessDenied(refusedIfSocketPathBroken(network, err))
			return conn, dialErr
		},
		Logger: &diag,
	})
	switch result {
	case liveness.Healthy:
		clearOnce(address)
		return daemonProbeHealthy
	case liveness.AccessDenied:
		what := "tymuxd rejected our credentials; it is alive and left running, but this client is not authorized"
		if dialErr != nil {
			what = "permission denied connecting to tymuxd; it is alive and left running, but this user may not use it"
		}
		logOnce("denied|"+address, log.Warn, "[tymux] daemon probe: "+what, "network", network, "address", address, "err", probeErr(diag))
		return daemonProbeAccessDenied
	case liveness.NotListening:
		if network == "unix" && errors.Is(dialErr, syscall.ENOENT) {
			return daemonProbeNoSocketFile
		}
		log.Warn("[tymux] "+diag.msg, diag.args...)
		return daemonProbeNotListening
	case liveness.Busy:
		log.Warn("[tymux] "+diag.msg, diag.args...)
		return daemonProbeBusy
	default:
		if dialErr != nil && ctx.Err() != nil {
			return daemonProbeNotListening // cancelled mid-dial: liveness says Unhealthy, squad's contract is "not up"
		}
		log.Warn("[tymux] "+diag.msg, diag.args...)
		return daemonProbeUnhealthy
	}
}

// probeErr extracts the "err" attribute liveness attached to its diagnostic.
func probeErr(p probeLog) error {
	for i := 0; i+1 < len(p.args); i += 2 {
		if p.args[i] == "err" {
			if err, ok := p.args[i+1].(error); ok {
				return err
			}
		}
	}
	return nil
}

// ownedSocketPath is the control socket a daemon at cfg is, or may be, listening on: cfg.SocketPath
// (spawn exports it to tymuxd as TYMUXD_SOCKET_PATH) or the shared default daemon's path for the
// default address. Unfiltered by address pairing, so it is the right input for the never-kill-a-
// daemon-whose-socket-answers veto.
func ownedSocketPath(cfg DaemonConfig) string {
	if sharedDaemonBlockedInTest(cfg.Addr) {
		return ""
	}
	if cfg.SocketPath != "" {
		return cleanSocketPath(cfg.SocketPath)
	}
	if cfg.Addr == defaultTymuxdAddr {
		return udsdialer.DefaultSocketPath(os.Getuid()) // not ResolveSocketPath: TYMUXD_SOCKET_PATH names another daemon
	}
	return ""
}

// unixSocketPath is the socket squad may talk to as its primary transport for cfg.Addr, or "" when
// it cannot be proven to belong to the daemon at that address (see socketPairedWithAddr) or does
// not fit sun_path. Used by the client selector and the probe; the kill veto uses ownedSocketPath.
func unixSocketPath(cfg DaemonConfig) string {
	path := ownedSocketPath(cfg)
	if path != "" && cfg.SocketPath != "" && !socketPairedWithAddr(cfg) {
		return ""
	}
	if path != "" && !socketPathUsable(path) {
		warnOverlongSocketPath(path)
		return ""
	}
	return path
}

// cleanSocketPath normalizes a configured socket path. Upstream's trust check refuses unclean or
// relative paths, but that is a spelling problem, not a trust one, so every consumer (client,
// probe, kill veto) sees the same absolute clean form.
func cleanSocketPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// socketPathMax is the longest sun_path the platform accepts, including the NUL terminator.
var socketPathMax = func() int {
	if runtime.GOOS == "darwin" {
		return 104
	}
	return 108
}()

// socketPathBroken reports a dial error proving the path itself can never work (too long, a
// non-directory component, a symlink loop), as opposed to a daemon that is merely absent or busy.
func socketPathBroken(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTDIR) ||
		errors.Is(err, syscall.ENAMETOOLONG) || errors.Is(err, syscall.ELOOP)
}

// socketPathUsable reports whether path fits in sockaddr_un.sun_path. Beyond it connect() fails
// with EINVAL forever, so such a daemon is reached over TCP only.
func socketPathUsable(path string) bool { return len(path) < socketPathMax }

var warnedOverlongSockets sync.Map

func warnOverlongSocketPath(path string) {
	if _, dup := warnedOverlongSockets.LoadOrStore(path, struct{}{}); dup {
		return
	}
	log.Warn("[tymux] control socket path is too long for this platform; using TCP only",
		"path", path, "len", len(path), "max", socketPathMax-1)
}

// socketPairedWithAddr rejects the env-resolved combinations where only one half was overridden,
// since the other half then names a different daemon: TYMUXD_ADDR with an instance-hashed socket,
// or TYMUXD_SOCKET_PATH with the default address. A hand-built cfg (not the env resolution) is
// trusted as given.
func socketPairedWithAddr(cfg DaemonConfig) bool {
	rc := ResolveDaemonConfig()
	if rc.Addr != cfg.Addr || rc.SocketPath != cfg.SocketPath {
		return true
	}
	envAddr, envSock := os.Getenv("TYMUXD_ADDR") != "", os.Getenv("TYMUXD_SOCKET_PATH") != ""
	if envAddr == envSock {
		return true
	}
	if envSock { // socket overridden, address not
		return namedInstance()
	}
	return !namedInstance() || cfg.Addr == instanceDaemonAddr() // address overridden, socket not
}

// unixSocketListening reports whether the control socket might be live: anything but a definite
// refusal (timeout, resource error) counts as listening, like portListening.
func unixSocketListening(cfg DaemonConfig) bool {
	path := ownedSocketPath(cfg)
	if path == "" || !socketPathUsable(path) {
		return false // an undialable path cannot hold a live daemon
	}
	conn, err := dialFn(context.Background(), "unix", path, portDialTimeout)
	if err != nil {
		return !dialRefused("unix", err) && !socketPathBroken(err)
	}
	_ = conn.Close()
	return true
}

// portListening reports whether something is accepting TCP connections at
// cfg.Addr's host:port, independent of whether it speaks tymuxd's gRPC
// protocol. checkDaemonHealthy alone can't distinguish "nothing there yet"
// from "something is squatting the port and will never become healthy" --
// this raw dial is the second signal EnsureDaemonRunning uses to make that
// call (Task 2.1.2d). Only a refusal means absent: a timeout (full accept
// backlog) or a resource error is inconclusive and counts as listening.
func portListening(cfg DaemonConfig) bool {
	hostPort, err := addrToHostPort(cfg.Addr)
	if err != nil {
		return false
	}
	conn, err := dialFn(context.Background(), "tcp", hostPort, portDialTimeout)
	if err != nil {
		return !dialRefused("tcp", err)
	}
	_ = conn.Close()
	return true
}

func addrToHostPort(addr string) (string, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("tymux: invalid daemon addr %q: %w", addr, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("tymux: invalid daemon addr %q: no host", addr)
	}
	return u.Host, nil
}

// tymuxdOutputLogName is where startDaemonAttempt captures the spawned
// tymuxd's combined stdout/stderr -- BUG-110: this codebase had no way to
// see WHY a tymuxd cold start failed (e.g. its own "another tymuxd is
// already starting against <sock>" message when a stuck predecessor still
// held the lock), because that output was previously discarded entirely.
const tymuxdOutputLogName = "tymuxd-output.log"

// openTymuxdOutputLog opens (creating/appending) $configDir/tymuxd-output.log
// for startDaemonAttempt to redirect the spawned tymuxd's stdout/stderr into.
// Returns nil, nil on any failure to resolve/open it -- diagnostics being
// unavailable must never block starting the daemon itself.
func openTymuxdOutputLog() *os.File {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(configDir, tymuxdOutputLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return f
}

// startDaemonAttempt spawns tymuxd from cfg.BinaryPath and returns its
// *os.Process. Follows the same detached-daemon pattern used elsewhere in
// this codebase: detached Stdin, a parent-death-aware SysProcAttr
// (EnsurePdeathsig, below), a PID file written to $configDir/tymuxd.pid
// (distinct filename from other daemons' PID files), and
// a background cmd.Wait() so an exited child is reaped, never left a zombie. Stdout/Stderr are captured to
// $configDir/tymuxd-output.log (openTymuxdOutputLog), not discarded.
//
// Explicitly sets TYMUXD_ADDR on the child's environment (never relies on
// inheriting it from this process's own environment): tymuxd itself reads
// TYMUXD_ADDR to decide its bind address, expecting a bare "host:port" (no
// "http://" scheme) -- confirmed empirically against the real binary. cfg.Addr
// carries the scheme (needed by NewRealTransport's gRPC client), and for the
// default/unnamed instance this process's own environment usually has no
// TYMUXD_ADDR set at all, so relying on inheritance would silently spawn
// tymuxd bound to its own hardcoded default (127.0.0.1:7419) instead of
// cfg.Addr's instance-derived port (ResolveDaemonConfig, Task 1.3.1a) --
// defeating the whole point of per-instance port derivation for any named
// STAPLER_SQUAD_INSTANCE. Masked only in the single case where cfg.Addr
// already equals that hardcoded default.
func startDaemonAttempt(cfg DaemonConfig) (*os.Process, error) {
	hostPort, err := addrToHostPort(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("tymux: cannot derive TYMUXD_ADDR for spawned tymuxd: %w", err)
	}

	cmd := safeexec.CommandContext(context.Background(), cfg.BinaryPath)
	cmd.Env = append(os.Environ(), "TYMUXD_ADDR="+hostPort)
	if cfg.SocketPath != "" {
		// Without this, two STAPLER_SQUAD_INSTANCEs both collide on tymuxd's
		// own default Unix-socket lock even though they bind different
		// TYMUXD_ADDR ports — see resolveSocketPath's doc comment.
		cmd.Env = append(cmd.Env, "TYMUXD_SOCKET_PATH="+cfg.SocketPath)
	}
	cmd.Stdin = nil
	if out := openTymuxdOutputLog(); out != nil {
		// Not closed here: cmd.Start() below dup2's this fd into the child,
		// which keeps its own reference; closing our copy immediately after
		// Start() is the standard os/exec pattern for a detached child this
		// process only Wait()s on in the background (the child owns
		// the fd from here on).
		defer func() { _ = out.Close() }()
		cmd.Stdout = out
		cmd.Stderr = out
	}

	// Adopt the existing SIGKILL-on-parent-death convention (not SIGTERM) --
	// same as session/external_tmux_streamer.go, session/mux/multiplexer.go,
	// and session/tmux/server_registry.go's use of this helper. A known gap
	// on macOS (no Pdeathsig equivalent) is accepted here too, same as those
	// call sites: the next EnsureDaemonRunning call's health-check-and-reap
	// path is what recovers from an orphaned tymuxd on that platform.
	safeexec.EnsurePdeathsig(cmd)
	detachFromSession(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tymux: failed to start tymuxd (%s): %w", cfg.BinaryPath, err)
	}

	if err := writeTymuxdPIDFile(cmd.Process.Pid); err != nil {
		// The background Wait has not started yet, so Process.Kill() still works -- without
		// this, a PID-file write failure here would orphan the just-spawned
		// child with zero record of it anywhere (no PID file, and the
		// *os.Process handle is about to go out of scope unreleased and
		// unkilled).
		if killErr := cmd.Process.Kill(); killErr != nil {
			log.Warn("[tymux] failed to kill tymuxd after PID file write failure (process may be orphaned)", "err", killErr)
		}
		return nil, fmt.Errorf("tymux: failed to write tymuxd PID file: %w", err)
	}

	// Release() would not reap: the exited child stays a zombie that still answers signal 0, so
	// terminateProcess would wait out its whole grace period. Reap it in the background instead.
	go func() { _ = cmd.Wait() }()

	return cmd.Process, nil
}

func writeTymuxdPIDFile(pid int) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	pidFile := filepath.Join(configDir, tymuxdPIDFileName)
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	return nil
}

// ensureDaemonRunningWithRetry polls healthy up to attempts times with
// exponential backoff (capped at backoffMax) between tries, to ride out a
// tymuxd startup that doesn't answer its first few health checks. Same
// shape as session/tmux/tmux.go's ensureServerRunningWithRetry
// (tmux.go:647-664); healthy is injected so tests can simulate this
// deterministically instead of depending on a real subprocess's startup
// timing. Returns true as soon as healthy reports true, false if every
// attempt is exhausted.
func ensureDaemonRunningWithRetry(healthy func() bool, attempts int, backoffStart, backoffMax time.Duration) bool {
	backoff := backoffStart
	for i := 0; i < attempts; i++ {
		if healthy() {
			return true
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
	}
	return false
}

// EnsureDaemonRunning reuses an already-healthy tymuxd at cfg.Addr, or
// spawns one and retries-verify until it becomes healthy. Concurrent callers
// that both observe a cold daemon for the same cfg.Addr coalesce onto
// exactly one spawn attempt via spawnSF (Task 2.1.2g, ADR-004) rather than
// racing to bind the port. Returns a TymuxdReady proof token on success;
// returns ErrTymuxdPortSquatted (wrapped) if something is listening at
// cfg.Addr but never answers ListSessions correctly after every retry --
// this never silently proceeds with a session pointed at an unverified
// daemon (research/pitfalls.md §2/§4).
//
// The initial reuse check uses the caller's own ctx (each caller's own
// budget applies to its own check). The coalesced spawn-and-retry closure
// deliberately does NOT reuse whichever caller happened to become the
// singleflight leader's ctx for its retry health-checks -- if the leader's
// own ctx were cancelled partway through (e.g. its own 15s per-call budget,
// ADR-004), every coalesced follower would see spurious failures for the
// rest of the retry budget even though its own ctx might still be valid.
// Mirrors session/tmux/tmux.go's existsSF/noCacheSF precedent, which builds
// a fresh context.Background()-derived timeout inside the closure rather
// than depending on any one caller's context.
func EnsureDaemonRunning(ctx context.Context, cfg DaemonConfig) (TymuxdReady, error) {
	return ensureDaemonRunning(ctx, cfg, daemonRestartColdStart)
}

// ensureDaemonRunning is EnsureDaemonRunning with the tymux_daemon_restarts_total reason to
// record when it has to spawn, so a deliberate replace counts once as "replace", not also as
// "cold_start".
func ensureDaemonRunning(ctx context.Context, cfg DaemonConfig, spawnReason string) (TymuxdReady, error) {
	if sharedDaemonBlockedInTest(cfg.Addr) {
		return TymuxdReady{}, ErrSharedDaemonInTest
	}
	if err := ctx.Err(); err != nil {
		return TymuxdReady{}, err
	}
	if checkDaemonHealthyFn(ctx, cfg) {
		return TymuxdReady{}, nil // reuse case: correct steady-state outcome
	}
	if err := ctx.Err(); err != nil {
		return TymuxdReady{}, err
	}

	listenersUp := func() bool { return portListeningFn(cfg) || unixListeningFn(cfg) }
	v, err, _ := spawnSF.Do(cfg.Addr, func() (interface{}, error) {
		// checkDaemonHealthyFn above already confirmed nothing answers
		// ListSessions correctly at cfg.Addr -- if a PID file nonetheless
		// names a still-alive process, that process is stuck (e.g. its TCP
		// listener panicked/closed while the process itself survived --
		// BUG-110, confirmed against a real production incident) and must be
		// cleared before spawning a replacement. Without this, the
		// replacement's own child collides with the still-held tymuxd.sock
		// lock and dies within milliseconds, and by the time the retry loop
		// below gives up, the PID file has already been overwritten with
		// that new, already-dead attempt's PID -- the stop call after the
		// retry loop then targets the wrong process, and the actual stuck
		// one is never killed, permanently poisoning every future call for
		// cfg.Addr. Idempotent/safe to call unconditionally: a no-op when no
		// PID file exists (the normal cold-start case).
		//
		// Killing tymuxd destroys every pane child it owns, so a daemon with any listener
		// still up is never stopped: a single failed probe is retried with the startup grace
		// first, and a listener that stays unresponsive is reported, not killed.
		if listenersUp() {
			log.Warn("[tymux] tymuxd did not answer the first probe but a listener is up, retrying before treating it as dead", "addr", cfg.Addr)
			if ensureDaemonRunningWithRetry(
				func() bool { return checkDaemonHealthyFn(context.Background(), cfg) },
				daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax,
			) {
				return TymuxdReady{}, nil
			}
			if tcpUp, unixUp := portListeningFn(cfg), unixListeningFn(cfg); tcpUp || unixUp {
				// The daemon may hold live panes, so an unresponsive listener is reported, never killed.
				err := unresponsiveListenersError(cfg, tcpUp, unixUp)
				log.Warn("[tymux] tymuxd did not answer and has a live listener; not killing it", "addr", cfg.Addr, "tcp_dial_accepted", tcpUp, "unix_dial_accepted", unixUp, "err", err)
				return TymuxdReady{}, err
			}
		}
		staleReason, ok := stopReasonFrom(ctx)
		if !ok {
			staleReason = StopReason{Kind: StopStaleDaemon, Detail: "no listener answers a probe"}
		}
		staleReason.Probes = lastProbeFor(cfg.Addr)
		if stopErr := stopTymuxdFn(staleReason); stopErr != nil {
			if !errors.Is(stopErr, errTymuxdPIDUnverified) {
				return TymuxdReady{}, fmt.Errorf("tymux: cannot safely stop unhealthy daemon: %w", stopErr)
			}
			// Neither listener is up and the recorded pid is not demonstrably tymuxd, so
			// StopTymuxd dropped the stale pid file without killing anything.
			log.Warn("[tymux] ignoring unverifiable tymuxd pid file, spawning a replacement", "err", stopErr)
		}

		spawnedAt := time.Now()
		if _, spawnErr := startDaemonAttemptFn(cfg); spawnErr != nil {
			return TymuxdReady{}, fmt.Errorf("tymux: failed to spawn tymuxd at %s: %w", cfg.Addr, spawnErr)
		}
		recordDaemonRestart(spawnReason)

		// Independent of any one caller's ctx (see doc comment above) --
		// each checkDaemonHealthy call still self-bounds via
		// daemonHealthCheckTimeout.
		retryCtx := context.Background()
		healthy := ensureDaemonRunningWithRetry(
			func() bool { return checkDaemonHealthyFn(retryCtx, cfg) },
			daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax,
		)
		if healthy {
			recordDaemonStartDuration(time.Since(spawnedAt), daemonStartOK)
			return TymuxdReady{Spawned: true}, nil
		}
		recordDaemonStartDuration(time.Since(spawnedAt), daemonStartFailed)

		// The health-check retry loop exhausted without the daemon becoming
		// healthy. The process spawnAttemptFn just started (if any) is about
		// to be abandoned by this closure -- stop it via the recorded PID
		// file before returning the error, so a failed cold start doesn't
		// leave an orphan squatting cfg.Addr that poisons every future
		// EnsureDaemonRunning call for it. startDaemonAttempt returned only the
		// *os.Process handle -- StopTymuxd() already knows how to
		// read the PID file startDaemonAttempt wrote, kill that PID, and
		// remove the file, idempotently.
		coldStartReason := StopReason{
			Kind:   StopFailedColdStart,
			Detail: fmt.Sprintf("replacement never became healthy after %d attempts", daemonStartAttempts),
			Probes: lastProbeFor(cfg.Addr),
		}
		if stopErr := stopTymuxdFn(coldStartReason); stopErr != nil {
			log.Warn("[tymux] failed to stop orphaned tymuxd after failed cold start", "err", stopErr)
		}

		if portListeningFn(cfg) {
			return TymuxdReady{Spawned: true}, fmt.Errorf("tymux: %w: TCP dial to %s still accepts connections after our replacement was stopped, but nothing answered as tymuxd in %d attempts", ErrTymuxdPortSquatted, cfg.Addr, daemonStartAttempts)
		}
		return TymuxdReady{Spawned: true}, fmt.Errorf("tymux: tymuxd at %s did not become healthy after %d attempts", cfg.Addr, daemonStartAttempts)
	})
	// Spawned stays true on a post-spawn failure so the caller still owns what it started.
	ready, _ := v.(TymuxdReady)
	return ready, err
}

// errTymuxdPIDUnverified marks a pid file whose process is not demonstrably tymuxd (recycled pid,
// or an executable path that no longer resolves, e.g. after a brew upgrade). Nothing was killed.
var errTymuxdPIDUnverified = errors.New("tymux: recorded daemon pid is not verifiably tymuxd")

func validateTymuxdProcess(ctx context.Context, pid int, binary string) error {
	if pid <= 1 {
		return fmt.Errorf("%w: refusing unsafe daemon PID %d", errTymuxdPIDUnverified, pid)
	}
	exe, err := processExeFn(ctx, pid)
	if err != nil {
		return fmt.Errorf("%w: cannot verify daemon PID %d: %w", errTymuxdPIDUnverified, pid, err)
	}
	configured, err := exec.LookPath(binary)
	if err != nil {
		configured = binary
	}
	actual := strings.TrimSuffix(exe, " (deleted)")
	if filepath.Clean(actual) == filepath.Clean(configured) {
		return nil
	}
	a, aerr := os.Stat(actual)
	b, berr := os.Stat(configured)
	if aerr == nil && berr == nil && os.SameFile(a, b) {
		return nil
	}
	return fmt.Errorf("%w: refusing to kill PID %d: executable %q does not match %q", errTymuxdPIDUnverified, pid, exe, configured)
}

// StopTymuxd stops the recorded daemon after verifying its executable identity.
// A missing PID file is a no-op. reason is logged and recorded (see RecentDaemonStops). Shutdown callers must first establish spawn
// ownership; executable identity alone does not establish ownership.
func StopTymuxd(reason StopReason) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pidFile := filepath.Join(configDir, tymuxdPIDFileName)
	data, err := os.ReadFile(pidFile) // #nosec G304 -- pidFile is configDir + the hardcoded tymuxdPIDFileName constant, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read tymuxd PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid tymuxd PID file format: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find tymuxd process: %w", err)
	}
	defer func() { _ = proc.Release() }()
	if err := proc.Signal(syscall.Signal(0)); errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return os.Remove(pidFile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), daemonHealthCheckTimeout)
	defer cancel()
	if err := validateTymuxdProcess(ctx, pid, TymuxdBinary()); err != nil {
		// Never kill it, but a pid file we cannot trust must not block respawning forever.
		if rmErr := os.Remove(pidFile); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Warn("[tymux] failed to remove untrusted tymuxd PID file", "err", rmErr)
		}
		return err
	}

	if err := terminateProcess(pid, reason); err != nil {
		return fmt.Errorf("failed to stop tymuxd process: %w", err)
	}

	if err := os.Remove(pidFile); err != nil {
		return fmt.Errorf("failed to remove tymuxd PID file: %w", err)
	}

	log.Info("[tymux] tymuxd process stopped successfully", "pid", pid)
	return nil
}

// terminateProcess sends SIGTERM (tymuxd flushes its session records on it), waits up to
// daemonStopGracePeriod for pid to exit, then escalates to SIGKILL.
func terminateProcess(pid int, reason StopReason) error {
	attempt := beginStop(pid, reason)
	if err := signalProcessFn(pid, syscall.SIGTERM); err != nil {
		if processGone(err) {
			attempt.finish(StopAlreadyGone, nil)
			return nil
		}
		attempt.finish(StopSignalFailed, err)
		return err
	}
	deadline := time.Now().Add(daemonStopGracePeriod)
	for time.Now().Before(deadline) {
		if processGone(signalProcessFn(pid, syscall.Signal(0))) {
			attempt.finish(StopExitedAfterTerm, nil)
			return nil
		}
		time.Sleep(daemonStopPollInterval)
	}
	log.Warn("[tymux] tymuxd ignored SIGTERM, escalating to SIGKILL", "pid", pid, "reason_kind", string(reason.Kind))
	if err := signalProcessFn(pid, syscall.SIGKILL); err != nil && !processGone(err) {
		attempt.finish(StopSignalFailed, err)
		return err
	}
	attempt.finish(StopEscalatedToKill, nil)
	return nil
}

// unresponsiveListenersError names exactly what was observed: each dial only proves a connection
// was accepted (or not refused), never that anything else is wrong with the daemon.
func unresponsiveListenersError(cfg DaemonConfig, tcpUp, unixUp bool) error {
	switch {
	case tcpUp && unixUp:
		return fmt.Errorf("tymux: %w: TCP dial to %s and the Unix control socket both accept connections but neither answered as tymuxd; the daemon looks wedged, not restarting it", ErrTymuxdUnresponsive, cfg.Addr)
	case unixUp:
		return fmt.Errorf("tymux: %w: TCP dial to %s was refused and the Unix control socket accepts connections but does not answer; not restarting it", ErrTymuxdControlSocketUnresponsive, cfg.Addr)
	default:
		return fmt.Errorf("tymux: %w: TCP dial to %s accepts connections but nothing answered as tymuxd, and the Unix control socket is not listening; not restarting it", ErrTymuxdPortSquatted, cfg.Addr)
	}
}

func processGone(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}
