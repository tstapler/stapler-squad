package tymux

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/sync/singleflight"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// tymuxdPIDFileName is the PID file this package writes when it spawns
// tymuxd, distinct from daemon/daemon.go's "daemon.pid" so the two
// supervised processes never collide on the same file.
const tymuxdPIDFileName = "tymuxd.pid"

// daemonHealthCheckTimeout bounds a single checkDaemonHealthy RPC attempt so
// a hung/black-holed port can't block startup (or a retry loop) indefinitely.
const daemonHealthCheckTimeout = 2 * time.Second

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

// checkDaemonHealthy reports whether a real tymuxd is answering at
// cfg.Addr: true only for a successful gRPC ListSessions response (proves
// process identity + protocol, not just TCP-accept), false on any error --
// connection refused, timeout, or a non-gRPC response from a squatting
// process (classifyRPCError/ErrTymuxdUnreachable already know how to
// classify that as unreachable rather than a false positive). Side-effect
// free: ListSessions with an empty filter is safe to call repeatedly.
func checkDaemonHealthy(ctx context.Context, cfg DaemonConfig) bool {
	healthCtx, cancel := context.WithTimeout(ctx, daemonHealthCheckTimeout)
	defer cancel()
	transport := NewRealTransport(cfg.Addr)
	_, err := transport.ListSessions(healthCtx, connect.NewRequest(&v1.ListSessionsRequest{}))
	return err == nil
}

// portListening reports whether something is accepting TCP connections at
// cfg.Addr's host:port, independent of whether it speaks tymuxd's gRPC
// protocol. checkDaemonHealthy alone can't distinguish "nothing there yet"
// from "something is squatting the port and will never become healthy" --
// this raw dial is the second signal EnsureDaemonRunning uses (only after
// every retry has already failed) to make that call (Task 2.1.2d).
func portListening(cfg DaemonConfig) bool {
	hostPort, err := addrToHostPort(cfg.Addr)
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", hostPort, portDialTimeout)
	if err != nil {
		return false
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

// startDaemonAttempt spawns tymuxd from cfg.BinaryPath and returns its
// *os.Process. Mirrors daemon/daemon.go's LaunchDaemon construction: detached
// Stdin/Stdout/Stderr, a parent-death-aware SysProcAttr (EnsurePdeathsig,
// below), a PID file written to $configDir/tymuxd.pid (daemon.go:361-370's
// daemon.pid pattern, distinct filename), and cmd.Process.Release()
// (daemon.go:372-376) so the child is never left as a zombie-risk once this
// process no longer needs to wait on it.
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
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Adopt the existing SIGKILL-on-parent-death convention (not SIGTERM) --
	// same as session/external_tmux_streamer.go, session/mux/multiplexer.go,
	// and session/tmux/server_registry.go's use of this helper. A known gap
	// on macOS (no Pdeathsig equivalent) is accepted here too, same as those
	// call sites: the next EnsureDaemonRunning call's health-check-and-reap
	// path is what recovers from an orphaned tymuxd on that platform.
	safeexec.EnsurePdeathsig(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tymux: failed to start tymuxd (%s): %w", cfg.BinaryPath, err)
	}

	if err := writeTymuxdPIDFile(cmd.Process.Pid); err != nil {
		// Release() has not run yet, so Process.Kill() still works -- without
		// this, a PID-file write failure here would orphan the just-spawned
		// child with zero record of it anywhere (no PID file, and the
		// *os.Process handle is about to go out of scope unreleased and
		// unkilled).
		if killErr := cmd.Process.Kill(); killErr != nil {
			log.Warn("[tymux] failed to kill tymuxd after PID file write failure (process may be orphaned)", "err", killErr)
		}
		return nil, fmt.Errorf("tymux: failed to write tymuxd PID file: %w", err)
	}

	// Release the process so it won't become a zombie when it exits -- this
	// process doesn't wait on it.
	if err := cmd.Process.Release(); err != nil {
		log.Warn("[tymux] failed to release tymuxd process (may become zombie on exit)", "err", err)
	}

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
	if checkDaemonHealthyFn(ctx, cfg) {
		return TymuxdReady{}, nil // reuse case: correct steady-state outcome
	}

	v, err, _ := spawnSF.Do(cfg.Addr, func() (interface{}, error) {
		if _, spawnErr := startDaemonAttemptFn(cfg); spawnErr != nil {
			return nil, fmt.Errorf("tymux: failed to spawn tymuxd at %s: %w", cfg.Addr, spawnErr)
		}

		// Independent of any one caller's ctx (see doc comment above) --
		// each checkDaemonHealthy call still self-bounds via
		// daemonHealthCheckTimeout.
		retryCtx := context.Background()
		healthy := ensureDaemonRunningWithRetry(
			func() bool { return checkDaemonHealthyFn(retryCtx, cfg) },
			daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax,
		)
		if healthy {
			return TymuxdReady{Spawned: true}, nil
		}

		// The health-check retry loop exhausted without the daemon becoming
		// healthy. The process spawnAttemptFn just started (if any) is about
		// to be abandoned by this closure -- stop it via the recorded PID
		// file before returning the error, so a failed cold start doesn't
		// leave an orphan squatting cfg.Addr that poisons every future
		// EnsureDaemonRunning call for it. cmd.Process.Release() already ran
		// inside startDaemonAttempt, so this process can no longer Kill() the
		// *os.Process handle directly -- StopTymuxd() already knows how to
		// read the PID file startDaemonAttempt wrote, kill that PID, and
		// remove the file, idempotently.
		if stopErr := stopTymuxdFn(); stopErr != nil {
			log.Warn("[tymux] failed to stop orphaned tymuxd after failed cold start", "err", stopErr)
		}

		if portListeningFn(cfg) {
			return nil, fmt.Errorf("tymux: %w: something is listening at %s but never answered ListSessions after %d attempts", ErrTymuxdPortSquatted, cfg.Addr, daemonStartAttempts)
		}
		return nil, fmt.Errorf("tymux: tymuxd at %s did not become healthy after %d attempts", cfg.Addr, daemonStartAttempts)
	})
	if err != nil {
		return TymuxdReady{}, err
	}
	return v.(TymuxdReady), nil
}

// StopTymuxd kills whatever process is recorded in $configDir/tymuxd.pid and
// removes the PID file; no-op (returns nil) if the PID file doesn't exist --
// mirrors daemon/daemon.go's StopDaemon's idempotent-stop contract exactly.
//
// This is idempotent, NOT ownership-aware: it has no way to tell whether the
// recorded PID belongs to a tymuxd this process itself started, or one a
// different process (sharing the same configDir -- see TymuxdReady.Spawned's
// doc comment) started and is still relying on. A caller registering this as
// an App.OnStop hook MUST gate that registration on
// EnsureDaemonRunning's TymuxdReady.Spawned being true for a spawn this
// process performed -- calling it unconditionally risks killing another
// process's daemon out from under it.
func StopTymuxd() error {
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

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("failed to stop tymuxd process: %w", err)
	}

	if err := os.Remove(pidFile); err != nil {
		return fmt.Errorf("failed to remove tymuxd PID file: %w", err)
	}

	log.Info("[tymux] tymuxd process stopped successfully", "pid", pid)
	return nil
}
